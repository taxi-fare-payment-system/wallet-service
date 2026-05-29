package handlers

import (
	"errors"
	"log/slog"
	"net/url"
	"strconv"
	"strings"

	"wallet_service/internal/auth"
	"wallet_service/internal/models"
	"wallet_service/internal/payment"
	"wallet_service/internal/repository"
	"wallet_service/internal/server_utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type TransactionsHandlers struct {
	Logger        *slog.Logger
	PaymentClient *payment.Client
	WalletRepo    *repository.WalletRepository
	AuthClient    *auth.Client
}

var allowedTransactionQueryParams = map[string]bool{
	"reason":             true,
	"status":             true,
	"sender_wallet_id":   true,
	"receiver_wallet_id": true,
	"sort":               true,
	"order":              true,
	"limit":              true,
	"offset":             true,
}

var forbiddenTransactionQueryParams = map[string]bool{
	"payer_user_id": true,
	"trip_id":       true,
	"wallet_id":     true,
}

var errNoWalletForRole = errors.New("no wallet for role")

func (h *TransactionsHandlers) ListTransactions(c *gin.Context) {
	callerID, ok := server_utils.ParseXUserID(c)
	if !ok {
		c.JSON(401, server_utils.ErrorResponse{Message: "missing X-User-ID"})
		return
	}

	q := c.Request.URL.Query()

	for key := range q {
		if forbiddenTransactionQueryParams[key] {
			c.JSON(400, server_utils.ErrorResponse{Message: "query param not supported: " + key})
			return
		}
		if !allowedTransactionQueryParams[key] {
			c.JSON(400, server_utils.ErrorResponse{Message: "unknown query param: " + key})
			return
		}
	}

	sender := strings.TrimSpace(q.Get("sender_wallet_id"))
	receiver := strings.TrimSpace(q.Get("receiver_wallet_id"))

	if !h.walletOwnedByUser(c, callerID, sender) || !h.walletOwnedByUser(c, callerID, receiver) {
		c.JSON(403, server_utils.ErrorResponse{Message: "forbidden"})
		return
	}

	proxyQ := url.Values{}
	for key, vals := range q {
		proxyQ[key] = append([]string(nil), vals...)
	}

	if sender == "" && receiver == "" && !server_utils.IsPlatformAdminRole(server_utils.XUserRole(c)) {
		walletID, err := h.resolveCallerWalletID(c, callerID)
		if err != nil {
			if repository.IsNotFound(err) {
				c.JSON(404, server_utils.ErrorResponse{Message: "wallet not found"})
				return
			}
			if errors.Is(err, errNoWalletForRole) {
				c.JSON(403, server_utils.ErrorResponse{Message: "forbidden"})
				return
			}
			c.JSON(500, server_utils.ErrorResponse{Message: "internal error"})
			return
		}
		proxyQ.Set("wallet_id", walletID)
	}

	if lim := q.Get("limit"); lim != "" {
		n, err := strconv.Atoi(lim)
		if err != nil || n < 0 || n > 200 {
			c.JSON(400, server_utils.ErrorResponse{Message: "invalid limit"})
			return
		}
	}
	if off := q.Get("offset"); off != "" {
		n, err := strconv.Atoi(off)
		if err != nil || n < 0 {
			c.JSON(400, server_utils.ErrorResponse{Message: "invalid offset"})
			return
		}
	}

	ctx := server_utils.WithTrustUserID(c.Request.Context(), callerID)
	ctx = server_utils.WithTrustUserRole(ctx, server_utils.XUserRole(c))
	out, err := h.PaymentClient.ListTransactions(ctx, proxyQ)
	if err != nil {
		if h.Logger != nil {
			logAttrs := []any{
				slog.String("request_id", server_utils.RequestIDFromContext(c.Request.Context())),
				slog.String("caller_user_id", callerID),
				slog.Any("error", err),
			}
			var apiErr *payment.APIError
			if errors.As(err, &apiErr) {
				logAttrs = append(logAttrs, slog.Int("payment_status", apiErr.StatusCode))
			}
			h.Logger.Error("payment_list_transactions_failed", logAttrs...)
		}
		c.JSON(502, server_utils.ErrorResponse{Message: err.Error()})
		return
	}
	c.JSON(200, out)
}

func (h *TransactionsHandlers) resolveCallerWalletID(c *gin.Context, userID string) (string, error) {
	role := server_utils.XUserRole(c)
	walletType, ok := walletTypeForRole(role)
	if !ok {
		return "", errNoWalletForRole
	}
	ownerID := userID
	if strings.ToLower(strings.TrimSpace(role)) == "driver-assistant" {
		if h.AuthClient == nil {
			return "", errors.New("auth client not configured")
		}
		driverID, err := h.AuthClient.GetDriverByAssistant(c.Request.Context(), userID)
		if err != nil {
			return "", err
		}
		ownerID = driverID
	}
	w, err := h.WalletRepo.GetByUserIDAndType(c.Request.Context(), ownerID, walletType)
	if err != nil {
		return "", err
	}
	return w.ID, nil
}

func walletTypeForRole(role string) (models.WalletType, bool) {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "passenger":
		return models.WalletTypePassenger, true
	case "driver":
		return models.WalletTypeDriver, true
	case "owner":
		return models.WalletTypeOwner, true
	case "driver-assistant":
		return models.WalletTypeDriver, true
	default:
		return "", false
	}
}

func (h *TransactionsHandlers) walletOwnedByUser(c *gin.Context, callerUserID string, walletIDStr string) bool {
	if walletIDStr == "" {
		return true
	}
	if _, err := uuid.Parse(walletIDStr); err != nil {
		return false
	}
	w, err := h.WalletRepo.GetByID(c.Request.Context(), walletIDStr)
	if err != nil {
		return false
	}
	if w.UserID == callerUserID {
		return true
	}
	// Allow driver-assistant to access the driver's wallet
	role := strings.ToLower(strings.TrimSpace(server_utils.XUserRole(c)))
	if role == "driver-assistant" && h.AuthClient != nil {
		driverID, err := h.AuthClient.GetDriverByAssistant(c.Request.Context(), callerUserID)
		if err != nil {
			return false
		}
		return w.UserID == driverID
	}
	return false
}
