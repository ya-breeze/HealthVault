package backupapi

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"filippo.io/age"
	"github.com/google/uuid"
	"github.com/mattn/go-sqlite3"
)

// CaptureBarrier excludes in-process application requests while the database
// snapshot and its referenced uploads are copied. The server uses one shared
// RWMutex around all HTTP requests and this runner takes its exclusive lock.
// This protects one backend process; multiple replicas or direct DB writers
// require a cross-process coordination mechanism and are unsupported here.
type CaptureBarrier interface {
	TryLock() bool
	Unlock()
}

// SnapshotFunc exists so tests can force interruption at the SQLite boundary.
type SnapshotFunc func(context.Context, string, string) error

const (
	ArchiveManifestName  = "manifest.json"
	ArchiveDatabaseName  = "database.sqlite"
	ArchiveUploadsPrefix = "uploads/"
)

type SetRunner struct {
	DatabasePath   string
	UploadsDir     string
	SpoolDir       string
	PersistentRoot string
	AgeRecipient   string
	EncryptionID   string
	Barrier        CaptureBarrier

	// Snapshot is optional. Production uses SQLite's online backup API.
	Snapshot SnapshotFunc
	Now      func() time.Time
	// removeScratch is a test seam; production always uses os.RemoveAll.
	removeScratch func(string) error
}

// BackupManifest describes the contents of a plaintext backup archive. Paths
// are relative archive member names, never local filesystem paths.
type BackupManifest struct {
	FormatVersion int                  `json:"format_version"`
	CreatedAt     time.Time            `json:"created_at"`
	Database      BackupManifestFile   `json:"database"`
	Uploads       []BackupManifestFile `json:"uploads"`
}

// BackupManifestFile records the relative path, byte size, and SHA-256 of one
// regular archive member.
type BackupManifestFile struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

type capturedSet struct {
	databasePath string
	uploadsDir   string
	archivePath  string
	manifest     BackupManifest
	manifestJSON []byte
	manifestHash string
}

// Run creates and durably publishes a verified encrypted project backup.
// It leaves no plaintext outside a private temporary directory and returns
// evidence only after the ready marker is atomically published.
func (r *SetRunner) Run(ctx context.Context, _ Job) (evidence *Evidence, failure *Error) {
	if r == nil || r.SpoolDir == "" || r.DatabasePath == "" || r.UploadsDir == "" || r.AgeRecipient == "" || r.EncryptionID == "" {
		return nil, &Error{Code: "storage_unconfigured"}
	}
	if !safeEvidenceLabel(r.EncryptionID) {
		return nil, &Error{Code: "storage_unconfigured"}
	}
	if r.PersistentRoot != "" && !pathIsWithin(r.PersistentRoot, r.SpoolDir) {
		return nil, &Error{Code: "storage_unconfigured"}
	}
	if pathsOverlap(r.SpoolDir, r.UploadsDir) {
		return nil, &Error{Code: "storage_unconfigured"}
	}
	if err := ensurePrivateSpool(r.SpoolDir); err != nil {
		return nil, &Error{Code: "storage_unconfigured"}
	}
	if r.PersistentRoot != "" && !resolvedPathIsWithin(r.PersistentRoot, r.SpoolDir) {
		return nil, &Error{Code: "storage_unconfigured"}
	}
	if resolvedPathsOverlap(r.SpoolDir, r.UploadsDir) {
		return nil, &Error{Code: "storage_unconfigured"}
	}
	tmp, err := os.MkdirTemp("", "healthvault-backup-")
	if err != nil {
		return nil, &Error{Code: "snapshot_failed"}
	}
	if err := os.Chmod(tmp, 0o700); err != nil {
		_ = os.RemoveAll(tmp)
		return nil, &Error{Code: "snapshot_failed"}
	}
	defer func() {
		remove := os.RemoveAll
		if r.removeScratch != nil {
			remove = r.removeScratch
		}
		if err := remove(tmp); err != nil {
			// A completed backup must not leave plaintext in local scratch.
			evidence = nil
			failure = &Error{Code: "internal"}
		}
	}() // exact, newly created private directory

	set, err := r.capture(ctx, tmp)
	if err != nil {
		code := "snapshot_failed"
		var staged *stageError
		if errors.As(err, &staged) {
			code = staged.code
		}
		return nil, failureForContext(ctx, code)
	}
	archivePath := set.archivePath

	recipients, err := age.ParseRecipients(strings.NewReader(r.AgeRecipient + "\n"))
	if err != nil || len(recipients) != 1 {
		return nil, failureForContext(ctx, "encryption_failed")
	}
	cipherPath := filepath.Join(tmp, "backup.age")
	if err := encryptFile(ctx, archivePath, cipherPath, recipients[0]); err != nil {
		return nil, failureForContext(ctx, "encryption_failed")
	}
	cipherSize, cipherHash, err := digestFile(cipherPath)
	if err != nil || cipherSize <= 0 {
		return nil, failureForContext(ctx, "encryption_failed")
	}
	fileID, err := uuid.NewRandom()
	if err != nil {
		return nil, failureForContext(ctx, "publish_failed")
	}
	readyFileID := fileID.String() + ".age"
	if err := publishArtifact(ctx, r.SpoolDir, cipherPath, readyFileID, cipherSize, cipherHash); err != nil {
		return nil, failureForContext(ctx, "publish_failed")
	}
	return &Evidence{
		BackupSetType:       "full",
		ManifestSHA256:      set.manifestHash,
		ReadyFileID:         readyFileID,
		CiphertextSHA256:    cipherHash,
		CiphertextSizeBytes: cipherSize,
		EncryptionKeyID:     r.EncryptionID,
	}, nil
}

