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
	"wallet_service/internal/services"
	"wallet_service/internal/trip"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type PayFareHandlers struct {
	WalletRepo    *repository.WalletRepository
	WalletService *services.WalletService
	PaymentClient *payment.Client
	TripClient    *trip.Client
	Bus           *messaging.Publisher
}

type payFareRequest struct {
	Amount           float64 `json:"amount"`
	DriverWalletID   int64   `json:"driver_wallet_id"`
	TripID           string  `json:"trip_id"`
	ReceiverFullName string  `json:"receiver_full_name"`
	SubCityID        string  `json:"sub_city_id"`
	AssistantID      string  `json:"assistant_id"`
	Message          string  `json:"message,omitempty"`
}

type payFareResponse struct {
	Success       bool    `json:"success"`
	TransactionID string  `json:"transaction_id"`
	ReceiptURL    *string `json:"receipt_url"`
}

func (h *PayFareHandlers) PayFare(c *gin.Context) {
	passengerWalletIDStr := c.Param("wallet_id")
	passengerWalletID, err := strconv.ParseInt(passengerWalletIDStr, 10, 64)
	if err != nil || passengerWalletID <= 0 {
		c.JSON(400, server_utils.ErrorResponse{Message: "invalid wallet id"})
		return
	}

	var req payFareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, server_utils.ErrorResponse{Message: "invalid json body"})
		return
	}
	if req.Amount <= 0 {
		c.JSON(400, server_utils.ErrorResponse{Message: "amount must be > 0"})
		return
	}
	if req.DriverWalletID <= 0 {
		c.JSON(400, server_utils.ErrorResponse{Message: "invalid driver_wallet_id"})
		return
	}
	if strings.TrimSpace(req.TripID) == "" {
		c.JSON(400, server_utils.ErrorResponse{Message: "trip_id is required"})
		return
	}
	if strings.TrimSpace(req.ReceiverFullName) == "" {
		c.JSON(400, server_utils.ErrorResponse{Message: "receiver_full_name is required"})
		return
	}

	passengerWallet, err := h.WalletRepo.GetByID(c.Request.Context(), passengerWalletID)
	if err != nil {
		if repository.IsNotFound(err) {
			c.JSON(404, server_utils.ErrorResponse{Message: "wallet not found"})
			return
		}
		c.JSON(500, server_utils.ErrorResponse{Message: "internal error"})
		return
	}
	if passengerWallet.WalletType != models.WalletTypePassenger {
		c.JSON(403, server_utils.ErrorResponse{Message: "only passenger wallets can pay fare"})
		return
	}
	if passengerWallet.Freezed {
		c.JSON(403, server_utils.ErrorResponse{Message: "wallet is frozen"})
		return
	}

	driverWallet, err := h.WalletRepo.GetByID(c.Request.Context(), req.DriverWalletID)
	if err != nil {
		if repository.IsNotFound(err) {
			c.JSON(404, server_utils.ErrorResponse{Message: "driver wallet not found"})
			return
		}
		c.JSON(500, server_utils.ErrorResponse{Message: "internal error"})
		return
	}
	if driverWallet.WalletType != models.WalletTypeDriver {
		c.JSON(400, server_utils.ErrorResponse{Message: "driver_wallet_id must reference a driver wallet"})
		return
	}
	if driverWallet.UserID == passengerWallet.UserID {
		c.JSON(400, server_utils.ErrorResponse{Message: "driver wallet must not belong to the same user"})
		return
	}
	if driverWallet.Freezed {
		c.JSON(403, server_utils.ErrorResponse{Message: "driver wallet is frozen"})
		return
	}

	amountDec := decimal.NewFromFloat(req.Amount)
	subCity := strings.TrimSpace(req.SubCityID)
	assistant := strings.TrimSpace(req.AssistantID)

	ctx := server_utils.WithAuthBearer(c.Request.Context(), c.GetHeader("Authorization"))

	var transferOut payment.TransferResponse
	hook := func(ctx context.Context) error {
		if h.TripClient == nil {
			return errors.New("trip client not configured")
		}
		if err := h.TripClient.ValidateTripActive(ctx, req.TripID); err != nil {
			return err
		}
		out, err := h.PaymentClient.Transfer(ctx, payment.TransferRequest{
			Amount:           req.Amount,
			PayerUserID:      strconv.FormatInt(passengerWallet.UserID, 10),
			SenderWalletID:   strconv.FormatInt(passengerWallet.ID, 10),
			ReceiverWalletID: strconv.FormatInt(driverWallet.ID, 10),
			ReceiverID:       strconv.FormatInt(driverWallet.UserID, 10),
			ReceiverFullName: strings.TrimSpace(req.ReceiverFullName),
			TripID:           strings.TrimSpace(req.TripID),
			SubCityID:        subCity,
			AssistantID:      assistant,
			Message:          strings.TrimSpace(req.Message),
		})
		if err != nil {
			return err
		}
		transferOut = out
		return nil
	}

	if err := h.WalletService.TransferBalanceWithHook(ctx, passengerWallet.ID, driverWallet.ID, amountDec, hook); err != nil {
		switch {
		case errors.Is(err, trip.ErrTripNotActive):
			c.JSON(400, server_utils.ErrorResponse{Message: "trip not found or not active"})
			return
		case errors.Is(err, services.ErrInvalidAmount):
			c.JSON(400, server_utils.ErrorResponse{Message: "amount must be > 0"})
			return
		case errors.Is(err, services.ErrWalletFrozen):
			c.JSON(403, server_utils.ErrorResponse{Message: "wallet is frozen"})
			return
		case errors.Is(err, services.ErrInsufficientFunds):
			c.JSON(400, server_utils.ErrorResponse{Message: "insufficient balance"})
			return
		default:
			c.JSON(502, server_utils.ErrorResponse{Message: err.Error()})
			return
		}
	}

	passAfter, errPass := h.WalletRepo.GetByID(c.Request.Context(), passengerWallet.ID)
	drvAfter, errDrv := h.WalletRepo.GetByID(c.Request.Context(), driverWallet.ID)
	if errPass == nil && errDrv == nil {
		deltaPass := amountDec.Neg().StringFixed(2)
		deltaDrv := amountDec.StringFixed(2)
		balPass := passAfter.Balance.StringFixed(2)
		balDrv := drvAfter.Balance.StringFixed(2)

		fieldsDebit := map[string]any{
			"wallet_id":   passengerWallet.ID,
			"balance":     balPass,
			"delta":       deltaPass,
			"reason":      "fare_debit",
			"sub_city_id": subCity,
		}
		fieldsCredit := map[string]any{
			"wallet_id":   driverWallet.ID,
			"balance":     balDrv,
			"delta":       deltaDrv,
			"reason":      "fare_credit",
			"sub_city_id": subCity,
		}
		if subCity == "" {
			delete(fieldsDebit, "sub_city_id")
			delete(fieldsCredit, "sub_city_id")
		}
		_ = h.Bus.PublishAnalytics(c.Request.Context(), "analytics.wallet.balance_updated", fieldsDebit)
		_ = h.Bus.PublishAnalytics(c.Request.Context(), "analytics.wallet.balance_updated", fieldsCredit)
	}

	amtStr := amountDec.StringFixed(2)
	meta := map[string]any{
		"amount":         amtStr,
		"currency":       "ETB",
		"trip_id":        strings.TrimSpace(req.TripID),
		"transaction_id": transferOut.TransactionID,
	}
	if assistant != "" {
		meta["assistant_id"] = assistant
	}
	_ = h.Bus.PublishNotification(c.Request.Context(), "notification.wallet.pay_fare_succeeded", map[string]any{
		"event_id":  uuid.NewString(),
		"user_id":   strconv.FormatInt(passengerWallet.UserID, 10),
		"user_role": "passenger",
		"type":      "fare_paid",
		"title":     "Fare Paid",
		"content":   "You paid " + amtStr + " ETB for your trip.",
		"priority":  "normal",
		"category":  "billing",
		"channels":  []string{"push"},
		"metadata":  meta,
	})

	c.JSON(200, payFareResponse{
		Success:       true,
		TransactionID: transferOut.TransactionID,
		ReceiptURL:    transferOut.ReceiptURL,
	})
}
