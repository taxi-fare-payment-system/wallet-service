package handlers

import (
	"net/url"
	"strconv"

	"wallet_service/internal/payment"
	"wallet_service/internal/repository"
	"wallet_service/internal/server_utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type TransactionsHandlers struct {
	PaymentClient *payment.Client
	WalletRepo    *repository.WalletRepository
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
}

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

	sender := q.Get("sender_wallet_id")
	receiver := q.Get("receiver_wallet_id")
	if sender == "" && receiver == "" {
		c.JSON(400, server_utils.ErrorResponse{Message: "sender_wallet_id or receiver_wallet_id required"})
		return
	}

	if !h.walletOwnedByUser(c, callerID, sender) || !h.walletOwnedByUser(c, callerID, receiver) {
		c.JSON(403, server_utils.ErrorResponse{Message: "forbidden"})
		return
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

	out, err := h.PaymentClient.ListTransactions(c.Request.Context(), url.Values(q))
	if err != nil {
		c.JSON(502, server_utils.ErrorResponse{Message: err.Error()})
		return
	}
	c.JSON(200, out)
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
	return w.UserID == callerUserID
}
