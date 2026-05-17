package handlers

import (
	"context"
	"errors"
	"strconv"
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
	Amount        float64 `json:"amount"`
	ToPhoneNumber string  `json:"to_phone_number"`
	Message       string  `json:"message,omitempty"`
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
	toPhone := strings.TrimSpace(req.ToPhoneNumber)
	if toPhone == "" {
		c.JSON(400, server_utils.ErrorResponse{Message: "invalid to_phone_number"})
		return
	}

	if h.AuthClient == nil {
		c.JSON(502, server_utils.ErrorResponse{Message: "auth client not configured"})
		return
	}
	authz := strings.TrimSpace(c.GetHeader("Authorization"))
	if authz == "" {
		c.JSON(401, server_utils.ErrorResponse{Message: "authentication error"})
		return
	}
	authCtx := server_utils.WithAuthBearer(c.Request.Context(), authz)

	fromWallet, err := h.WalletRepo.GetByID(c.Request.Context(), fromWalletID)
	if err != nil {
		c.JSON(404, server_utils.ErrorResponse{Message: "source wallet not found"})
		return
	}

	receiver, err := h.AuthClient.GetUserByPhone(authCtx, toPhone)
	if err != nil {
		var api *auth.APIError
		if errors.As(err, &api) {
			switch api.StatusCode {
			case 401, 403:
				c.JSON(401, server_utils.ErrorResponse{Message: "authentication error"})
				return
			case 404:
				c.JSON(404, server_utils.ErrorResponse{Message: "receiver not found"})
				return
			}
			c.JSON(502, server_utils.ErrorResponse{Message: err.Error()})
			return
		}
		c.JSON(502, server_utils.ErrorResponse{Message: err.Error()})
		return
	}

	toWallet, err := h.WalletRepo.GetByUserIDAndType(c.Request.Context(), receiver.Data.ID, fromWallet.WalletType)
	if err != nil {
		if repository.IsNotFound(err) {
			c.JSON(404, server_utils.ErrorResponse{Message: "destination wallet not found"})
			return
		}
		c.JSON(500, server_utils.ErrorResponse{Message: "internal error"})
		return
	}

	receiverFullName := strings.TrimSpace(receiver.Data.DisplayName)
	if receiverFullName == "" {
		receiverFullName = strings.TrimSpace(receiver.Data.Phone)
	}
	if receiverFullName == "" {
		c.JSON(502, server_utils.ErrorResponse{Message: "receiver display name not available from auth"})
		return
	}

	// For P2P, both should ideally be passengers or owners, but let's keep it general
	// as per service capability unless restricted.
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
		"receipt_url":    transferOut.ReceiptURL,
	})
}
