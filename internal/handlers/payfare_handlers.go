package handlers

import (
	"context"
	"errors"
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
	ConfigRepo    *repository.ConfigRepository
	PaymentClient *payment.Client
	TripClient    *trip.Client
	Bus           *messaging.Publisher
}

type payFareRequest struct {
	Amount           float64 `json:"amount"`
	DriverWalletID   string  `json:"driver_wallet_id"`
	TripID           string  `json:"trip_id"`
	ReceiverFullName string  `json:"receiver_full_name"`
	SubCityID        *uint   `json:"sub_city_id,omitempty"`
	AssistantID      string  `json:"assistant_id"`
	Message          string  `json:"message,omitempty"`
}

type payFareResponse struct {
	Success       bool    `json:"success"`
	TransactionID string  `json:"transaction_id"`
	ReceiptURL    *string `json:"receipt_url"`
}

func (h *PayFareHandlers) PayFare(c *gin.Context) {
	passengerWalletID := strings.TrimSpace(c.Param("wallet_id"))
	if _, err := uuid.Parse(passengerWalletID); err != nil {
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
	req.DriverWalletID = strings.TrimSpace(req.DriverWalletID)
	if _, err := uuid.Parse(req.DriverWalletID); err != nil {
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

	platformFee, err := parseFarePlatformFee(c.Request.Context(), h.ConfigRepo)
	if err != nil {
		c.JSON(500, server_utils.ErrorResponse{Message: "invalid fare platform fee configuration"})
		return
	}

	systemWallet, err := h.WalletRepo.GetSystemWallet(c.Request.Context())
	if err != nil {
		if repository.IsNotFound(err) {
			c.JSON(500, server_utils.ErrorResponse{Message: "system wallet not configured"})
			return
		}
		c.JSON(500, server_utils.ErrorResponse{Message: "internal error"})
		return
	}

	amountDec := decimal.NewFromFloat(req.Amount)
	assistant := strings.TrimSpace(req.AssistantID)

	ctx := server_utils.WithAuthBearer(c.Request.Context(), c.GetHeader("Authorization"))

	var transferOut payment.TransferResponse
	hook := func(ctx context.Context) error {
		if h.TripClient == nil {
			return errors.New("trip client not configured")
		}
		// if err := h.TripClient.ValidateTripActive(ctx, req.TripID); err != nil {
		// 	return err
		// }
		transferReq := payment.TransferRequest{
			Amount:           req.Amount,
			PayerUserID:      passengerWallet.UserID,
			SenderWalletID:   passengerWallet.ID,
			ReceiverWalletID: driverWallet.ID,
			ReceiverID:       driverWallet.UserID,
			ReceiverFullName: strings.TrimSpace(req.ReceiverFullName),
			TripID:           strings.TrimSpace(req.TripID),
			SubCityID:        req.SubCityID,
			AssistantID:      assistant,
			Message:          strings.TrimSpace(req.Message),
		}
		if platformFee.Cmp(decimal.Zero) > 0 {
			fee := platformFee.InexactFloat64()
			transferReq.PlatformFee = &fee
			transferReq.SystemWalletID = systemWallet.ID
		}
		out, err := h.PaymentClient.Transfer(ctx, transferReq)
		if err != nil {
			return err
		}
		transferOut = out
		return nil
	}

	if err := h.WalletService.TransferFareWithPlatformFee(
		ctx,
		passengerWallet.ID,
		driverWallet.ID,
		systemWallet.ID,
		amountDec,
		platformFee,
		hook,
	); err != nil {
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
		deltaPass := amountDec.Add(platformFee).Neg().StringFixed(2)
		deltaDrv := amountDec.StringFixed(2)
		balPass := passAfter.Balance.StringFixed(2)
		balDrv := drvAfter.Balance.StringFixed(2)

		fieldsDebit := map[string]any{
			"wallet_id": passengerWallet.ID,
			"balance":   balPass,
			"delta":     deltaPass,
			"reason":    "fare_debit",
		}
		fieldsCredit := map[string]any{
			"wallet_id": driverWallet.ID,
			"balance":   balDrv,
			"delta":     deltaDrv,
			"reason":    "fare_credit",
		}
		if req.SubCityID != nil && *req.SubCityID != 0 {
			fieldsDebit["sub_city_id"] = *req.SubCityID
			fieldsCredit["sub_city_id"] = *req.SubCityID
		}
		_ = h.Bus.PublishAnalytics(c.Request.Context(), "analytics.wallet.balance_updated", fieldsDebit)
		_ = h.Bus.PublishAnalytics(c.Request.Context(), "analytics.wallet.balance_updated", fieldsCredit)
		if platformFee.Cmp(decimal.Zero) > 0 {
			sysAfter, errSys := h.WalletRepo.GetByID(c.Request.Context(), systemWallet.ID)
			if errSys == nil {
				_ = h.Bus.PublishAnalytics(c.Request.Context(), "analytics.wallet.balance_updated", map[string]any{
					"wallet_id": systemWallet.ID,
					"balance":   sysAfter.Balance.StringFixed(2),
					"delta":     platformFee.StringFixed(2),
					"reason":    "fare_platform_fee",
				})
			}
		}
	}

	auditMeta := map[string]any{
		"amount":              amountDec.StringFixed(2),
		"total_charged":       amountDec.Add(platformFee).StringFixed(2),
		"currency":            "ETB",
		"trip_id":             strings.TrimSpace(req.TripID),
		"transaction_id":      transferOut.TransactionID,
		"driver_wallet_id":    driverWallet.ID,
		"platform_fee":        platformFee.StringFixed(2),
		"system_wallet_id":    systemWallet.ID,
	}
	if transferOut.PlatformFeeTransactionID != "" {
		auditMeta["platform_fee_transaction_id"] = transferOut.PlatformFeeTransactionID
	}
	if assistant != "" {
		auditMeta["assistant_id"] = assistant
	}
	auditEntry := messaging.AuditEntry{
		Action:        "wallet.fare_paid",
		ActorUserID:   passengerWallet.UserID,
		ActorUserRole: "passenger",
		TargetType:    "wallet",
		TargetID:      passengerWallet.ID,
		Metadata:      auditMeta,
	}
	if req.SubCityID != nil && *req.SubCityID != 0 {
		auditEntry.SubCityID = req.SubCityID
	}
	_ = h.Bus.PublishAuditLog(c.Request.Context(), auditEntry)

	totalPaid := amountDec.Add(platformFee).StringFixed(2)
	amtStr := amountDec.StringFixed(2)
	meta := map[string]any{
		"amount":         amtStr,
		"total_paid":     totalPaid,
		"platform_fee":   platformFee.StringFixed(2),
		"currency":       "ETB",
		"trip_id":        strings.TrimSpace(req.TripID),
		"transaction_id": transferOut.TransactionID,
	}
	if assistant != "" {
		meta["assistant_id"] = assistant
	}
	_ = h.Bus.PublishNotification(c.Request.Context(), "notification.wallet.pay_fare_succeeded", map[string]any{
		"event_id":  uuid.NewString(),
		"user_id":   passengerWallet.UserID,
		"user_role": "passenger",
		"type":      "fare_paid",
		"title":     "Fare Paid",
		"content":   "You paid " + totalPaid + " ETB for your trip.",
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
