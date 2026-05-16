package handlers

import (
	"errors"
	"strconv"

	"wallet_service/internal/models"
	"wallet_service/internal/repository"
	"wallet_service/internal/server_utils"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type WithdrawDeleteHandlers struct {
	WalletRepo     *repository.WalletRepository
	WithdrawalRepo *repository.WithdrawalRepository
	ConfigRepo     *repository.ConfigRepository
}

type withdrawRequest struct {
	Amount float64 `json:"amount"`
	Method string  `json:"method"`
}

func (h *WithdrawDeleteHandlers) Withdraw(c *gin.Context) {
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

	db := h.WalletRepo.DB()
	err = db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		wlt, err := h.WalletRepo.LockByID(c.Request.Context(), tx, walletID)
		if err != nil {
			return err
		}
		if wlt.Freezed {
			return errors.New("wallet is frozen")
		}
		if wlt.WalletType != models.WalletTypeDriver && wlt.WalletType != models.WalletTypeOwner {
			return errors.New("withdraw not allowed for this wallet type")
		}

		amt := decimal.NewFromFloat(req.Amount)
		if wlt.Balance.Cmp(amt) < 0 {
			return errors.New("insufficient funds")
		}
		if wlt.WalletType == models.WalletTypeOwner {
			remaining := wlt.Balance.Sub(amt)
			if remaining.Cmp(decimal.NewFromInt(100)) < 0 {
				return errors.New("owner wallet must keep minimum balance of 100 ETB")
			}
		}

		// Check withdrawal limits
		status := models.WithdrawalStatusCompleted
		if h.ConfigRepo != nil && h.WithdrawalRepo != nil {
			limitStr, _ := h.ConfigRepo.Get(c.Request.Context(), "daily_withdrawal_limit")
			thresholdStr, _ := h.ConfigRepo.Get(c.Request.Context(), "auto_approve_threshold")
			
			if limitStr != "" {
				limit, _ := strconv.ParseFloat(limitStr, 64)
				totalToday, _ := h.WithdrawalRepo.GetTotalWithdrawnToday(c.Request.Context(), wlt.ID)
				if totalToday+req.Amount > limit {
					return errors.New("daily withdrawal limit exceeded")
				}
			}

			if thresholdStr != "" {
				threshold, _ := strconv.ParseFloat(thresholdStr, 64)
				if req.Amount > threshold {
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
			WalletID:  wlt.ID,
			Amount:    amt,
			Fee:       fee,
			NetAmount: netAmount,
			Method:    req.Method,
			Status:    status,
		}
		
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
		if err.Error() == "wallet is frozen" {
			c.JSON(403, server_utils.ErrorResponse{Message: "wallet is frozen"})
			return
		}
		if err.Error() == "insufficient funds" {
			c.JSON(400, server_utils.ErrorResponse{Message: "insufficient balance"})
			return
		}
		if err.Error() == "daily withdrawal limit exceeded" {
			c.JSON(400, server_utils.ErrorResponse{Message: "daily withdrawal limit exceeded"})
			return
		}
		c.JSON(400, server_utils.ErrorResponse{Message: err.Error()})
		return
	}

	c.JSON(202, map[string]any{"status": "accepted"})
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

func (h *WithdrawDeleteHandlers) ListWithdrawals(c *gin.Context) {
	walletIDStr := c.Param("wallet_id")
	walletID, err := strconv.ParseInt(walletIDStr, 10, 64)
	if err != nil || walletID <= 0 {
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

	withdrawals, err := h.WithdrawalRepo.ListByWalletID(c.Request.Context(), walletID, limit, offset)
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
