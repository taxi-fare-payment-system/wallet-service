package repository

import (
	"context"

	"wallet_service/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type WithdrawalRepository struct {
	db *gorm.DB
}

func NewWithdrawalRepository(db *gorm.DB) *WithdrawalRepository {
	return &WithdrawalRepository{db: db}
}

func (r *WithdrawalRepository) DB() *gorm.DB {
	return r.db
}

func (r *WithdrawalRepository) Create(ctx context.Context, tx *gorm.DB, w *models.Withdrawal) error {
	conn := r.db
	if tx != nil {
		conn = tx
	}
	return conn.WithContext(ctx).Create(w).Error
}

func (r *WithdrawalRepository) ListByWalletID(ctx context.Context, walletID uuid.UUID, limit, offset int) ([]models.Withdrawal, error) {
	var withdrawals []models.Withdrawal
	q := r.db.WithContext(ctx).Where("wallet_id = ?", walletID).Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	err := q.Find(&withdrawals).Error
	return withdrawals, err
}

func (r *WithdrawalRepository) GetByID(ctx context.Context, id uuid.UUID) (models.Withdrawal, error) {
	var w models.Withdrawal
	err := r.db.WithContext(ctx).First(&w, "id = ?", id).Error
	return w, err
}

func (r *WithdrawalRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status models.WithdrawalStatus, txRef *string) error {
	updates := map[string]interface{}{
		"status": status,
	}
	if txRef != nil {
		updates["transaction_ref"] = *txRef
	}
	return r.db.WithContext(ctx).Model(&models.Withdrawal{}).Where("id = ?", id).Updates(updates).Error
}

func (r *WithdrawalRepository) GetTotalWithdrawnToday(ctx context.Context, walletID uuid.UUID) (float64, error) {
	var total float64
	err := r.db.WithContext(ctx).
		Model(&models.Withdrawal{}).
		Where("wallet_id = ? AND status != ? AND created_at >= CURRENT_DATE", walletID, models.WithdrawalStatusFailed).
		Select("COALESCE(SUM(amount), 0)").
		Row().
		Scan(&total)
	return total, err
}
