package models

import (
	"time"

	"github.com/shopspring/decimal"
)

type WalletType string

const (
	WalletTypePassenger WalletType = "passenger"
	WalletTypeDriver    WalletType = "driver"
	WalletTypeOwner     WalletType = "owner"
)

type Wallet struct {
	ID         int64           `gorm:"primaryKey"`
	UserID     int64           `gorm:"not null;uniqueIndex:idx_wallets_user_type"`
	WalletType WalletType      `gorm:"type:wallet_type;not null;uniqueIndex:idx_wallets_user_type"`
	Freezed    bool            `gorm:"not null;default:false"`
	Balance    decimal.Decimal `gorm:"type:numeric(12,2);not null;default:0"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
