package repository

import (
	"context"

	"wallet_service/internal/models"

	"gorm.io/gorm"
)

type ConfigRepository struct {
	db *gorm.DB
}

func NewConfigRepository(db *gorm.DB) *ConfigRepository {
	return &ConfigRepository{db: db}
}

func (r *ConfigRepository) Get(ctx context.Context, key string) (string, error) {
	var config models.SystemConfig
	err := r.db.WithContext(ctx).Where("key = ?", key).First(&config).Error
	if err != nil {
		return "", err
	}
	return config.Value, nil
}

func (r *ConfigRepository) GetAll(ctx context.Context) ([]models.SystemConfig, error) {
	var configs []models.SystemConfig
	err := r.db.WithContext(ctx).Find(&configs).Error
	return configs, err
}

func (r *ConfigRepository) Set(ctx context.Context, key, value string) error {
	return r.db.WithContext(ctx).Save(&models.SystemConfig{
		Key:   key,
		Value: value,
	}).Error
}
