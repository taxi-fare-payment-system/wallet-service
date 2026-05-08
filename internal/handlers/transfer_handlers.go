package handlers

import (
	"strconv"
	"strings"

	"wallet_service/internal/models"
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
}

type transferRequest struct {
	Amount           float64 `json:"amount"`
	ToWalletID       int64   `json:"to_wallet_id"`
	ReceiverFullName string  `json:"receiver_full_name"`
	Message          string  `json:"message,omitempty"`
}

func (h *TransferHandlers) Transfer(c *gin.Context) {
	fromWalletIDStr := c.Param("wallet_id")
	fromWalletID, err := strconv.ParseInt(fromWalletIDStr, 10, 64)
	if err != nil || fromWalletID <= 0 {
		c.JSON(400, server_utils.ErrorResponse{Message: "invalid wallet id"})
		return
	}

	var req transferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, server_utils.ErrorResponse{Message: "invalid json body"})
		return
	}

	if req.Amount <= 0 {
		c.JSON(400, server_utils.ErrorResponse{Message: "amount must be > 0"})
		return
	}
	if req.ToWalletID <= 0 {
		c.JSON(400, server_utils.ErrorResponse{Message: "invalid to_wallet_id"})
		return
	}

	fromWallet, err := h.WalletRepo.GetByID(c.Request.Context(), fromWalletID)
	if err != nil {
		c.JSON(404, server_utils.ErrorResponse{Message: "source wallet not found"})
		return
	}

	toWallet, err := h.WalletRepo.GetByID(c.Request.Context(), req.ToWalletID)
	if err != nil {
		c.JSON(404, server_utils.ErrorResponse{Message: "destination wallet not found"})
		return
	}

	// For P2P, both should ideally be passengers or owners, but let's keep it general
	// as per service capability unless restricted.
	
	amountDec := decimal.NewFromFloat(req.Amount)

	var transferOut payment.TransferResponse
	hook := func(ctx gin.Context) error {
		out, err := h.PaymentClient.Transfer(c.Request.Context(), payment.TransferRequest{
			Amount:           req.Amount,
			PayerUserID:      strconv.FormatInt(fromWallet.UserID, 10),
			SenderWalletID:   strconv.FormatInt(fromWallet.ID, 10),
			ReceiverWalletID: strconv.FormatInt(toWallet.ID, 10),
			ReceiverID:       strconv.FormatInt(toWallet.UserID, 10),
			ReceiverFullName: strings.TrimSpace(req.ReceiverFullName),
			Message:          strings.TrimSpace(req.Message),
			Reason:           "transfer",
		})
		if err != nil {
			return err
		}
		transferOut = out
		return nil
	}

	// Note: We need a slight modification to services.TransferHook to pass context correctly or use closure.
	// But s.transferBalance handles the hook.

	if err := h.WalletService.TransferBalanceWithHook(c.Request.Context(), fromWallet.ID, toWallet.ID, amountDec, func(ctx context.Context) error {
		out, err := h.PaymentClient.Transfer(ctx, payment.TransferRequest{
			Amount:           req.Amount,
			PayerUserID:      strconv.FormatInt(fromWallet.UserID, 10),
			SenderWalletID:   strconv.FormatInt(fromWallet.ID, 10),
			ReceiverWalletID: strconv.FormatInt(toWallet.ID, 10),
			ReceiverID:       strconv.FormatInt(toWallet.UserID, 10),
			ReceiverFullName: strings.TrimSpace(req.ReceiverFullName),
			Message:          strings.TrimSpace(req.Message),
			Reason:           "transfer",
		});
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
