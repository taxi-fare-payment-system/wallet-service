package models

import (
	"time"

	"github.com/shopspring/decimal"
)

type WalletTopupCredit struct {
	PaymentTransactionID string          `gorm:"primaryKey;type:uuid"`
	WalletID             string          `gorm:"type:uuid;not null;index"`
	Amount               decimal.Decimal `gorm:"type:numeric(12,2);not null"`
	Currency             string          `gorm:"not null"`
	TxRef                *string
	ChapaReference       *string
	CreatedAt            time.Time `gorm:"autoCreateTime"`
}
