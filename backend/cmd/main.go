package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/ya-breeze/healthvault/cmd/commands"

	// The runtime image (debian:bookworm-slim, see backend/Dockerfile)
	// installs no tzdata package, so without this embedded zone database
	// every time.LoadLocation call fails and database.ResolveTimezone
	// silently degrades every user's chart buckets to UTC.
	_ "time/tzdata"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	root := &cobra.Command{Use: "hcw", Short: "HealthVault"}
	root.AddCommand(commands.CmdServer(logger))
	root.AddCommand(commands.CmdMCPConfig(logger))
	root.AddCommand(commands.CmdImportUSDA(logger))
	root.AddCommand(commands.CmdImportOFF(logger))
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
