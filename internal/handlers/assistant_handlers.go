package handlers

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"wallet_service/internal/payment"
	"wallet_service/internal/server_utils"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type AssistantHandlers struct {
	PaymentClient *payment.Client
}

type assistantEarningsItem struct {
	TransactionID string `json:"transaction_id"`
	Amount        string `json:"amount"`
	TripID        string `json:"trip_id"`
	CreatedAt     string `json:"created_at"`
}

type assistantEarningsResponse struct {
	AssistantID      string                  `json:"assistant_id"`
	Date             string                  `json:"date"`
	TotalAmount      float64                 `json:"total_amount"`
	TransactionCount int                     `json:"transaction_count"`
	Items            []assistantEarningsItem `json:"items"`
}

func (h *AssistantHandlers) ListEarnings(c *gin.Context) {
	assistantID := strings.TrimSpace(c.Param("assistantId"))
	if assistantID == "" {
		c.JSON(400, server_utils.ErrorResponse{Message: "invalid assistant id"})
		return
	}

	callerID, hasCaller := server_utils.ParseXUserID(c)
	if !server_utils.IsPlatformAdminRole(server_utils.XUserRole(c)) {
		if !hasCaller || strconv.FormatInt(callerID, 10) != assistantID {
			c.JSON(403, server_utils.ErrorResponse{Message: "forbidden"})
			return
		}
	}

	dateStr := strings.TrimSpace(c.DefaultQuery("date", time.Now().UTC().Format("2006-01-02")))
	if _, err := time.Parse("2006-01-02", dateStr); err != nil {
		c.JSON(400, server_utils.ErrorResponse{Message: "invalid date"})
		return
	}

	limit := 50
	offset := 0
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 200 {
			c.JSON(400, server_utils.ErrorResponse{Message: "invalid limit"})
			return
		}
		limit = n
	}
	if v := c.Query("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			c.JSON(400, server_utils.ErrorResponse{Message: "invalid offset"})
			return
		}
		offset = n
	}

	q := url.Values{}
	q.Set("assistant_id", assistantID)
	q.Set("reason", "fare")
	q.Set("date", dateStr)
	q.Set("limit", strconv.Itoa(limit))
	q.Set("offset", strconv.Itoa(offset))

	list, err := h.PaymentClient.ListTransactions(c.Request.Context(), q)
	if err != nil {
		c.JSON(502, server_utils.ErrorResponse{Message: err.Error()})
		return
	}

	items := make([]assistantEarningsItem, 0, len(list.Items))
	total := decimal.Zero
	for _, raw := range list.Items {
		it := assistantEarningsItem{}
		if v, ok := raw["transaction_id"].(string); ok {
			it.TransactionID = v
		} else if v, ok := raw["id"].(string); ok {
			it.TransactionID = v
		}
		if v, ok := raw["trip_id"].(string); ok {
			it.TripID = v
		}
		amtStr := ""
		switch v := raw["amount"].(type) {
		case string:
			amtStr = v
		case float64:
			amtStr = decimal.NewFromFloat(v).StringFixed(2)
		default:
			if v != nil {
				amtStr = strings.TrimSpace(toString(v))
			}
		}
		it.Amount = amtStr
		if v, ok := raw["created_at"].(string); ok {
			it.CreatedAt = v
		}
		if amtStr != "" {
			if d, err := decimal.NewFromString(amtStr); err == nil {
				total = total.Add(d)
			}
		}
		items = append(items, it)
	}

	totalF, _ := total.Float64()
	c.JSON(200, assistantEarningsResponse{
		AssistantID:      assistantID,
		Date:             dateStr,
		TotalAmount:      totalF,
		TransactionCount: len(items),
		Items:            items,
	})
}

func toString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	default:
		return ""
	}
}
