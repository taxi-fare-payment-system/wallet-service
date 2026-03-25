package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"wallet_service/internal/httpx"
	"wallet_service/internal/models"
	"wallet_service/internal/repository"

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

func (h *WalletHandlers) GetWallet(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid wallet id")
		return
	}

	wallet, err := h.WalletRepo.GetByID(r.Context(), id)
	if err != nil {
		if repository.IsNotFound(err) {
			httpx.WriteError(w, http.StatusNotFound, "wallet not found")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, toWalletResponse(wallet))
}

func (h *WalletHandlers) GetWalletByUser(w http.ResponseWriter, r *http.Request) {
	userIDStr := r.PathValue("userId")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil || userID <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	walletTypeRaw := strings.TrimSpace(r.URL.Query().Get("type"))
	if walletTypeRaw == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing wallet type")
		return
	}
	walletType := models.WalletType(walletTypeRaw)
	switch walletType {
	case models.WalletTypePassenger, models.WalletTypeDriver, models.WalletTypeOwner:
	default:
		httpx.WriteError(w, http.StatusBadRequest, "invalid wallet type")
		return
	}

	wallet, err := h.WalletRepo.GetByUserIDAndType(r.Context(), userID, walletType)
	if err != nil {
		if repository.IsNotFound(err) {
			httpx.WriteError(w, http.StatusNotFound, "wallet not found")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, toWalletResponse(wallet))
}

func (h *WalletHandlers) CreateWallet(w http.ResponseWriter, r *http.Request) {
	var req createWalletRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	walletType := models.WalletType(strings.TrimSpace(req.Type))
	switch walletType {
	case models.WalletTypePassenger, models.WalletTypeDriver, models.WalletTypeOwner:
	default:
		httpx.WriteError(w, http.StatusBadRequest, "invalid wallet type")
		return
	}
	if req.UserID <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	newWallet := models.Wallet{
		UserID:     req.UserID,
		WalletType: walletType,
		Freezed:    false,
		Balance:    decimal.Zero,
	}
	if err := h.WalletRepo.Create(r.Context(), &newWallet); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			httpx.WriteError(w, http.StatusConflict, "wallet already exists for user and type")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, toWalletResponse(newWallet))
}
