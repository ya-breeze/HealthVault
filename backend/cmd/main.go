package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/ya-breeze/healthvault/cmd/commands"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	root := &cobra.Command{Use: "hcw", Short: "HealthVault"}
	root.AddCommand(commands.CmdServer(logger))
	root.AddCommand(commands.CmdMCPConfig(logger))
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
