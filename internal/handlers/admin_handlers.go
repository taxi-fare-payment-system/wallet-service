package handlers

import (
	"net/http"
	"strconv"

	"wallet_service/internal/auth"
	"wallet_service/internal/models"
	"wallet_service/internal/repository"
	"wallet_service/internal/server_utils"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type AdminHandlers struct {
	WalletRepo *repository.WalletRepository
	AuthClient *auth.Client
}

func (h *AdminHandlers) requireAdmin(c *gin.Context) (adminUserID int64, ok bool) {
	adminUserIDStr := c.GetHeader("X-Admin-User-Id")
	adminUserID, err := strconv.ParseInt(adminUserIDStr, 10, 64)
	if err != nil || adminUserID <= 0 {
		c.JSON(401, server_utils.ErrorResponse{Message: "missing or invalid admin user id"})
		return 0, false
	}
	if h.AuthClient == nil {
		c.JSON(503, server_utils.ErrorResponse{Message: "auth service not configured"})
		return 0, false
	}
	isAdmin, err := h.AuthClient.VerifyAdmin(c.Request.Context(), adminUserID)
	if err != nil {
		c.JSON(502, server_utils.ErrorResponse{Message: "auth service error"})
		return 0, false
	}
	if !isAdmin {
		c.JSON(403, server_utils.ErrorResponse{Message: "admin access required"})
		return 0, false
	}
	return adminUserID, true
}

func (h *AdminHandlers) FreezeWallet(c *gin.Context) {
	walletIDStr := c.Param("wallet_id")
	walletID, err := strconv.ParseInt(walletIDStr, 10, 64)
	if err != nil || walletID <= 0 {
		c.JSON(400, server_utils.ErrorResponse{Message: "invalid wallet id"})
		return
	}
	if _, ok := h.requireAdmin(c); !ok {
		return
	}

	db := h.WalletRepo.DB()
	res := db.WithContext(c.Request.Context()).Model(&models.Wallet{}).Where("id = ?", walletID).Update("freezed", true)
	if res.Error != nil {
		c.JSON(500, server_utils.ErrorResponse{Message: "internal error"})
		return
	}
	if res.RowsAffected == 0 {
		c.JSON(404, server_utils.ErrorResponse{Message: "wallet not found"})
		return
	}
	c.JSON(200, map[string]any{"success": true, "wallet_id": walletID})
}

type findWalletsResponse struct {
	Items  []walletResponse `json:"items"`
	Limit  int              `json:"limit"`
	Offset int              `json:"offset"`
	Sort   string           `json:"sort"`
	Order  string           `json:"order"`
}

// FindWallets returns wallets for admin use with filtering, sorting, and pagination.
//
// Query params:
// - filters: user_id, wallet_type, freezed, min_balance, max_balance
// - sort: id (default), balance, created_at, updated_at
// - order: asc|desc (default desc)
// - limit: default 50, max 200
// - offset: default 0
func (h *AdminHandlers) FindWallets(c *gin.Context) {
	if _, ok := h.requireAdmin(c); !ok {
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

	db := h.WalletRepo.DB().WithContext(c.Request.Context()).Model(&models.Wallet{})

	if v := c.Query("user_id"); v != "" {
		userID, err := strconv.ParseInt(v, 10, 64)
		if err != nil || userID <= 0 {
			c.JSON(400, server_utils.ErrorResponse{Message: "invalid user_id"})
			return
		}
		db = db.Where("user_id = ?", userID)
	}

	if v := c.Query("wallet_type"); v != "" {
		wt := models.WalletType(v)
		switch wt {
		case models.WalletTypePassenger, models.WalletTypeDriver, models.WalletTypeOwner:
			db = db.Where("wallet_type = ?", wt)
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
		db = db.Where("freezed = ?", v == "true")
	}

	if v := c.Query("min_balance"); v != "" {
		minB, err := decimal.NewFromString(v)
		if err != nil {
			c.JSON(400, server_utils.ErrorResponse{Message: "invalid min_balance"})
			return
		}
		db = db.Where("balance >= ?", minB)
	}
	if v := c.Query("max_balance"); v != "" {
		maxB, err := decimal.NewFromString(v)
		if err != nil {
			c.JSON(400, server_utils.ErrorResponse{Message: "invalid max_balance"})
			return
		}
		db = db.Where("balance <= ?", maxB)
	}

	var wallets []models.Wallet
	if err := db.Order(sortCol + " " + order).Limit(limit).Offset(offset).Find(&wallets).Error; err != nil {
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
