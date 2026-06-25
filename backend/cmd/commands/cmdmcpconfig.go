package commands

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/ya-breeze/healthvault/pkg/config"
)

// CmdMCPConfig generates or updates a .mcp.json file for use with Claude Desktop / Claude Code.
func CmdMCPConfig(logger *slog.Logger) *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "mcp-config",
		Short: "Generate .mcp.json for Claude Desktop / Claude Code",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			mcpURL := fmt.Sprintf("http://localhost:%s/mcp", cfg.Port)
			entry := map[string]any{
				"healthvault": map[string]any{
					"url": mcpURL,
				},
			}

			// Load existing file if present, so we can merge rather than overwrite.
			var existing map[string]any
			if data, err := os.ReadFile(output); err == nil {
				_ = json.Unmarshal(data, &existing)
			}
			if existing == nil {
				existing = map[string]any{}
			}
			servers, _ := existing["mcpServers"].(map[string]any)
			if servers == nil {
				servers = map[string]any{}
			}
			for k, v := range entry {
				servers[k] = v
			}
			existing["mcpServers"] = servers

			b, err := json.MarshalIndent(existing, "", "  ")
			if err != nil {
				return fmt.Errorf("marshal: %w", err)
			}

			// Write to stdout when output is "-"
			if output == "-" {
				_, err = fmt.Fprintln(os.Stdout, string(b))
				return err
			}

			if err := os.WriteFile(output, b, 0o644); err != nil {
				return fmt.Errorf("write %s: %w", output, err)
			}
			logger.Info("wrote MCP config", "path", output)
			fmt.Fprintf(os.Stderr, "wrote MCP config to %s\n", output)
			return nil
		},
	}
	cmd.Flags().StringVar(&output, "output", ".mcp.json", `output file path (use "-" for stdout)`)
	return cmd
}
