package backupapi

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"filippo.io/age"
)

// RestoreReport contains verification results only. RestoreDrill never accepts
// a destination path, so callers cannot direct archive contents into live data.
type RestoreReport struct {
	ManifestSHA256 string
	DatabaseOK     bool
	UploadCount    int
	UploadBytes    int64
}

// RestoreDrill verifies one locally published backup in a fresh private
// temporary directory. identities are provided by the caller and are never
// written to disk. No files are installed into an application data directory.
func RestoreDrill(ctx context.Context, spoolDir string, evidence Evidence, identities []age.Identity) (result *RestoreReport, resultErr error) {
	if spoolDir == "" || validateEvidence(evidence) != nil || len(identities) == 0 {
		return nil, errors.New("invalid restore drill input")
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.New("restore drill canceled")
	}
	if err := ensurePrivateSpool(spoolDir); err != nil {
		return nil, errors.New("backup spool unavailable")
	}
	artifactPath := filepath.Join(spoolDir, evidence.ReadyFileID)
	markerPath := filepath.Join(spoolDir, strings.TrimSuffix(evidence.ReadyFileID, ".age")+".ready.json")
	markerInfo, err := os.Lstat(markerPath)
	if err != nil || !markerInfo.Mode().IsRegular() || markerInfo.Mode()&os.ModeSymlink != 0 || markerInfo.Size() < 1 || markerInfo.Size() > 4096 {
		return nil, errors.New("backup ready marker unavailable")
	}
	markerBytes, err := os.ReadFile(markerPath)
	if err != nil {
		return nil, errors.New("backup ready marker unavailable")
	}
	var marker readyManifest
	decoder := json.NewDecoder(strings.NewReader(string(markerBytes)))
	decoder.DisallowUnknownFields()
	var trailing any
	if decoder.Decode(&marker) != nil || decoder.Decode(&trailing) != io.EOF || marker.ArtifactFile != evidence.ReadyFileID ||
		marker.CiphertextSizeBytes != evidence.CiphertextSizeBytes || marker.CiphertextSHA256 != evidence.CiphertextSHA256 {
		return nil, errors.New("backup ready marker does not match evidence")
	}
	artifactInfo, err := os.Lstat(artifactPath)
	if err != nil || !artifactInfo.Mode().IsRegular() || artifactInfo.Mode()&os.ModeSymlink != 0 || artifactInfo.Size() != evidence.CiphertextSizeBytes {
		return nil, errors.New("backup artifact unavailable")
	}
	// Ignore TMPDIR: a caller must not redirect decrypted scratch files into
	// the live HealthVault data volume through an environment variable.
	tmp, err := os.MkdirTemp("/tmp", "healthvault-restore-drill-")
	if err != nil {
		return nil, errors.New("restore drill workspace unavailable")
	}
	if err := os.Chmod(tmp, 0o700); err != nil {
		_ = os.RemoveAll(tmp)
		return nil, errors.New("restore drill workspace unavailable")
	}
	defer func() {
		if cleanupErr := os.RemoveAll(tmp); cleanupErr != nil {
			result = nil
			resultErr = errors.New("restore drill temporary workspace cleanup failed")
		}
	}()

	cipherPath := filepath.Join(tmp, "backup.age")
	remote, err := os.Open(artifactPath)
	if err != nil {
		return nil, errors.New("backup artifact unavailable")
	}
	cipherFile, err := os.OpenFile(cipherPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		_ = remote.Close()
		return nil, errors.New("restore drill workspace unavailable")
	}
	cipherHash := sha256.New()
	cipherSize, copyErr := io.Copy(io.MultiWriter(cipherFile, cipherHash), io.LimitReader(&contextReader{ctx: ctx, reader: remote}, evidence.CiphertextSizeBytes+1))
	remoteCloseErr := remote.Close()
	cipherCloseErr := cipherFile.Close()
	if copyErr != nil || remoteCloseErr != nil || cipherCloseErr != nil ||
		cipherSize != evidence.CiphertextSizeBytes || hex.EncodeToString(cipherHash.Sum(nil)) != evidence.CiphertextSHA256 {
		return nil, errors.New("backup ciphertext verification failed")
	}

	cipherFile, err = os.Open(cipherPath)
	if err != nil {
		return nil, errors.New("backup ciphertext unavailable")
	}
	plain, err := age.Decrypt(&contextReader{ctx: ctx, reader: cipherFile}, identities...)
	if err != nil {
		_ = cipherFile.Close()
		return nil, errors.New("backup decryption failed")
	}
	archivePath := filepath.Join(tmp, "backup.tar")
	archiveFile, err := os.OpenFile(archivePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		_ = cipherFile.Close()
		return nil, errors.New("restore drill workspace unavailable")
	}
	plainSize, copyErr := io.Copy(archiveFile, io.LimitReader(&contextReader{ctx: ctx, reader: plain}, evidence.CiphertextSizeBytes+1))
	cipherCloseErr = cipherFile.Close()
	archiveCloseErr := archiveFile.Close()
	if copyErr != nil || cipherCloseErr != nil || archiveCloseErr != nil || plainSize > evidence.CiphertextSizeBytes {
		return nil, errors.New("backup decryption failed")
	}

	report, err := verifyRestoredArchive(ctx, archivePath, tmp, evidence.ManifestSHA256)
	if err != nil {
		return nil, err
	}
	return report, nil
}

