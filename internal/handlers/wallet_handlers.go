package handlers

import (
	"errors"
	"strings"
	"time"

	"wallet_service/internal/messaging"
	"wallet_service/internal/models"
	"wallet_service/internal/repository"
	"wallet_service/internal/server_utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
)

type WalletHandlers struct {
	WalletRepo *repository.WalletRepository
	Bus        *messaging.Publisher
}

type createWalletRequest struct {
	UserID string `json:"user_id"`
	Type   string `json:"type"`
}

type walletResponse struct {
	ID         string            `json:"id"`
	UserID     string            `json:"user_id"`
	WalletType models.WalletType `json:"wallet_type"`
	Freezed    bool              `json:"freezed"`
	Balance    decimal.Decimal   `json:"balance"`
	CreatedAt  string            `json:"created_at"`
	UpdatedAt  string            `json:"updated_at"`
}

type walletIDOnlyResponse struct {
	WalletID string `json:"wallet_id"`
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
	id := strings.TrimSpace(c.Param("id"))
	if _, err := uuid.Parse(id); err != nil {
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
	userID := strings.TrimSpace(c.Param("userId"))
	if userID == "" {
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

	callerID, hasCaller := server_utils.ParseXUserID(c)
	role := strings.ToLower(server_utils.XUserRole(c))
	if hasCaller && callerID == userID {
		c.JSON(200, toWalletResponse(wallet))
		return
	}
	if server_utils.IsPlatformAdminRole(role) {
		c.JSON(200, toWalletResponse(wallet))
		return
	}
	if role == "passenger" && walletType == models.WalletTypeDriver {
		c.JSON(200, walletIDOnlyResponse{WalletID: wallet.ID})
		return
	}

	c.JSON(403, server_utils.ErrorResponse{Message: "forbidden"})
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
	req.UserID = strings.TrimSpace(req.UserID)
	if req.UserID == "" {
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

	_ = h.Bus.PublishAnalytics(c.Request.Context(), "analytics.wallet.created", map[string]any{
		"wallet_id":   newWallet.ID,
		"user_id":     newWallet.UserID,
		"wallet_type": string(newWallet.WalletType),
		"balance":     0,
	})

	c.JSON(201, toWalletResponse(newWallet))
}
