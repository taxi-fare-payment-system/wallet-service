package handlers

import (
	"net/http"
	"net/url"
	"strconv"

	"wallet_service/internal/httpx"
	"wallet_service/internal/payment"
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

func (h *TransactionsHandlers) ListTransactions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	for key := range q {
		if forbiddenTransactionQueryParams[key] {
			httpx.WriteError(w, http.StatusBadRequest, "query param not supported: "+key)
			return
		}
		if !allowedTransactionQueryParams[key] {
			httpx.WriteError(w, http.StatusBadRequest, "unknown query param: "+key)
			return
		}
	}

	// Optional local validation (payment service also validates)
	if lim := q.Get("limit"); lim != "" {
		n, err := strconv.Atoi(lim)
		if err != nil || n < 0 || n > 200 {
			httpx.WriteError(w, http.StatusBadRequest, "invalid limit")
			return
		}
	}
	if off := q.Get("offset"); off != "" {
		n, err := strconv.Atoi(off)
		if err != nil || n < 0 {
			httpx.WriteError(w, http.StatusBadRequest, "invalid offset")
			return
		}
	}

	out, err := h.PaymentClient.ListTransactions(r.Context(), url.Values(q))
	if err != nil {
		httpx.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
