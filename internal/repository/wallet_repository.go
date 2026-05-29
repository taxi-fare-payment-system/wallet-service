package repository

import (
	"context"
	"errors"

	"wallet_service/internal/models"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrNotFound = gorm.ErrRecordNotFound

type WalletRepository struct {
	db *gorm.DB
}

func NewWalletRepository(db *gorm.DB) *WalletRepository {
	return &WalletRepository{db: db}
}

func (r *WalletRepository) DB() *gorm.DB {
	return r.db
}

func (r *WalletRepository) GetByID(ctx context.Context, id string) (models.Wallet, error) {
	var w models.Wallet
	err := r.db.WithContext(ctx).First(&w, "id = ?", id).Error
	return w, err
}

func (r *WalletRepository) GetByUserID(ctx context.Context, userID string) (models.Wallet, error) {
	var w models.Wallet
	err := r.db.WithContext(ctx).First(&w, "user_id = ?", userID).Error
	return w, err
}

func (r *WalletRepository) GetByUserIDAndType(ctx context.Context, userID string, walletType models.WalletType) (models.Wallet, error) {
	var w models.Wallet
	err := r.db.WithContext(ctx).First(&w, "user_id = ? AND wallet_type = ?", userID, walletType).Error
	return w, err
}

func (r *WalletRepository) GetSystemWallet(ctx context.Context) (models.Wallet, error) {
	return r.GetByUserIDAndType(ctx, models.SystemWalletUserID, models.WalletTypeSystem)
}

// EnsureSystemWallet creates the platform system wallet if it does not exist.
func (r *WalletRepository) EnsureSystemWallet(ctx context.Context) (models.Wallet, error) {
	w, err := r.GetSystemWallet(ctx)
	if err == nil {
		return w, nil
	}
	if !IsNotFound(err) {
		return w, err
	}
	w = models.Wallet{
		UserID:     models.SystemWalletUserID,
		WalletType: models.WalletTypeSystem,
		Freezed:    false,
		Balance:    decimal.Zero,
	}
	if err := r.Create(ctx, &w); err != nil {
		if w2, err2 := r.GetSystemWallet(ctx); err2 == nil {
			return w2, nil
		}
		return w, err
	}
	return w, nil
}

func (r *WalletRepository) Create(ctx context.Context, w *models.Wallet) error {
	return r.db.WithContext(ctx).Create(w).Error
}

func (r *WalletRepository) LockByID(ctx context.Context, tx *gorm.DB, id string) (models.Wallet, error) {
	if tx == nil {
		tx = r.db
	}
	var w models.Wallet
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&w, "id = ?", id).Error
	return w, err
}

// CreateTopupCreditIfNotExists tries to persist the payment service transaction id.
// If the row already exists, created=false and err=nil.
func (r *WalletRepository) CreateTopupCreditIfNotExists(
	ctx context.Context,
	tx *gorm.DB,
	credit *models.WalletTopupCredit,
) (created bool, err error) {
	if tx == nil {
		tx = r.db
	}
	res := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(credit)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}
