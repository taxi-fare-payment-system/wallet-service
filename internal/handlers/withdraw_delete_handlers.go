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
	WalletRepo *repository.WalletRepository
}

type withdrawRequest struct {
	Amount float64 `json:"amount"`
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

		wlt.Balance = wlt.Balance.Sub(amt)
		return tx.Save(&wlt).Error
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
