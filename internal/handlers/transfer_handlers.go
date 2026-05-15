package handlers

import (
	"context"
	"errors"
	"strings"

	"wallet_service/internal/auth"
	"wallet_service/internal/payment"
	"wallet_service/internal/repository"
	"wallet_service/internal/server_utils"
	"wallet_service/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type TransferHandlers struct {
	WalletRepo    *repository.WalletRepository
	WalletService *services.WalletService
	PaymentClient *payment.Client
	AuthClient    *auth.Client
}

type transferRequest struct {
	Amount     float64 `json:"amount"`
	ToWalletID string  `json:"to_wallet_id"`
	Message    string  `json:"message,omitempty"`
}

func (h *TransferHandlers) Transfer(c *gin.Context) {
	fromWalletID := strings.TrimSpace(c.Param("wallet_id"))
	var req transferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, server_utils.ErrorResponse{Message: "invalid json body"})
		return
	}

	if req.Amount <= 0 {
		c.JSON(400, server_utils.ErrorResponse{Message: "amount must be > 0"})
		return
	}
	if len(strings.TrimSpace(req.ToWalletID)) == 0 {
		c.JSON(400, server_utils.ErrorResponse{Message: "invalid to_wallet_id"})
		return
	}

	fromWallet, err := h.WalletRepo.GetByID(c.Request.Context(), fromWalletID)
	if err != nil {
		c.JSON(404, server_utils.ErrorResponse{Message: "source wallet not found"})
		return
	}

	toWallet, err := h.WalletRepo.GetByID(c.Request.Context(), strings.TrimSpace(req.ToWalletID))
	if err != nil {
		c.JSON(404, server_utils.ErrorResponse{Message: "destination wallet not found"})
		return
	}

	if h.AuthClient == nil {
		c.JSON(502, server_utils.ErrorResponse{Message: "auth client not configured"})
		return
	}
	contact, err := h.AuthClient.GetInternalUserContact(c.Request.Context(), toWallet.UserID)
	if err != nil {
		var api *auth.APIError
		if errors.As(err, &api) {
			c.JSON(502, server_utils.ErrorResponse{Message: err.Error()})
			return
		}
		c.JSON(502, server_utils.ErrorResponse{Message: err.Error()})
		return
	}
	receiverFullName := strings.TrimSpace(contact.Data.DisplayName)
	if receiverFullName == "" {
		receiverFullName = strings.TrimSpace(contact.Data.Phone)
	}
	if receiverFullName == "" {
		c.JSON(502, server_utils.ErrorResponse{Message: "receiver display name not available from auth"})
		return
	}

	amountDec := decimal.NewFromFloat(req.Amount)

	var transferOut payment.TransferResponse
	if err := h.WalletService.TransferBalanceWithHook(c.Request.Context(), fromWallet.ID, toWallet.ID, amountDec, func(ctx context.Context) error {
		out, err := h.PaymentClient.Transfer(ctx, payment.TransferRequest{
			Amount:           req.Amount,
			PayerUserID:      fromWallet.UserID,
			SenderWalletID:   fromWallet.ID,
			ReceiverWalletID: toWallet.ID,
			ReceiverID:       toWallet.UserID,
			ReceiverFullName: receiverFullName,
			TripID:           "",
			Message:          strings.TrimSpace(req.Message),
		})
		if err != nil {
			return err
		}
		transferOut = out
		return nil
	}); err != nil {
		c.JSON(400, server_utils.ErrorResponse{Message: err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"success":        true,
		"transaction_id": transferOut.TransactionID,
	})
}
