package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"wallet_service/internal/httpx"
	"wallet_service/internal/models"
	"wallet_service/internal/repository"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type WithdrawDeleteHandlers struct {
	WalletRepo *repository.WalletRepository
}

type withdrawRequest struct {
	Amount float64 `json:"amount"`
}

func (h *WithdrawDeleteHandlers) Withdraw(w http.ResponseWriter, r *http.Request) {
	walletIDStr := r.PathValue("wallet_id")
	walletID, err := strconv.ParseInt(walletIDStr, 10, 64)
	if err != nil || walletID <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid wallet id")
		return
	}

	var req withdrawRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.Amount <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "amount must be > 0")
		return
	}

	db := h.WalletRepo.DB()
	err = db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		wlt, err := h.WalletRepo.LockByID(r.Context(), tx, walletID)
		if err != nil {
			return err
		}
		if wlt.Freezed {
			return errors.New("wallet is frozen")
		}
		if wlt.WalletType != models.WalletTypeDriver && wlt.WalletType != models.WalletTypeOwner {
			return errors.New("withdraw not allowed for this wallet type")
		}

		amt := decimal.NewFromFloat(req.Amount)
		if wlt.Balance.Cmp(amt) < 0 {
			return errors.New("insufficient funds")
		}
		if wlt.WalletType == models.WalletTypeOwner {
			remaining := wlt.Balance.Sub(amt)
			if remaining.Cmp(decimal.NewFromInt(100)) < 0 {
				return errors.New("owner wallet must keep minimum balance of 100 ETB")
			}
		}

		wlt.Balance = wlt.Balance.Sub(amt)
		return tx.Save(&wlt).Error
	})
	if err != nil {
		if repository.IsNotFound(err) {
			httpx.WriteError(w, http.StatusNotFound, "wallet not found")
			return
		}
		if err.Error() == "wallet is frozen" {
			httpx.WriteError(w, http.StatusForbidden, "wallet is frozen")
			return
		}
		if err.Error() == "insufficient funds" {
			httpx.WriteError(w, http.StatusBadRequest, "insufficient balance")
			return
		}
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"status": "accepted"})
}

func (h *WithdrawDeleteHandlers) DeleteWallet(w http.ResponseWriter, r *http.Request) {
	walletIDStr := r.PathValue("wallet_id")
	walletID, err := strconv.ParseInt(walletIDStr, 10, 64)
	if err != nil || walletID <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid wallet id")
		return
	}

	db := h.WalletRepo.DB()
	err = db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		wlt, err := h.WalletRepo.LockByID(r.Context(), tx, walletID)
		if err != nil {
			return err
		}
		if !wlt.Balance.Equal(decimal.Zero) {
			return errors.New("wallet balance must be zero")
		}
		return tx.Delete(&wlt).Error
	})
	if err != nil {
		if repository.IsNotFound(err) {
			httpx.WriteError(w, http.StatusNotFound, "wallet not found")
			return
		}
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
