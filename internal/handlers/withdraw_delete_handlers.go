package handlers

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"wallet_service/internal/messaging"
	"wallet_service/internal/models"
	"wallet_service/internal/payment"
	"wallet_service/internal/repository"
	"wallet_service/internal/server_utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type WithdrawDeleteHandlers struct {
	WalletRepo    *repository.WalletRepository
	PaymentClient *payment.Client
	Bus           *messaging.Publisher
}

type withdrawRequest struct {
	Amount              float64 `json:"amount"`
	AccountName         string  `json:"account_name"`
	AccountNumber       string  `json:"account_number"`
	BankCode            string  `json:"bank_code"`
	WithdrawalReference string  `json:"withdrawal_reference,omitempty"`
	Message             string  `json:"message,omitempty"`
}

func bankCodeValid(list payment.ChapaBanksResponse, code string) bool {
	code = strings.TrimSpace(code)
	for _, it := range list.Items {
		if strings.TrimSpace(it.Code) == code {
			return true
		}
	}
	return false
}

func accountEnding(num string) string {
	n := strings.TrimSpace(num)
	if len(n) <= 4 {
		return n
	}
	return n[len(n)-4:]
}

func (h *WithdrawDeleteHandlers) ListChapaBanks(c *gin.Context) {
	ctx := server_utils.WithAuthBearer(c.Request.Context(), c.GetHeader("Authorization"))
	out, err := h.PaymentClient.GetChapaBanks(ctx)
	if err != nil {
		var api *payment.APIError
		if errors.As(err, &api) {
			if api.StatusCode >= 500 {
				c.JSON(api.StatusCode, server_utils.ErrorResponse{Message: err.Error()})
				return
			}
		}
		c.JSON(502, server_utils.ErrorResponse{Message: err.Error()})
		return
	}
	c.JSON(200, out)
}

