package models

import (
	"time"
)

const (
	ConfigFarePlatformFee    = "fare_platform_fee"
	DefaultFarePlatformFee   = "0.05"
	ConfigDailyWithdrawalLimit = "daily_withdrawal_limit"
	ConfigAutoApproveThreshold = "auto_approve_threshold"
)

type SystemConfig struct {
	Key       string    `json:"key" gorm:"primaryKey;type:varchar(100)"`
	Value     string    `json:"value" gorm:"type:text;not null"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}
