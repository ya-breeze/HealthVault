package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"filippo.io/age"
	"github.com/spf13/cobra"
	"github.com/ya-breeze/healthvault/pkg/backupapi"
	"github.com/ya-breeze/healthvault/pkg/config"
)

func CmdBackupRestoreDrill() *cobra.Command {
	var readyFileID, manifestHash, ciphertextHash string
	var ciphertextSize int64
	cmd := &cobra.Command{
		Use:   "backup-restore-drill",
		Short: "verify one encrypted HealthVault backup in an isolated temporary directory",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return errors.New("backup configuration is unavailable")
			}
			if cfg.BackupEncryptionKeyID == "" || cfg.BackupSpoolDir == "" {
				return errors.New("backup spool and encryption key ID must be configured")
			}
			identityBytes, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), 16*1024+1))
			if err != nil || len(identityBytes) == 0 || len(identityBytes) > 16*1024 {
				return errors.New("read one age identity from standard input")
			}
			identities, err := age.ParseIdentities(bytes.NewReader(identityBytes))
			for i := range identityBytes {
				identityBytes[i] = 0
			}
			if err != nil || len(identities) == 0 {
				return errors.New("standard input does not contain a valid age identity")
			}
			evidence := backupapi.Evidence{BackupSetType: "full", ManifestSHA256: manifestHash,
				ReadyFileID: readyFileID, CiphertextSHA256: ciphertextHash, CiphertextSizeBytes: ciphertextSize,
				EncryptionKeyID: cfg.BackupEncryptionKeyID}
			report, err := backupapi.RestoreDrill(cmd.Context(), cfg.BackupSpoolDir, evidence, identities)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(report)
		},
	}
	cmd.Flags().StringVar(&readyFileID, "ready-file-id", "", "artifact basename from backup evidence")
	cmd.Flags().StringVar(&manifestHash, "manifest-sha256", "", "manifest SHA-256 from backup evidence")
	cmd.Flags().StringVar(&ciphertextHash, "ciphertext-sha256", "", "ciphertext SHA-256 from backup evidence")
	cmd.Flags().Int64Var(&ciphertextSize, "ciphertext-size-bytes", 0, "ciphertext size from backup evidence")
	_ = cmd.MarkFlagRequired("ready-file-id")
	_ = cmd.MarkFlagRequired("manifest-sha256")
	_ = cmd.MarkFlagRequired("ciphertext-sha256")
	_ = cmd.MarkFlagRequired("ciphertext-size-bytes")
	return cmd
}
