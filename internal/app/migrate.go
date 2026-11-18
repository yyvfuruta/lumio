package app

import (
	"context"

	"github.com/yyvfuruta/lumio/internal/adapters/maintenance"
)

func MigrateStorage(ctx context.Context, databaseURL, targetURL string) error {
	return maintenance.MigrateStorage(ctx, databaseURL, targetURL)
}
