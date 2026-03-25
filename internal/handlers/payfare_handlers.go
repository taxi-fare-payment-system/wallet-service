package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"wallet_service/internal/httpx"
	"wallet_service/internal/models"
	"wallet_service/internal/payment"
	"wallet_service/internal/repository"
	"wallet_service/internal/services"
	"wallet_service/internal/trip"

	"github.com/shopspring/decimal"
)

type PayFareHandlers struct {
	WalletRepo    *repository.WalletRepository
	WalletService *services.WalletService
	PaymentClient *payment.Client
	TripClient    *trip.Client
}

type payFareRequest struct {
	Amount           float64 `json:"amount"`
	DriverWalletID   int64   `json:"driver_wallet_id"`
	TripID           string  `json:"trip_id"`
	ReceiverFullName string  `json:"receiver_full_name"`
	Message          string  `json:"message,omitempty"`
}

type payFareResponse struct {
	Success       bool    `json:"success"`
	TransactionID string  `json:"transaction_id"`
	ReceiptURL    *string `json:"receipt_url"`
}

func (h *PayFareHandlers) PayFare(w http.ResponseWriter, r *http.Request) {
	passengerWalletIDStr := r.PathValue("wallet_id")
	passengerWalletID, err := strconv.ParseInt(passengerWalletIDStr, 10, 64)
	if err != nil || passengerWalletID <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid wallet id")
		return
	}

	var req payFareRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.Amount <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "amount must be > 0")
		return
	}
	if req.DriverWalletID <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid driver_wallet_id")
		return
	}
	if strings.TrimSpace(req.TripID) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "trip_id is required")
		return
	}
	if strings.TrimSpace(req.ReceiverFullName) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "receiver_full_name is required")
		return
	}

	passengerWallet, err := h.WalletRepo.GetByID(r.Context(), passengerWalletID)
	if err != nil {
		if repository.IsNotFound(err) {
			httpx.WriteError(w, http.StatusNotFound, "wallet not found")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if passengerWallet.WalletType != models.WalletTypePassenger {
		httpx.WriteError(w, http.StatusForbidden, "only passenger wallets can pay fare")
		return
	}
	if passengerWallet.Freezed {
		httpx.WriteError(w, http.StatusForbidden, "wallet is frozen")
		return
	}

	driverWallet, err := h.WalletRepo.GetByID(r.Context(), req.DriverWalletID)
	if err != nil {
		if repository.IsNotFound(err) {
			httpx.WriteError(w, http.StatusNotFound, "driver wallet not found")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if driverWallet.WalletType != models.WalletTypeDriver {
		httpx.WriteError(w, http.StatusBadRequest, "driver_wallet_id must reference a driver wallet")
		return
	}
	if driverWallet.UserID == passengerWallet.UserID {
		httpx.WriteError(w, http.StatusBadRequest, "driver wallet must not belong to the same user")
		return
	}
	if driverWallet.Freezed {
		httpx.WriteError(w, http.StatusForbidden, "driver wallet is frozen")
		return
	}

	amountDec := decimal.NewFromFloat(req.Amount)

	var transferOut payment.TransferResponse
	hook := func(ctx context.Context) error {
		if h.TripClient == nil {
			return errors.New("trip client not configured")
		}
		if err := h.TripClient.ValidateTripMembership(ctx, req.TripID, passengerWallet.UserID, driverWallet.UserID); err != nil {
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
			Message:          strings.TrimSpace(req.Message),
		})
		if err != nil {
			return err
		}
		transferOut = out
		return nil
	}

	if err := h.WalletService.TransferBalanceWithHook(r.Context(), passengerWallet.ID, driverWallet.ID, amountDec, hook); err != nil {
		switch err {
		case services.ErrInvalidAmount:
			httpx.WriteError(w, http.StatusBadRequest, "amount must be > 0")
			return
		case services.ErrWalletFrozen:
			httpx.WriteError(w, http.StatusForbidden, "wallet is frozen")
			return
		case services.ErrInsufficientFunds:
			httpx.WriteError(w, http.StatusBadRequest, "insufficient balance")
			return
		default:
			httpx.WriteError(w, http.StatusBadGateway, err.Error())
			return
		}
	}

	httpx.WriteJSON(w, http.StatusOK, payFareResponse{
		Success:       true,
		TransactionID: transferOut.TransactionID,
		ReceiptURL:    transferOut.ReceiptURL,
	})
}
