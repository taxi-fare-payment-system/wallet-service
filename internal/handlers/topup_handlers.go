package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"wallet_service/internal/httpx"
	"wallet_service/internal/models"
	"wallet_service/internal/payment"
	"wallet_service/internal/repository"
	"wallet_service/internal/services"

	"github.com/shopspring/decimal"
)

type TopupHandlers struct {
	WalletRepo    *repository.WalletRepository
	WalletService *services.WalletService
	PaymentClient *payment.Client
}

type topupRequest struct {
	Amount      float64 `json:"amount"`
	PhoneNumber string  `json:"phone_number"`
	FirstName   string  `json:"first_name"`
	LastName    string  `json:"last_name"`
	Email       string  `json:"email,omitempty"`
	Message     string  `json:"message,omitempty"`
}

type topupResponse struct {
	TransactionID string `json:"transaction_id"`
	CheckoutURL   string `json:"checkout_url"`
}

func (h *TopupHandlers) TopupWallet(w http.ResponseWriter, r *http.Request) {
	walletIDStr := r.PathValue("wallet_id")
	walletID, err := strconv.ParseInt(walletIDStr, 10, 64)
	if err != nil || walletID <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid wallet id")
		return
	}

	var req topupRequest
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
	if strings.TrimSpace(req.PhoneNumber) == "" || strings.TrimSpace(req.FirstName) == "" || strings.TrimSpace(req.LastName) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "phone_number, first_name, and last_name are required")
		return
	}

	wallet, err := h.WalletRepo.GetByID(r.Context(), walletID)
	if err != nil {
		if repository.IsNotFound(err) {
			httpx.WriteError(w, http.StatusNotFound, "wallet not found")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if wallet.Freezed {
		httpx.WriteError(w, http.StatusForbidden, "wallet is frozen")
		return
	}
	if wallet.WalletType != models.WalletTypePassenger {
		httpx.WriteError(w, http.StatusForbidden, "topup is only allowed for passenger wallets")
		return
	}

	pReq := payment.InitiateRequest{
		Amount:         req.Amount,
		Reason:         "wallet topup",
		PayerUserID:    strconv.FormatInt(wallet.UserID, 10),
		SenderWalletID: strconv.FormatInt(wallet.ID, 10),
		PhoneNumber:    strings.TrimSpace(req.PhoneNumber),
		FirstName:      strings.TrimSpace(req.FirstName),
		LastName:       strings.TrimSpace(req.LastName),
		Email:          strings.TrimSpace(req.Email),
		Message:        strings.TrimSpace(req.Message),
		ReceiverID:     "",
		TripID:         "",
	}

	out, err := h.PaymentClient.InitiateTopup(r.Context(), pReq)
	if err != nil {
		httpx.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, topupResponse{
		TransactionID: out.TransactionID,
		CheckoutURL:   out.CheckoutURL,
	})
}

type finalizeTopupRequest struct {
	TransactionID    string `json:"transaction_id"`
	TxRef            string `json:"tx_ref"`
	ChapaReference   string `json:"chapa_reference"`
	PayerUserID      string `json:"payer_user_id"`
	ReceiverWalletID string `json:"receiver_wallet_id"`
	Amount           string `json:"amount"`
	Currency         string `json:"currency"`
}

type finalizeTopupResponse struct {
	Received bool `json:"received"`
}

func (h *TopupHandlers) FinalizeTopup(w http.ResponseWriter, r *http.Request) {
	var req finalizeTopupRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, finalizeTopupResponse{Received: false})
		return
	}

	if strings.TrimSpace(req.TransactionID) == "" || strings.TrimSpace(req.ReceiverWalletID) == "" || strings.TrimSpace(req.Amount) == "" {
		httpx.WriteJSON(w, http.StatusBadRequest, finalizeTopupResponse{Received: false})
		return
	}

	walletID, err := strconv.ParseInt(req.ReceiverWalletID, 10, 64)
	if err != nil || walletID <= 0 {
		httpx.WriteJSON(w, http.StatusBadRequest, finalizeTopupResponse{Received: false})
		return
	}

	amt, err := decimal.NewFromString(req.Amount)
	if err != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, finalizeTopupResponse{Received: false})
		return
	}

	currency := strings.TrimSpace(req.Currency)
	if currency == "" {
		currency = "ETB"
	}
	txRef := strings.TrimSpace(req.TxRef)
	chapaRef := strings.TrimSpace(req.ChapaReference)
	var txRefPtr *string
	if txRef != "" {
		txRefPtr = &txRef
	}
	var chapaPtr *string
	if chapaRef != "" {
		chapaPtr = &chapaRef
	}

	if err := h.WalletService.ApplyTopupIdempotent(
		r.Context(),
		strings.TrimSpace(req.TransactionID),
		walletID,
		amt,
		currency,
		txRefPtr,
		chapaPtr,
	); err != nil {
		// Payment service expects 400/401/500 style responses; here we only distinguish invalid input vs server error.
		httpx.WriteJSON(w, http.StatusInternalServerError, finalizeTopupResponse{Received: false})
		return
	}

	httpx.WriteJSON(w, http.StatusOK, finalizeTopupResponse{Received: true})
}
