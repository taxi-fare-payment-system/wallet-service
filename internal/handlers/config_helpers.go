package handlers

import (
	"context"
	"strings"

	"wallet_service/internal/models"
	"wallet_service/internal/repository"

	"github.com/shopspring/decimal"
)

func parseFarePlatformFee(ctx context.Context, repo *repository.ConfigRepository) (decimal.Decimal, error) {
	raw := models.DefaultFarePlatformFee
	if repo != nil {
		v, err := repo.Get(ctx, models.ConfigFarePlatformFee)
		if err == nil && strings.TrimSpace(v) != "" {
			raw = strings.TrimSpace(v)
		}
	}
	fee, err := decimal.NewFromString(raw)
	if err != nil || fee.Cmp(decimal.Zero) < 0 {
		return decimal.Zero, err
	}
	return fee, nil
}