func (h *WithdrawDeleteHandlers) Withdraw(c *gin.Context) {
	callerID, ok := server_utils.ParseXUserID(c)
	if !ok {
		c.JSON(401, server_utils.ErrorResponse{Message: "missing X-User-ID"})
		return
	}

	walletIDStr := c.Param("wallet_id")
	walletID, err := strconv.ParseInt(walletIDStr, 10, 64)
	if err != nil || walletID <= 0 {
		c.JSON(400, server_utils.ErrorResponse{Message: "invalid wallet id"})
		return
	}

	var req withdrawRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, server_utils.ErrorResponse{Message: "invalid json body"})
		return
	}
	if req.Amount <= 0 {
		c.JSON(400, server_utils.ErrorResponse{Message: "amount must be > 0"})
		return
	}
	if strings.TrimSpace(req.AccountName) == "" || strings.TrimSpace(req.AccountNumber) == "" || strings.TrimSpace(req.BankCode) == "" {
		c.JSON(400, server_utils.ErrorResponse{Message: "account_name, account_number, and bank_code are required"})
		return
	}

	walletRow, err := h.WalletRepo.GetByID(c.Request.Context(), walletID)
	if err != nil {
		if repository.IsNotFound(err) {
			c.JSON(404, server_utils.ErrorResponse{Message: "wallet not found"})
			return
		}
		c.JSON(500, server_utils.ErrorResponse{Message: "internal error"})
		return
	}
	if walletRow.UserID != callerID {
		c.JSON(403, server_utils.ErrorResponse{Message: "forbidden"})
		return
	}

	ctx := server_utils.WithAuthBearer(c.Request.Context(), c.GetHeader("Authorization"))
	banks, err := h.PaymentClient.GetChapaBanks(ctx)
	if err != nil {
		c.JSON(502, server_utils.ErrorResponse{Message: err.Error()})
		return
	}
	if !bankCodeValid(banks, req.BankCode) {
		c.JSON(400, server_utils.ErrorResponse{Message: "invalid bank_code"})
		return
	}

	amt := decimal.NewFromFloat(req.Amount)
	db := h.WalletRepo.DB()
	err = db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		wlt, err := h.WalletRepo.LockByID(c.Request.Context(), tx, walletID)
		if err != nil {
			return err
		}
		if wlt.Freezed {
			return errFrozen
		}
		if wlt.WalletType != models.WalletTypeDriver && wlt.WalletType != models.WalletTypeOwner {
			return errWithdrawType
		}
		if wlt.Balance.Cmp(amt) < 0 {
			return errInsufficient
		}
		if wlt.WalletType == models.WalletTypeOwner {
			remaining := wlt.Balance.Sub(amt)
			if remaining.Cmp(decimal.NewFromInt(100)) < 0 {
				return errOwnerMin
			}
		}
		wlt.Balance = wlt.Balance.Sub(amt)
		return tx.Save(&wlt).Error
	})
	if err != nil {
		if repository.IsNotFound(err) {
			c.JSON(404, server_utils.ErrorResponse{Message: "wallet not found"})
			return
		}
		switch err {
		case errFrozen:
			c.JSON(403, server_utils.ErrorResponse{Message: "wallet is frozen"})
			return
		case errInsufficient:
			c.JSON(422, server_utils.ErrorResponse{Message: "insufficient balance"})
			return
		case errWithdrawType:
			c.JSON(403, server_utils.ErrorResponse{Message: "withdraw not allowed for this wallet type"})
			return
		case errOwnerMin:
			c.JSON(400, server_utils.ErrorResponse{Message: "owner wallet must keep minimum balance of 100 ETB"})
			return
		}
		c.JSON(400, server_utils.ErrorResponse{Message: err.Error()})
		return
	}

	payOut, payErr := h.PaymentClient.InitiateWithdrawal(ctx, payment.WithdrawalRequest{
		Amount:              req.Amount,
		PayerUserID:         callerID,
		AccountName:         strings.TrimSpace(req.AccountName),
		AccountNumber:       strings.TrimSpace(req.AccountNumber),
		BankCode:            strings.TrimSpace(req.BankCode),
		WithdrawalReference: strings.TrimSpace(req.WithdrawalReference),
		Message:             strings.TrimSpace(req.Message),
	})
	if payErr != nil {
		_ = h.reverseWithdrawCredit(c.Request.Context(), walletID, amt)
		var api *payment.APIError
		if errors.As(payErr, &api) && (api.StatusCode == 500 || api.StatusCode == 502 || api.StatusCode == 503) {
			c.JSON(api.StatusCode, server_utils.ErrorResponse{Message: payErr.Error()})
			return
		}
		if errors.As(payErr, &api) {
			c.JSON(api.StatusCode, server_utils.ErrorResponse{Message: payErr.Error()})
			return
		}
		c.JSON(502, server_utils.ErrorResponse{Message: payErr.Error()})
		return
	}

	wAfter, err := h.WalletRepo.GetByID(c.Request.Context(), walletID)
	if err == nil {
		_ = h.Bus.PublishAnalytics(c.Request.Context(), "analytics.wallet.balance_updated", map[string]any{
			"wallet_id": walletID,
			"balance":   wAfter.Balance.StringFixed(2),
			"delta":     amt.Neg().StringFixed(2),
			"reason":    "withdrawal",
			"tx_ref":    payOut.TxRef,
		})
	}

	amtStr := amt.StringFixed(2)
	_ = h.Bus.PublishNotification(c.Request.Context(), "notification.wallet.withdrawal_initiated", map[string]any{
		"event_id": uuid.NewString(),
		"user_id":  strconv.FormatInt(walletRow.UserID, 10),
		"type":     "withdrawal_initiated",
		"title":    "Withdrawal Initiated",
		"content":  "Your withdrawal of " + amtStr + " ETB to account ending " + accountEnding(req.AccountNumber) + " has been initiated.",
		"priority": "normal",
		"category": "billing",
		"channels": []string{"sms"},
		"metadata": map[string]any{
			"amount":         amtStr,
			"currency":       "ETB",
			"bank_code":      strings.TrimSpace(req.BankCode),
			"tx_ref":         payOut.TxRef,
			"transaction_id": payOut.TransactionID,
		},
	})

	body := gin.H{
		"transaction_id": payOut.TransactionID,
		"tx_ref":         payOut.TxRef,
		"status":         payOut.Status,
	}
	if payOut.WithdrawalReference != nil && *payOut.WithdrawalReference != "" {
		body["withdrawal_reference"] = *payOut.WithdrawalReference
	} else if strings.TrimSpace(req.WithdrawalReference) != "" {
		body["withdrawal_reference"] = strings.TrimSpace(req.WithdrawalReference)
	}
	c.JSON(200, body)
}

var (
	errFrozen       = errors.New("wallet is frozen")
	errInsufficient = errors.New("insufficient balance")
	errWithdrawType = errors.New("withdraw not allowed for this wallet type")
	errOwnerMin     = errors.New("owner wallet must keep minimum balance of 100 ETB")
)

func (h *WithdrawDeleteHandlers) reverseWithdrawCredit(ctx context.Context, walletID int64, amt decimal.Decimal) error {
	db := h.WalletRepo.DB()
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		wlt, err := h.WalletRepo.LockByID(ctx, tx, walletID)
		if err != nil {
			return err
		}
		wlt.Balance = wlt.Balance.Add(amt)
		return tx.Save(&wlt).Error
	})
}

func (h *WithdrawDeleteHandlers) DeleteWallet(c *gin.Context) {
	walletIDStr := c.Param("wallet_id")
	walletID, err := strconv.ParseInt(walletIDStr, 10, 64)
	if err != nil || walletID <= 0 {
		c.JSON(400, server_utils.ErrorResponse{Message: "invalid wallet id"})
		return
	}

	db := h.WalletRepo.DB()
	err = db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		wlt, err := h.WalletRepo.LockByID(c.Request.Context(), tx, walletID)
		if err != nil {
			return err
		}
		if !wlt.Balance.Equal(decimal.Zero) {
			return errors.New("wallet balance must be zero")
		}
		return tx.Delete(&wlt).Error
	})
	if err != nil {
		if repository.IsNotFound(err) {
			c.JSON(404, server_utils.ErrorResponse{Message: "wallet not found"})
			return
		}
		c.JSON(400, server_utils.ErrorResponse{Message: err.Error()})
		return
	}

	c.Status(204)
}
