package models

import (
	"time"
)

type SystemConfig struct {
	Key       string    `json:"key" gorm:"primaryKey;type:varchar(100)"`
	Value     string    `json:"value" gorm:"type:text;not null"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}
