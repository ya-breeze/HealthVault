package commands

import (
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"
	"github.com/ya-breeze/healthvault/pkg/config"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
)

func CmdServer(logger *slog.Logger) *cobra.Command {
	return &cobra.Command{
		Use:   "server",
		Short: "Start the HealthVault HTTP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if len(cfg.JWTSecret) < 16 {
				return fmt.Errorf("HCW_JWT_SECRET must be set and at least 16 characters (got %d)", len(cfg.JWTSecret))
			}
			db, err := database.Open(logger, cfg.DBPath)
			if err != nil {
				return err
			}
			if err := database.SeedUsers(db, cfg.SeedUsers); err != nil {
				return err
			}
			storage := database.NewStorage(db)
			return server.Run(cmd.Context(), logger, cfg, storage)
		},
	}
}
