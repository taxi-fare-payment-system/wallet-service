package handlers

import (
	"net/http"
	"strconv"

	"wallet_service/internal/auth"
	"wallet_service/internal/httpx"
	"wallet_service/internal/models"
	"wallet_service/internal/repository"
)

type AdminHandlers struct {
	WalletRepo *repository.WalletRepository
	AuthClient *auth.Client
}

func (h *AdminHandlers) FreezeWallet(w http.ResponseWriter, r *http.Request) {
	walletIDStr := r.PathValue("wallet_id")
	walletID, err := strconv.ParseInt(walletIDStr, 10, 64)
	if err != nil || walletID <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid wallet id")
		return
	}
	adminUserIDStr := r.Header.Get("X-Admin-User-Id")
	adminUserID, err := strconv.ParseInt(adminUserIDStr, 10, 64)
	if err != nil || adminUserID <= 0 {
		httpx.WriteError(w, http.StatusUnauthorized, "missing or invalid admin user id")
		return
	}
	if h.AuthClient == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "auth service not configured")
		return
	}
	isAdmin, err := h.AuthClient.VerifyAdmin(r.Context(), adminUserID)
	if err != nil {
		httpx.WriteError(w, http.StatusBadGateway, "auth service error")
		return
	}
	if !isAdmin {
		httpx.WriteError(w, http.StatusForbidden, "admin access required")
		return
	}

	db := h.WalletRepo.DB()
	res := db.WithContext(r.Context()).Model(&models.Wallet{}).Where("id = ?", walletID).Update("freezed", true)
	if res.Error != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if res.RowsAffected == 0 {
		httpx.WriteError(w, http.StatusNotFound, "wallet not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"success": true, "wallet_id": walletID})
}
