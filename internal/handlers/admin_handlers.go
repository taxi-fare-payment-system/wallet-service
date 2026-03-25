package handlers

import (
	"strconv"

	"wallet_service/internal/auth"
	"wallet_service/internal/models"
	"wallet_service/internal/repository"
	"wallet_service/internal/server_utils"

	"github.com/gin-gonic/gin"
)

type AdminHandlers struct {
	WalletRepo *repository.WalletRepository
	AuthClient *auth.Client
}

func (h *AdminHandlers) FreezeWallet(c *gin.Context) {
	walletIDStr := c.Param("wallet_id")
	walletID, err := strconv.ParseInt(walletIDStr, 10, 64)
	if err != nil || walletID <= 0 {
		c.JSON(400, server_utils.ErrorResponse{Message: "invalid wallet id"})
		return
	}
	adminUserIDStr := c.GetHeader("X-Admin-User-Id")
	adminUserID, err := strconv.ParseInt(adminUserIDStr, 10, 64)
	if err != nil || adminUserID <= 0 {
		c.JSON(401, server_utils.ErrorResponse{Message: "missing or invalid admin user id"})
		return
	}
	if h.AuthClient == nil {
		c.JSON(503, server_utils.ErrorResponse{Message: "auth service not configured"})
		return
	}
	isAdmin, err := h.AuthClient.VerifyAdmin(c.Request.Context(), adminUserID)
	if err != nil {
		c.JSON(502, server_utils.ErrorResponse{Message: "auth service error"})
		return
	}
	if !isAdmin {
		c.JSON(403, server_utils.ErrorResponse{Message: "admin access required"})
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
