package handlers

import (
	"net/url"
	"strconv"

	"wallet_service/internal/payment"
	"wallet_service/internal/server_utils"

	"github.com/gin-gonic/gin"
)

type TransactionsHandlers struct {
	PaymentClient *payment.Client
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

	// Optional local validation (payment service also validates)
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
