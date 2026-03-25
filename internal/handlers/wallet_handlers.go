package handlers

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"wallet_service/internal/models"
	"wallet_service/internal/repository"
	"wallet_service/internal/server_utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
)

type WalletHandlers struct {
	WalletRepo *repository.WalletRepository
}

type createWalletRequest struct {
	UserID int64  `json:"user_id"`
	Type   string `json:"type"`
}

type walletResponse struct {
	ID         int64             `json:"id"`
	UserID     int64             `json:"user_id"`
	WalletType models.WalletType `json:"wallet_type"`
	Freezed    bool              `json:"freezed"`
	Balance    decimal.Decimal   `json:"balance"`
	CreatedAt  string            `json:"created_at"`
	UpdatedAt  string            `json:"updated_at"`
}

func toWalletResponse(w models.Wallet) walletResponse {
	return walletResponse{
		ID:         w.ID,
		UserID:     w.UserID,
		WalletType: w.WalletType,
		Freezed:    w.Freezed,
		Balance:    w.Balance,
		CreatedAt:  w.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:  w.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func (h *WalletHandlers) GetWallet(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		c.JSON(400, server_utils.ErrorResponse{Message: "invalid wallet id"})
		return
	}

	wallet, err := h.WalletRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if repository.IsNotFound(err) {
			c.JSON(404, server_utils.ErrorResponse{Message: "wallet not found"})
			return
		}
		c.JSON(500, server_utils.ErrorResponse{Message: "internal error"})
		return
	}

	c.JSON(200, toWalletResponse(wallet))
}

func (h *WalletHandlers) GetWalletByUser(c *gin.Context) {
	userIDStr := c.Param("userId")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil || userID <= 0 {
		c.JSON(400, server_utils.ErrorResponse{Message: "invalid user id"})
		return
	}

	walletTypeRaw := strings.TrimSpace(c.Query("type"))
	if walletTypeRaw == "" {
		c.JSON(400, server_utils.ErrorResponse{Message: "missing wallet type"})
		return
	}
	walletType := models.WalletType(walletTypeRaw)
	switch walletType {
	case models.WalletTypePassenger, models.WalletTypeDriver, models.WalletTypeOwner:
	default:
		c.JSON(400, server_utils.ErrorResponse{Message: "invalid wallet type"})
		return
	}

	wallet, err := h.WalletRepo.GetByUserIDAndType(c.Request.Context(), userID, walletType)
	if err != nil {
		if repository.IsNotFound(err) {
			c.JSON(404, server_utils.ErrorResponse{Message: "wallet not found"})
			return
		}
		c.JSON(500, server_utils.ErrorResponse{Message: "internal error"})
		return
	}

	c.JSON(200, toWalletResponse(wallet))
}

func (h *WalletHandlers) CreateWallet(c *gin.Context) {
	var req createWalletRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, server_utils.ErrorResponse{Message: "invalid json body"})
		return
	}

	walletType := models.WalletType(strings.TrimSpace(req.Type))
	switch walletType {
	case models.WalletTypePassenger, models.WalletTypeDriver, models.WalletTypeOwner:
	default:
		c.JSON(400, server_utils.ErrorResponse{Message: "invalid wallet type"})
		return
	}
	if req.UserID <= 0 {
		c.JSON(400, server_utils.ErrorResponse{Message: "invalid user id"})
		return
	}

	newWallet := models.Wallet{
		UserID:     req.UserID,
		WalletType: walletType,
		Freezed:    false,
		Balance:    decimal.Zero,
	}
	if err := h.WalletRepo.Create(c.Request.Context(), &newWallet); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			c.JSON(409, server_utils.ErrorResponse{Message: "wallet already exists for user and type"})
			return
		}
		c.JSON(500, server_utils.ErrorResponse{Message: "internal error"})
		return
	}

	c.JSON(201, toWalletResponse(newWallet))
}