func pathIsWithin(root, path string) bool {
	rootAbs, rootErr := filepath.Abs(root)
	pathAbs, pathErr := filepath.Abs(path)
	if rootErr != nil || pathErr != nil {
		return false
	}
	rootAbs, pathAbs = filepath.Clean(rootAbs), filepath.Clean(pathAbs)
	rel, err := filepath.Rel(rootAbs, pathAbs)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func resolvedPathIsWithin(root, path string) bool {
	rootResolved, rootErr := resolvePath(root)
	pathResolved, pathErr := resolvePath(path)
	if rootErr != nil || pathErr != nil {
		return false
	}
	return pathIsWithin(rootResolved, pathResolved)
}

func resolvePath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	current := filepath.Clean(absolute)
	var missing []string
	for {
		resolved, resolveErr := filepath.EvalSymlinks(current)
		if resolveErr == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(resolveErr) {
			return "", resolveErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", resolveErr
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func pathsOverlap(first, second string) bool {
	firstAbs, firstErr := filepath.Abs(first)
	secondAbs, secondErr := filepath.Abs(second)
	if firstErr != nil || secondErr != nil {
		return true
	}
	firstAbs, secondAbs = filepath.Clean(firstAbs), filepath.Clean(secondAbs)
	return pathIsWithinOrSame(firstAbs, secondAbs) || pathIsWithinOrSame(secondAbs, firstAbs)
}

func resolvedPathsOverlap(first, second string) bool {
	firstResolved, firstErr := resolvePath(first)
	secondResolved, secondErr := resolvePath(second)
	if firstErr != nil || secondErr != nil {
		return true
	}
	return pathsOverlap(firstResolved, secondResolved)
}

func pathIsWithinOrSame(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

type readyManifest struct {
	ArtifactFile        string `json:"artifact_file"`
	CiphertextSizeBytes int64  `json:"ciphertext_size_bytes"`
	CiphertextSHA256    string `json:"ciphertext_sha256"`
}

func ensurePrivateSpool(path string) error {
	if err := mkdirAllDurable(path); err != nil {
		return err
	}
	// Sync even when the directory already existed. This also makes a retry
	// safe if the previous creation's parent sync failed.
	if err := syncDirectory(filepath.Dir(filepath.Clean(path))); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return errors.New("backup spool must be a private directory")
	}
	return nil
}

// mkdirAllDurable creates each missing directory privately and syncs the
// containing directory after adding its entry. Existing components must be
// real directories so a symlink cannot redirect spool creation.
func mkdirAllDurable(path string) error {
	path = filepath.Clean(path)
	info, err := os.Lstat(path)
	if err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("backup spool path component is not a directory")
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return err
	}
	if err := mkdirAllDurable(parent); err != nil {
		return err
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		info, statErr := os.Lstat(path)
		if statErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("backup spool path component is not a directory")
		}
		return nil
	}
	return syncDirectory(parent)
}

func publishArtifact(ctx context.Context, spoolDir, sourcePath, fileID string, expectedSize int64, expectedHash string) error {
	if !readyFileIDPattern.MatchString(fileID) || expectedSize <= 0 || !isDigest(expectedHash) {
		return errors.New("invalid backup artifact")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	finalPath := filepath.Join(spoolDir, fileID)
	markerPath := filepath.Join(spoolDir, strings.TrimSuffix(fileID, ".age")+".ready.json")
	if _, err := os.Lstat(finalPath); err == nil || !errors.Is(err, os.ErrNotExist) {
		return errors.New("backup artifact already exists")
	}
	if _, err := os.Lstat(markerPath); err == nil || !errors.Is(err, os.ErrNotExist) {
		return errors.New("backup ready marker already exists")
	}

	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	staged, err := os.CreateTemp(spoolDir, ".backup-artifact-*.tmp")
	if err != nil {
		return err
	}
	stagedPath := staged.Name()
	defer os.Remove(stagedPath) // exact unique staging path; published files have another name
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(staged, hash), &contextReader{ctx: ctx, reader: source})
	syncErr := staged.Sync()
	closeErr := staged.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil || written != expectedSize || hex.EncodeToString(hash.Sum(nil)) != expectedHash {
		return errors.New("backup ciphertext changed while publishing")
	}
	if err := os.Rename(stagedPath, finalPath); err != nil {
		return err
	}
	if err := syncDirectory(spoolDir); err != nil {
		return err
	}
	if err := verifyPublishedArtifact(finalPath, expectedSize, expectedHash); err != nil {
		return err
	}

	manifest, err := json.Marshal(readyManifest{ArtifactFile: fileID, CiphertextSizeBytes: expectedSize, CiphertextSHA256: expectedHash})
	if err != nil {
		return err
	}
	marker, err := os.CreateTemp(spoolDir, ".backup-ready-*.tmp")
	if err != nil {
		return err
	}
	markerTemp := marker.Name()
	defer os.Remove(markerTemp)
	if _, err := marker.Write(manifest); err != nil {
		_ = marker.Close()
		return err
	}
	if err := marker.Sync(); err != nil {
		_ = marker.Close()
		return err
	}
	if err := marker.Close(); err != nil {
		return err
	}
	if err := os.Rename(markerTemp, markerPath); err != nil {
		return err
	}
	return syncDirectory(spoolDir)
}

func verifyPublishedArtifact(path string, expectedSize int64, expectedHash string) error {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("published backup artifact is not a regular file")
	}
	size, hash, err := digestFile(path)
	if err != nil || size != expectedSize || hash != expectedHash {
		return errors.New("published backup ciphertext failed local verification")
	}
	return nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

type stageError struct{ code string }

func (e *stageError) Error() string { return e.code }

func localVerificationFailure() error { return &stageError{code: "local_verification_failed"} }

func failureForContext(ctx context.Context, fallback string) *Error {
	if ctx.Err() != nil {
		return &Error{Code: "timeout"}
	}
	return &Error{Code: fallback}
}

func (r *SetRunner) capture(ctx context.Context, tmp string) (*capturedSet, error) {
	if r.Barrier != nil {
		if err := acquireCaptureBarrier(ctx, r.Barrier); err != nil {
			return nil, err
		}
		defer r.Barrier.Unlock()
	}
	dbCopy := filepath.Join(tmp, "database.sqlite")
	if err := r.snapshot(ctx, r.DatabasePath, dbCopy); err != nil {
		return nil, err
	}
	if err := os.Chmod(dbCopy, 0o600); err != nil {
		return nil, err
	}
	references, err := photoReferences(ctx, dbCopy)
	if err != nil {
		return nil, err
	}
	files, err := listManifestFiles("uploads", r.UploadsDir)
	if errors.Is(err, os.ErrNotExist) && len(references) == 0 {
		// Photo storage is created on first upload. A fresh database with no
		// photo references has a valid empty upload set.
		files = []BackupManifestFile{}
		err = nil
	}
	if err != nil {
		return nil, err
	}
	fileSet := make(map[string]struct{}, len(files))
	for _, file := range files {
		fileSet[filepath.ToSlash(file.Path)] = struct{}{}
	}
	for _, ref := range references {
		if _, ok := fileSet[filepath.ToSlash(filepath.Join("uploads", ref))]; !ok {
			return nil, fmt.Errorf("referenced upload missing")
		}
	}
	dbEntry, err := manifestFor(ArchiveDatabaseName, dbCopy)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if r.Now != nil {
		now = r.Now().UTC()
	}
	value := BackupManifest{FormatVersion: 1, CreatedAt: now, Database: dbEntry, Uploads: files}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(encoded)
	set := &capturedSet{databasePath: dbCopy, uploadsDir: r.UploadsDir, archivePath: filepath.Join(tmp, "backup.tar"), manifest: value,
		manifestJSON: encoded, manifestHash: hex.EncodeToString(digest[:])}
	if err := verifySQLite(dbCopy); err != nil {
		return nil, localVerificationFailure()
	}
	if err := writeArchive(set.archivePath, set); err != nil {
		return nil, err
	}
	if err := verifyArchive(set.archivePath, set); err != nil {
		return nil, localVerificationFailure()
	}
	if err := os.Remove(dbCopy); err != nil {
		return nil, err
	}
	return set, nil
}

// TryLock does not queue a writer behind a long-lived request. While waiting,
// the API remains responsive and the job can fail closed at its deadline.
func acquireCaptureBarrier(ctx context.Context, barrier CaptureBarrier) error {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if barrier.TryLock() {
			if err := ctx.Err(); err != nil {
				barrier.Unlock()
				return err
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (r *SetRunner) snapshot(ctx context.Context, source, dest string) error {
	if r.Snapshot != nil {
		return r.Snapshot(ctx, source, dest)
	}
	return onlineSQLiteBackup(ctx, source, dest)
}

func onlineSQLiteBackup(ctx context.Context, source, dest string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("SQLite source is not a regular file")
	}
	file, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	srcDB, err := sqlOpenSQLite(source)
	if err != nil {
		return err
	}
	defer srcDB.Close()
	dstDB, err := sqlOpenSQLite(dest)
	if err != nil {
		return err
	}
	defer dstDB.Close()
	srcConn, err := srcDB.Conn(ctx)
	if err != nil {
		return err
	}
	defer srcConn.Close()
	dstConn, err := dstDB.Conn(ctx)
	if err != nil {
		return err
	}
	defer dstConn.Close()
	var backupErr error
	err = dstConn.Raw(func(dstRaw any) error {
		dst, ok := dstRaw.(*sqlite3.SQLiteConn)
		if !ok {
			return errors.New("unexpected SQLite destination connection")
		}
		return srcConn.Raw(func(srcRaw any) error {
			src, ok := srcRaw.(*sqlite3.SQLiteConn)
			if !ok {
				return errors.New("unexpected SQLite source connection")
			}
			backup, err := dst.Backup("main", src, "main")
			if err != nil {
				return err
			}
			defer func() { backupErr = backup.Finish() }()
			for {
				if err := ctx.Err(); err != nil {
					return err
				}
				done, err := backup.Step(256)
				if err != nil {
					return err
				}
				if done {
					return nil
				}
				time.Sleep(10 * time.Millisecond)
			}
		})
	})
	if err != nil {
		return err
	}
	return backupErr
}

// sqlOpenSQLite is kept as a helper so the online backup source and target
// use file URIs instead of an interpolated SQL path.
func sqlOpenSQLite(path string) (*sql.DB, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	uri := (&url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}).String() + "?_busy_timeout=30000"
	return sql.Open("sqlite3", uri)
}

func photoReferences(ctx context.Context, path string) ([]string, error) {
	db, err := sqlOpenSQLite(path)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var refs []string
	for _, table := range []string{"food_meals", "food_calibration_samples"} {
		rows, err := db.QueryContext(ctx, "SELECT photo_path FROM \""+table+"\" WHERE photo_path <> ''")
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				_ = rows.Close()
				return nil, err
			}
			if !safeRelativePath(value) {
				_ = rows.Close()
				return nil, errors.New("unsafe database photo reference")
			}
			refs = append(refs, value)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return refs, nil
}

func safeRelativePath(path string) bool {
	if path == "" || strings.Contains(path, "\\") || filepath.IsAbs(path) || filepath.VolumeName(path) != "" {
		return false
	}
	clean := filepath.Clean(path)
	return clean == path && clean != "." && clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

func listManifestFiles(prefix, root string) ([]BackupManifestFile, error) {
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("uploads directory unavailable")
	}
	files := make([]BackupManifestFile, 0)
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("unsafe copied upload")
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || !safeRelativePath(rel) {
			return errors.New("invalid copied upload path")
		}
		file, err := manifestFor(filepath.Join(prefix, rel), path)
		if err != nil {
			return err
		}
		files = append(files, file)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func manifestFor(name, path string) (BackupManifestFile, error) {
	size, digest, err := digestFile(path)
	if err != nil {
		return BackupManifestFile{}, err
	}
	return BackupManifestFile{Path: filepath.ToSlash(name), SizeBytes: size, SHA256: digest}, nil
}

func verifySQLite(path string) error {
	db, err := sqlOpenSQLite(path)
	if err != nil {
		return err
	}
	defer db.Close()
	rows, err := db.Query("PRAGMA integrity_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return err
		}
		if result != "ok" {
			return errors.New("SQLite integrity check failed")
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != 1 {
		return errors.New("SQLite integrity check returned an unexpected result")
	}
	return nil
}

func writeArchive(path string, set *capturedSet) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	w := tar.NewWriter(file)
	writeEntry := func(name string, size int64, mod time.Time, source io.Reader) error {
		header := &tar.Header{Name: filepath.ToSlash(name), Mode: 0o600, Size: size, ModTime: mod.UTC(), Typeflag: tar.TypeReg, Format: tar.FormatUSTAR}
		if err := w.WriteHeader(header); err != nil {
			return err
		}
		_, err := io.CopyN(w, source, size)
		return err
	}
	err = writeEntry(ArchiveManifestName, int64(len(set.manifestJSON)), set.manifest.CreatedAt, strings.NewReader(string(set.manifestJSON)))
	if err == nil {
		err = archiveFile(w, ArchiveDatabaseName, set.databasePath, set.manifest.Database)
	}
	for _, item := range set.manifest.Uploads {
		if err != nil {
			break
		}
		rel := strings.TrimPrefix(item.Path, ArchiveUploadsPrefix)
		err = archiveFile(w, item.Path, filepath.Join(set.uploadsDir, filepath.FromSlash(rel)), item)
	}
	if closeErr := w.Close(); err == nil {
		err = closeErr
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}

func archiveFile(w *tar.Writer, name, path string, item BackupManifestFile) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return writeTarEntry(w, name, item.SizeBytes, file)
}

func writeTarEntry(w *tar.Writer, name string, size int64, source io.Reader) error {
	if err := w.WriteHeader(&tar.Header{Name: filepath.ToSlash(name), Mode: 0o600, Size: size, Typeflag: tar.TypeReg, Format: tar.FormatUSTAR}); err != nil {
		return err
	}
	_, err := io.CopyN(w, source, size)
	return err
}

func verifyArchive(path string, set *capturedSet) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	reader := tar.NewReader(file)
	expected := map[string]BackupManifestFile{ArchiveDatabaseName: set.manifest.Database}
	for _, item := range set.manifest.Uploads {
		expected[item.Path] = item
	}
	seen := map[string]bool{}
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil || header.Typeflag != tar.TypeReg || header.Name == "" || seen[header.Name] {
			return errors.New("invalid backup archive")
		}
		seen[header.Name] = true
		if header.Name == ArchiveManifestName {
			b, err := io.ReadAll(io.LimitReader(reader, int64(len(set.manifestJSON))+1))
			if err != nil || string(b) != string(set.manifestJSON) {
				return errors.New("backup manifest mismatch")
			}
			continue
		}
		item, ok := expected[header.Name]
		if !ok || header.Size != item.SizeBytes {
			return errors.New("unexpected backup member")
		}
		h := sha256.New()
		n, err := io.Copy(h, reader)
		if err != nil || n != item.SizeBytes || hex.EncodeToString(h.Sum(nil)) != item.SHA256 {
			return errors.New("backup member checksum mismatch")
		}
	}
	if len(seen) != len(expected)+1 || !seen[ArchiveManifestName] {
		return errors.New("backup archive is incomplete")
	}
	return nil
}

func encryptFile(ctx context.Context, sourcePath, destPath string, recipient age.Recipient) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	dest, err := os.OpenFile(destPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	enc, err := age.Encrypt(dest, recipient)
	if err != nil {
		_ = dest.Close()
		return err
	}
	_, copyErr := io.Copy(enc, &contextReader{ctx: ctx, reader: source})
	encErr := enc.Close()
	closeErr := dest.Close()
	if copyErr != nil {
		return copyErr
	}
	if encErr != nil {
		return encErr
	}
	return closeErr
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func digestFile(path string) (int64, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer file.Close()
	return digestReader(file)
}

func digestReader(reader io.Reader) (int64, string, error) {
	hash := sha256.New()
	size, err := io.Copy(hash, reader)
	if err != nil {
		return 0, "", err
	}
	return size, hex.EncodeToString(hash.Sum(nil)), nil
}

func safeEvidenceLabel(value string) bool {
	if value == "" || len(value) > 200 || containsUnsafe(value) || containsLeak(value) {
		return false
	}
	return true
}
