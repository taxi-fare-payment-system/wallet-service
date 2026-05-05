package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"wallet_service/internal/messaging"
	"wallet_service/internal/models"
	"wallet_service/internal/repository"
	"wallet_service/internal/server_utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type AdminHandlers struct {
	WalletRepo *repository.WalletRepository
	Bus        *messaging.Publisher
}

func (h *AdminHandlers) requireTrustedAdmin(c *gin.Context) (actorUserID int64, ok bool) {
	role := server_utils.XUserRole(c)
	if !server_utils.IsPlatformAdminRole(role) {
		c.JSON(403, server_utils.ErrorResponse{Message: "admin access required"})
		return 0, false
	}
	actorUserID, ok = server_utils.ParseXUserID(c)
	if !ok {
		c.JSON(401, server_utils.ErrorResponse{Message: "missing or invalid X-User-ID"})
		return 0, false
	}
	return actorUserID, true
}

func (h *AdminHandlers) FreezeWallet(c *gin.Context) {
	walletIDStr := c.Param("wallet_id")
	walletID, err := strconv.ParseInt(walletIDStr, 10, 64)
	if err != nil || walletID <= 0 {
		c.JSON(400, server_utils.ErrorResponse{Message: "invalid wallet id"})
		return
	}
	if _, ok := h.requireTrustedAdmin(c); !ok {
		return
	}

	var w models.Wallet
	db := h.WalletRepo.DB()
	if err := db.WithContext(c.Request.Context()).First(&w, "id = ?", walletID).Error; err != nil {
		if repository.IsNotFound(err) {
			c.JSON(404, server_utils.ErrorResponse{Message: "wallet not found"})
			return
		}
		c.JSON(500, server_utils.ErrorResponse{Message: "internal error"})
		return
	}

	res := db.WithContext(c.Request.Context()).Model(&models.Wallet{}).Where("id = ?", walletID).Update("freezed", true)
	if res.Error != nil {
		c.JSON(500, server_utils.ErrorResponse{Message: "internal error"})
		return
	}
	if res.RowsAffected == 0 {
		c.JSON(404, server_utils.ErrorResponse{Message: "wallet not found"})
		return
	}

	_ = h.Bus.PublishNotification(c.Request.Context(), "notification.wallet.frozen", map[string]any{
		"event_id":  uuid.NewString(),
		"user_id":   strconv.FormatInt(w.UserID, 10),
		"user_role": string(w.WalletType),
		"type":      "wallet_frozen",
		"title":     "Wallet Frozen",
		"content":   "Your wallet has been frozen by an admin. Contact support for details.",
		"priority":  "high",
		"category":  "account",
		"channels":  []string{"sms"},
	})

	c.JSON(200, map[string]any{"success": true, "wallet_id": walletID})
}

type findWalletsResponse struct {
	Items  []walletResponse `json:"items"`
	Limit  int              `json:"limit"`
	Offset int              `json:"offset"`
	Sort   string           `json:"sort"`
	Order  string           `json:"order"`
}

func (h *AdminHandlers) FindWallets(c *gin.Context) {
	if !server_utils.IsPlatformAdminRole(server_utils.XUserRole(c)) {
		c.JSON(403, server_utils.ErrorResponse{Message: "admin access required"})
		return
	}
	roleLower := strings.ToLower(server_utils.XUserRole(c))
	if _, ok := server_utils.ParseXUserID(c); !ok {
		c.JSON(401, server_utils.ErrorResponse{Message: "missing or invalid X-User-ID"})
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

	sort := c.DefaultQuery("sort", "id")
	order := c.DefaultQuery("order", "desc")
	if order != "asc" && order != "desc" {
		c.JSON(400, server_utils.ErrorResponse{Message: "invalid order"})
		return
	}
	sortCol := map[string]string{
		"id":         "id",
		"balance":    "balance",
		"created_at": "created_at",
		"updated_at": "updated_at",
	}[sort]
	if sortCol == "" {
		c.JSON(400, server_utils.ErrorResponse{Message: "invalid sort"})
		return
	}

	qdb := h.WalletRepo.DB().WithContext(c.Request.Context()).Model(&models.Wallet{})

	if roleLower == "admin" {
		sub := server_utils.XSubCity(c)
		if sub == "" {
			c.JSON(400, server_utils.ErrorResponse{Message: "missing X-Sub-City"})
			return
		}
		qdb = qdb.Where("sub_city_id = ?", sub)
	}

	if v := c.Query("user_id"); v != "" {
		userID, err := strconv.ParseInt(v, 10, 64)
		if err != nil || userID <= 0 {
			c.JSON(400, server_utils.ErrorResponse{Message: "invalid user_id"})
			return
		}
		qdb = qdb.Where("user_id = ?", userID)
	}

	if v := c.Query("wallet_type"); v != "" {
		wt := models.WalletType(v)
		switch wt {
		case models.WalletTypePassenger, models.WalletTypeDriver, models.WalletTypeOwner:
			qdb = qdb.Where("wallet_type = ?", wt)
		default:
			c.JSON(400, server_utils.ErrorResponse{Message: "invalid wallet_type"})
			return
		}
	}

	if v := c.Query("freezed"); v != "" {
		if v != "true" && v != "false" {
			c.JSON(400, server_utils.ErrorResponse{Message: "invalid freezed"})
			return
		}
		qdb = qdb.Where("freezed = ?", v == "true")
	}

	if v := c.Query("min_balance"); v != "" {
		minB, err := decimal.NewFromString(v)
		if err != nil {
			c.JSON(400, server_utils.ErrorResponse{Message: "invalid min_balance"})
			return
		}
		qdb = qdb.Where("balance >= ?", minB)
	}
	if v := c.Query("max_balance"); v != "" {
		maxB, err := decimal.NewFromString(v)
		if err != nil {
			c.JSON(400, server_utils.ErrorResponse{Message: "invalid max_balance"})
			return
		}
		qdb = qdb.Where("balance <= ?", maxB)
	}

	var wallets []models.Wallet
	if err := qdb.Order(sortCol + " " + order).Limit(limit).Offset(offset).Find(&wallets).Error; err != nil {
		c.JSON(http.StatusInternalServerError, server_utils.ErrorResponse{Message: "internal error"})
		return
	}
	items := make([]walletResponse, 0, len(wallets))
	for _, w := range wallets {
		items = append(items, toWalletResponse(w))
	}
	c.JSON(200, findWalletsResponse{
		Items:  items,
		Limit:  limit,
		Offset: offset,
		Sort:   sort,
		Order:  order,
	})
}
