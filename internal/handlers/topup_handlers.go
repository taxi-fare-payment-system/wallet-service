package handlers

import (
	"strings"

	"wallet_service/internal/auth"
	"wallet_service/internal/messaging"
	"wallet_service/internal/models"
	"wallet_service/internal/payment"
	"wallet_service/internal/repository"
	"wallet_service/internal/server_utils"
	"wallet_service/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type TopupHandlers struct {
	WalletRepo    *repository.WalletRepository
	WalletService *services.WalletService
	AuthClient    *auth.Client
	PaymentClient *payment.Client
	Bus           *messaging.Publisher
}

type topupRequest struct {
	Amount      float64 `json:"amount"`
	PhoneNumber string  `json:"phone_number,omitempty"`
	Email       string  `json:"email,omitempty"`
	Message     string  `json:"message,omitempty"`
}

type topupResponse struct {
	TransactionID string `json:"transaction_id"`
	CheckoutURL   string `json:"checkout_url"`
}

func (h *TopupHandlers) TopupWallet(c *gin.Context) {
	walletID := strings.TrimSpace(c.Param("wallet_id"))
	if _, err := uuid.Parse(walletID); err != nil {
		c.JSON(400, server_utils.ErrorResponse{Message: "invalid wallet id"})
		return
	}

	var req topupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, server_utils.ErrorResponse{Message: "invalid json body"})
		return
	}
	if req.Amount <= 0 {
		c.JSON(400, server_utils.ErrorResponse{Message: "amount must be > 0"})
		return
	}
	if h.AuthClient == nil {
		c.JSON(401, server_utils.ErrorResponse{Message: "authentication error"})
		return
	}
	authz := strings.TrimSpace(c.GetHeader("Authorization"))
	if authz == "" {
		c.JSON(401, server_utils.ErrorResponse{Message: "authentication error"})
		return
	}
	authCtx := server_utils.WithAuthBearer(c.Request.Context(), authz)
	me, err := h.AuthClient.GetMe(authCtx)
	if err != nil {
		c.JSON(401, server_utils.ErrorResponse{Message: "authentication error"})
		return
	}
	firstName, lastName := splitDisplayName(me.Data.DisplayName)
	if firstName == "" || lastName == "" {
		c.JSON(401, server_utils.ErrorResponse{Message: "authentication error"})
		return
	}
	phone := strings.TrimSpace(req.PhoneNumber)
	if phone == "" {
		phone = strings.TrimSpace(me.Data.Phone)
	}
	if phone == "" {
		c.JSON(401, server_utils.ErrorResponse{Message: "authentication error"})
		return
	}

	wallet, err := h.WalletRepo.GetByID(c.Request.Context(), walletID)
	if err != nil {
		if repository.IsNotFound(err) {
			c.JSON(404, server_utils.ErrorResponse{Message: "wallet not found"})
			return
		}
		c.JSON(500, server_utils.ErrorResponse{Message: "internal error"})
		return
	}
	if wallet.Freezed {
		c.JSON(403, server_utils.ErrorResponse{Message: "wallet is frozen"})
		return
	}
	if wallet.WalletType != models.WalletTypePassenger {
		c.JSON(403, server_utils.ErrorResponse{Message: "topup is only allowed for passenger wallets"})
		return
	}

	pReq := payment.InitiateRequest{
		Amount:         req.Amount,
		Reason:         "wallet topup",
		PayerUserID:    wallet.UserID,
		SenderWalletID: wallet.ID,
		PhoneNumber:    phone,
		FirstName:      firstName,
		LastName:       lastName,
		Email:          strings.TrimSpace(req.Email),
		Message:        strings.TrimSpace(req.Message),
		ReceiverID:     "",
		TripID:         "",
	}

	out, err := h.PaymentClient.InitiateTopup(c.Request.Context(), pReq)
	if err != nil {
		c.JSON(502, server_utils.ErrorResponse{Message: err.Error()})
		return
	}

	c.JSON(200, topupResponse{
		TransactionID: out.TransactionID,
		CheckoutURL:   out.CheckoutURL,
	})
}

func splitDisplayName(displayName string) (string, string) {
	parts := strings.Fields(strings.TrimSpace(displayName))
	if len(parts) == 0 {
		return "", ""
	}
	if len(parts) == 1 {
		return parts[0], parts[0]
	}
	return parts[0], strings.Join(parts[1:], " ")
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

func (h *TopupHandlers) FinalizeTopup(c *gin.Context) {
	var req finalizeTopupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, finalizeTopupResponse{Received: false})
		return
	}

	if strings.TrimSpace(req.TransactionID) == "" || strings.TrimSpace(req.ReceiverWalletID) == "" || strings.TrimSpace(req.Amount) == "" {
		c.JSON(400, finalizeTopupResponse{Received: false})
		return
	}

	walletID := strings.TrimSpace(req.ReceiverWalletID)
	if _, err := uuid.Parse(walletID); err != nil {
		c.JSON(400, finalizeTopupResponse{Received: false})
		return
	}

	amt, err := decimal.NewFromString(req.Amount)
	if err != nil {
		c.JSON(400, finalizeTopupResponse{Received: false})
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

	applied, newBal, err := h.WalletService.ApplyTopupIdempotent(
		c.Request.Context(),
		strings.TrimSpace(req.TransactionID),
		walletID,
		amt,
		currency,
		txRefPtr,
		chapaPtr,
	)
	if err != nil {
		c.JSON(500, finalizeTopupResponse{Received: false})
		return
	}

	if applied {
		txID := strings.TrimSpace(req.TransactionID)
		_ = h.Bus.PublishAnalytics(c.Request.Context(), "analytics.wallet.balance_updated", map[string]any{
			"wallet_id": walletID,
			"balance":   newBal.StringFixed(2),
			"delta":     amt.StringFixed(2),
			"reason":    "topup",
		})
		payer := strings.TrimSpace(req.PayerUserID)
		if payer == "" {
			payer = "unknown"
		}
		amtStr := amt.StringFixed(2)
		_ = h.Bus.PublishNotification(c.Request.Context(), "notification.wallet.topup_succeeded", map[string]any{
			"event_id":  uuid.NewString(),
			"user_id":   payer,
			"user_role": "passenger",
			"type":      "topup_success",
			"title":     "Wallet Topped Up",
			"content":   "Your wallet has been credited " + amtStr + " ETB.",
			"priority":  "normal",
			"category":  "billing",
			"channels":  []string{"sms"},
			"metadata": map[string]any{
				"amount":         amtStr,
				"currency":       currency,
				"transaction_id": txID,
			},
		})
	}

	c.JSON(200, finalizeTopupResponse{Received: true})
}