func verifyRestoredArchive(ctx context.Context, archivePath, root, expectedManifestHash string) (*RestoreReport, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return nil, errors.New("backup archive unavailable")
	}
	defer file.Close()
	reader := tar.NewReader(file)

	first, err := reader.Next()
	if err != nil || first.Name != ArchiveManifestName || first.Typeflag != tar.TypeReg || first.Format != tar.FormatUSTAR || first.Size < 1 || first.Size > 4*1024*1024 {
		return nil, errors.New("backup manifest missing or invalid")
	}
	manifestBytes, err := io.ReadAll(io.LimitReader(reader, first.Size+1))
	if err != nil || int64(len(manifestBytes)) != first.Size {
		return nil, errors.New("backup manifest invalid")
	}
	manifestHash := sha256.Sum256(manifestBytes)
	manifestDigest := hex.EncodeToString(manifestHash[:])
	if manifestDigest != expectedManifestHash {
		return nil, errors.New("backup manifest checksum mismatch")
	}
	var manifest BackupManifest
	decoder := json.NewDecoder(strings.NewReader(string(manifestBytes)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&manifest) != nil {
		return nil, errors.New("backup manifest invalid")
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF || manifest.FormatVersion != 1 || manifest.CreatedAt.IsZero() ||
		manifest.CreatedAt.Location() != time.UTC || !validManifestFile(manifest.Database, ArchiveDatabaseName) {
		return nil, errors.New("backup manifest invalid")
	}

	expected := map[string]BackupManifestFile{ArchiveDatabaseName: manifest.Database}
	for _, item := range manifest.Uploads {
		if !strings.HasPrefix(item.Path, ArchiveUploadsPrefix) || !validManifestFile(item, item.Path) {
			return nil, errors.New("backup manifest contains an unsafe path")
		}
		if _, exists := expected[item.Path]; exists {
			return nil, errors.New("backup manifest contains duplicate paths")
		}
		expected[item.Path] = item
	}

	seen := make(map[string]bool, len(expected))
	for {
		if err := ctx.Err(); err != nil {
			return nil, errors.New("restore drill canceled")
		}
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil || header.Typeflag != tar.TypeReg || header.Format != tar.FormatUSTAR || !safeArchivePath(header.Name) || seen[header.Name] {
			return nil, errors.New("backup archive contains an unsafe member")
		}
		item, ok := expected[header.Name]
		if !ok || header.Size != item.SizeBytes {
			return nil, errors.New("backup archive members do not match manifest")
		}
		seen[header.Name] = true
		dest := filepath.Join(root, filepath.FromSlash(header.Name))
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			return nil, errors.New("restore drill workspace unavailable")
		}
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, errors.New("backup archive member could not be materialized")
		}
		hash := sha256.New()
		written, copyErr := io.Copy(io.MultiWriter(out, hash), &contextReader{ctx: ctx, reader: reader})
		closeErr := out.Close()
		if copyErr != nil || closeErr != nil || written != item.SizeBytes || hex.EncodeToString(hash.Sum(nil)) != item.SHA256 {
			return nil, errors.New("backup archive member checksum mismatch")
		}
	}
	if len(seen) != len(expected) {
		return nil, errors.New("backup archive is incomplete")
	}

	databasePath := filepath.Join(root, ArchiveDatabaseName)
	if err := verifySQLite(databasePath); err != nil {
		return nil, errors.New("restored SQLite integrity check failed")
	}
	references, err := photoReferences(ctx, databasePath)
	if err != nil {
		return nil, errors.New("restored photo references could not be verified")
	}
	for _, reference := range references {
		member := ArchiveUploadsPrefix + path.Clean(reference)
		if _, ok := expected[member]; !ok || !seen[member] {
			return nil, errors.New("restored photo reference has no uploaded bytes")
		}
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(member)))
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("restored photo reference has no uploaded bytes")
		}
	}
	uploadCount := len(manifest.Uploads)
	var uploadBytes int64
	for _, item := range manifest.Uploads {
		uploadBytes += item.SizeBytes
	}
	return &RestoreReport{ManifestSHA256: manifestDigest, DatabaseOK: true, UploadCount: uploadCount, UploadBytes: uploadBytes}, nil
}

func validManifestFile(item BackupManifestFile, expectedPath string) bool {
	if item.Path != expectedPath || item.SizeBytes < 0 || !safeArchivePath(item.Path) || len(item.SHA256) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(item.SHA256)
	return err == nil && len(decoded) == sha256.Size && strings.ToLower(item.SHA256) == item.SHA256
}

func safeArchivePath(value string) bool {
	if value == "" || strings.Contains(value, "\\") || strings.ContainsRune(value, 0) || path.IsAbs(value) || path.Clean(value) != value {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}
