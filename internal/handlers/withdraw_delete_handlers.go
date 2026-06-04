package handlers

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"wallet_service/internal/auth"
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
	WalletRepo     *repository.WalletRepository
	WithdrawalRepo *repository.WithdrawalRepository
	ConfigRepo     *repository.ConfigRepository
	PaymentClient  *payment.Client
	AuthClient     *auth.Client
	Bus            *messaging.Publisher
}

type withdrawRequest struct {
	Amount              float64 `json:"amount"`
	Method              string  `json:"method"`
	AccountName         string  `json:"account_name"`
	AccountNumber       string  `json:"account_number"`
	BankCode            string  `json:"bank_code"`
	WithdrawalReference string  `json:"withdrawal_reference,omitempty"`
	Message             string  `json:"message,omitempty"`
}

func bankCodeValid(list payment.ChapaBanksResponse, code string) bool {
	code = strings.TrimSpace(code)
	for _, it := range list.Items {
		if strings.TrimSpace(it.ID) == code {
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

	walletID := strings.TrimSpace(c.Param("wallet_id"))
	if _, err := uuid.Parse(walletID); err != nil {
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

	ctx := server_utils.WithAuthBearer(c.Request.Context(), c.GetHeader("Authorization"))
	isSystemWallet := walletRow.WalletType.IsSystem()
	if isSystemWallet {
		if !server_utils.IsSuperadminRole(server_utils.XUserRole(c)) {
			c.JSON(404, server_utils.ErrorResponse{Message: "wallet not found"})
			return
		}
		if h.AuthClient == nil {
			c.JSON(502, server_utils.ErrorResponse{Message: "auth client not configured"})
			return
		}
		me, err := h.AuthClient.GetMe(ctx)
		if err != nil {
			var api *auth.APIError
			if errors.As(err, &api) && (api.StatusCode == 401 || api.StatusCode == 403) {
				c.JSON(401, server_utils.ErrorResponse{Message: "authentication error"})
				return
			}
			c.JSON(502, server_utils.ErrorResponse{Message: err.Error()})
			return
		}
		if !me.Data.TotpEnabled {
			c.JSON(403, server_utils.ErrorResponse{Message: "two-factor authentication is required for system wallet withdrawal"})
			return
		}
	} else if walletRow.UserID != callerID {
		c.JSON(403, server_utils.ErrorResponse{Message: "forbidden"})
		return
	}
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
	var withdrawalStatus models.WithdrawalStatus
	db := h.WalletRepo.DB()
	err = db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		wlt, err := h.WalletRepo.LockByID(c.Request.Context(), tx, walletID)
		if err != nil {
			return err
		}
		if wlt.Freezed {
			return errFrozen
		}
		switch wlt.WalletType {
		case models.WalletTypeDriver, models.WalletTypeOwner, models.WalletTypeSystem:
		default:
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
		// Check withdrawal limits (not applied to system wallet treasury withdrawals).
		status := models.WithdrawalStatusCompleted
		wltUUID, _ := uuid.Parse(wlt.ID)
		if !wlt.WalletType.IsSystem() && h.ConfigRepo != nil && h.WithdrawalRepo != nil {
			limitStr, _ := h.ConfigRepo.Get(c.Request.Context(), "daily_withdrawal_limit")
			thresholdStr, _ := h.ConfigRepo.Get(c.Request.Context(), "auto_approve_threshold")

			if limitStr != "" {
				limit, _ := decimal.NewFromString(limitStr)
				totalToday, _ := h.WithdrawalRepo.GetTotalWithdrawnToday(c.Request.Context(), wltUUID)
				totalTodayDec := decimal.NewFromFloat(totalToday)
				if totalTodayDec.Add(decimal.NewFromFloat(req.Amount)).GreaterThan(limit) {
					return errors.New("daily withdrawal limit exceeded")
				}
			}

			if thresholdStr != "" {
				threshold, _ := decimal.NewFromString(thresholdStr)
				if decimal.NewFromFloat(req.Amount).GreaterThan(threshold) {
					status = models.WithdrawalStatusPending
				}
			}
		}
		wlt.Balance = wlt.Balance.Sub(amt)
		if err := tx.Save(&wlt).Error; err != nil {
			return err
		}

		// Save withdrawal record
		fee := amt.Mul(decimal.NewFromFloat(0.02)) // Dummy 2% fee
		netAmount := amt.Sub(fee)

		withdrawal := models.Withdrawal{
			WalletID:  wltUUID,
			Amount:    amt,
			Fee:       fee,
			NetAmount: netAmount,
			Method:    req.Method,
			Status:    status,
		}
		
		withdrawalStatus = status
		if h.WithdrawalRepo != nil {
			return h.WithdrawalRepo.Create(c.Request.Context(), tx, &withdrawal)
		}
		return nil
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
		if err.Error() == "daily withdrawal limit exceeded" {
			c.JSON(400, server_utils.ErrorResponse{Message: "daily withdrawal limit exceeded"})
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

	actorRole := strings.ToLower(server_utils.XUserRole(c))
	if actorRole == "" {
		actorRole = string(walletRow.WalletType)
	}
	auditAction := "wallet.withdrawal_initiated"
	auditMeta := map[string]any{
		"amount":     amt.StringFixed(2),
		"currency":   "ETB",
		"method":     strings.TrimSpace(req.Method),
		"bank_code":  strings.TrimSpace(req.BankCode),
		"status":     string(withdrawalStatus),
		"tx_ref":     payOut.TxRef,
	}
	if isSystemWallet {
		auditAction = "wallet.system_withdrawal_initiated"
		auditMeta["wallet_type"] = string(models.WalletTypeSystem)
	}
	emitAudit(c, h.Bus, messaging.AuditEntry{
		Action:        auditAction,
		ActorUserID:   callerID,
		ActorUserRole: actorRole,
		TargetType:    "wallet",
		TargetID:      walletID,
		SubCityID:     walletRow.SubCityID,
		Metadata:      auditMeta,
	})

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
		"user_id":  walletRow.UserID,
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

func (h *WithdrawDeleteHandlers) reverseWithdrawCredit(ctx context.Context, walletID string, amt decimal.Decimal) error {
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
	walletID := strings.TrimSpace(c.Param("wallet_id"))
	if _, err := uuid.Parse(walletID); err != nil {
		c.JSON(400, server_utils.ErrorResponse{Message: "invalid wallet id"})
		return
	}

	db := h.WalletRepo.DB()
	err := db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
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

	emitAudit(c, h.Bus, messaging.AuditEntry{
		Action:     "wallet.deleted",
		TargetType: "wallet",
		TargetID:   walletID,
	})

	c.Status(204)
}

func (h *WithdrawDeleteHandlers) ListWithdrawals(c *gin.Context) {
	walletIDStr := strings.TrimSpace(c.Param("wallet_id"))
	walletUUID, err := uuid.Parse(walletIDStr)
	if err != nil {
		c.JSON(400, server_utils.ErrorResponse{Message: "invalid wallet id"})
		return
	}

	limit := 50
	offset := 0
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if o := c.Query("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed > 0 {
			offset = parsed
		}
	}

	if h.WithdrawalRepo == nil {
		c.JSON(500, server_utils.ErrorResponse{Message: "withdrawal repository not configured"})
		return
	}

	withdrawals, err := h.WithdrawalRepo.ListByWalletID(c.Request.Context(), walletUUID, limit, offset)
	if err != nil {
		c.JSON(500, server_utils.ErrorResponse{Message: "failed to list withdrawals"})
		return
	}

	c.JSON(200, map[string]interface{}{
		"items":  withdrawals,
		"limit":  limit,
		"offset": offset,
	})
}
