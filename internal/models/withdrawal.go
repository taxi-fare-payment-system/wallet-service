package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type WithdrawalStatus string

const (
	WithdrawalStatusPending   WithdrawalStatus = "pending"
	WithdrawalStatusCompleted WithdrawalStatus = "completed"
	WithdrawalStatusFailed    WithdrawalStatus = "failed"
)

type Withdrawal struct {
	ID             uuid.UUID        `json:"id" gorm:"type:uuid;primaryKey"`
	WalletID       uuid.UUID        `json:"wallet_id" gorm:"type:uuid;not null;index"`
	Amount         decimal.Decimal  `json:"amount" gorm:"type:numeric(12,2);not null"`
	Fee            decimal.Decimal  `json:"fee" gorm:"type:numeric(12,2);not null;default:0"`
	NetAmount      decimal.Decimal  `json:"net_amount" gorm:"type:numeric(12,2);not null"`
	Method         string           `json:"method" gorm:"type:varchar(50);not null"`
	Status         WithdrawalStatus `json:"status" gorm:"type:varchar(20);not null;default:'pending'"`
	TransactionRef *string          `json:"transaction_ref,omitempty" gorm:"type:varchar(100);uniqueIndex"`
	CreatedAt      time.Time        `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      time.Time        `json:"updated_at" gorm:"autoUpdateTime"`
}
