// ===========================================
// Logger Package
// ===========================================
// Centralized structured logging setup using slog
// ===========================================
package logger

import (
	"log/slog"
	"os"

	"github.com/laravel-paas/backend/internal/config"
)

// Setup initializes the global slog instance
func Setup(cfg *config.Config) {
	var handler slog.Handler

	if cfg.AppEnv == "production" {
		// Use JSON handler for production logging to support log aggregation (Datadog, ELK, Grafana)
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	} else {
		// Use Text handler for easy reading in local development
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
}
