package commands

import (
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/ya-breeze/healthvault/pkg/config"
	"github.com/ya-breeze/healthvault/pkg/usda"
)

// CmdImportUSDA builds the local USDA reference database.
//
// This is an operator-run command rather than a scheduled job on purpose:
// SR Legacy is frozen (final release April 2018) and Foundation Foods publishes
// roughly twice a year, so a background poller would be machinery for an event
// that happens 0-2 times a year — and would be the only background job in the
// service. See openspec .../usda-nutrition-database "Operator-Run USDA Import".
func CmdImportUSDA(logger *slog.Logger) *cobra.Command {
	var sources []string

	cmd := &cobra.Command{
		Use:   "import-usda",
		Short: "Import USDA FoodData Central reference data into the local search database",
		Long: "Downloads (or reads) FoodData Central CSV bundles and builds the local\n" +
			"SQLite FTS5 food index. The new database is built to a temporary file and\n" +
			"only replaces the existing one after it passes a minimum row-count check,\n" +
			"so a failed or partial import leaves the previous database in service.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if len(sources) == 0 {
				sources = usda.DefaultSources
			}

			builder, err := usda.NewBuilder(cfg.USDADBPath)
			if err != nil {
				return err
			}

			total := 0
			for _, src := range sources {
				logger.Info("fetching USDA dataset", "source", src)
				local, cleanup, err := usda.Fetch(src)
				if err != nil {
					builder.Discard()
					return err
				}
				n, err := usda.ImportZip(local, builder)
				cleanup()
				if err != nil {
					builder.Discard()
					return fmt.Errorf("import %s: %w", src, err)
				}
				logger.Info("parsed dataset", "source", src, "foods", n)
				total += n
			}

			n, err := builder.Promote()
			if err != nil {
				return err
			}
			logger.Info("USDA import complete", "foods", n, "path", cfg.USDADBPath)
			cmd.Printf("Imported %d foods into %s\n", n, cfg.USDADBPath)
			return nil
		},
	}

	cmd.Flags().StringSliceVar(&sources, "source", nil,
		"FoodData Central CSV zip URL or local path (repeatable). Defaults to the SR Legacy bundle.")
	return cmd
}
