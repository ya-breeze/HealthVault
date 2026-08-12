package commands

import (
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/ya-breeze/healthvault/pkg/config"
	"github.com/ya-breeze/healthvault/pkg/off"
)

// CmdImportOFF builds the local Open Food Facts reference database, filtered
// to Czech/Slovak-market products with complete calories/protein/carbs/fat.
//
// Operator-run, not scheduled, for the same reason as import-usda: the
// project has no background-job infrastructure. Unlike USDA's frozen SR
// Legacy bundle, the OFF export updates nightly, so re-running this command
// is a normal operator action, not a one-time bootstrap step. See
// openspec/specs/off-nutrition-database/spec.md "Operator-Run Open Food
// Facts Import".
func CmdImportOFF(logger *slog.Logger) *cobra.Command {
	var source string

	cmd := &cobra.Command{
		Use:   "import-off",
		Short: "Import Open Food Facts reference data into the local search database",
		Long: "Downloads (or reads) the Open Food Facts full JSONL export, filters it to\n" +
			"Czech/Slovak-market products with complete nutriments, and builds the local\n" +
			"SQLite FTS5 product index. The new database is built to a temporary file and\n" +
			"only replaces the existing one after it passes a minimum row-count check,\n" +
			"so a failed or partial import leaves the previous database in service.\n\n" +
			"Requires a backend restart to take effect — the server opens the index once\n" +
			"at startup and does not reload it.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if source == "" {
				source = off.DefaultSource
			}

			builder, err := off.NewBuilder(cfg.OFFDBPath)
			if err != nil {
				return err
			}

			logger.Info("fetching Open Food Facts dataset", "source", source)
			local, cleanup, err := off.Fetch(source)
			if err != nil {
				builder.Discard()
				return err
			}
			stats, err := off.ImportJSONL(local, builder)
			cleanup()
			if err != nil {
				builder.Discard()
				return fmt.Errorf("import %s: %w", source, err)
			}
			logger.Info("parsed dataset", "source", source,
				"kept", stats.Kept, "filtered", stats.Filtered, "malformed", stats.Malformed)

			n, err := builder.Promote()
			if err != nil {
				return err
			}
			logger.Info("Open Food Facts import complete", "foods", n, "path", cfg.OFFDBPath)
			cmd.Printf("Imported %d foods into %s (filtered %d, malformed %d)\n",
				n, cfg.OFFDBPath, stats.Filtered, stats.Malformed)
			return nil
		},
	}

	cmd.Flags().StringVar(&source, "source", "",
		"Open Food Facts JSONL export URL or local path. Defaults to the official nightly export.")
	return cmd
}
