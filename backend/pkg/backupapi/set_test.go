package backupapi

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"filippo.io/age"
	_ "github.com/mattn/go-sqlite3"
)

func TestSetRunnerCreatesEncryptedConsistentSetAndPublishesReadyArtifact(t *testing.T) {
	fx := newBackupFixture(t)
	scratch := filepath.Join(t.TempDir(), "private-scratch")
	if err := os.Mkdir(scratch, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", scratch)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	runner := fixtureRunner(t, fx, identity.Recipient().String())
	runner.Barrier = &sync.RWMutex{}
	runner.Now = func() time.Time { return time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC) }

	evidence, failure := runner.Run(context.Background(), Job{})
	if failure != nil {
		t.Fatalf("Run() failed: %s", failure.Code)
	}
	if evidence == nil || validateEvidence(*evidence) != nil {
		t.Fatalf("invalid evidence: %+v", evidence)
	}
	leftovers, err := os.ReadDir(scratch)
	if err != nil || len(leftovers) != 0 {
		t.Fatalf("private plaintext workspace was not cleaned: entries=%d err=%v", len(leftovers), err)
	}
	ciphertext := readArtifact(t, runner, *evidence)
	if bytes.Contains(ciphertext, []byte("synthetic-meal-photo")) || bytes.Contains(ciphertext, []byte("synthetic-health-row")) || bytes.Contains(ciphertext, []byte("fake-object-secret")) {
		t.Fatal("published artifact contains plaintext or credentials")
	}
	hash := sha256.Sum256(ciphertext)
	if hex.EncodeToString(hash[:]) != evidence.CiphertextSHA256 || int64(len(ciphertext)) != evidence.CiphertextSizeBytes {
		t.Fatal("evidence does not match published ciphertext")
	}
	entries, err := os.ReadDir(runner.SpoolDir)
	if err != nil || len(entries) != 2 {
		t.Fatalf("expected artifact and ready marker, entries=%d err=%v", len(entries), err)
	}
	marker, err := os.ReadFile(filepath.Join(runner.SpoolDir, strings.TrimSuffix(evidence.ReadyFileID, ".age")+".ready.json"))
	if err != nil || !bytes.Contains(marker, []byte(evidence.CiphertextSHA256)) || !bytes.Contains(marker, []byte(evidence.ReadyFileID)) {
		t.Fatalf("ready marker does not match evidence: %s (%v)", marker, err)
	}

	plain, err := age.Decrypt(bytes.NewReader(ciphertext), identity)
	if err != nil {
		t.Fatalf("decrypt backup: %v", err)
	}
	archive, err := io.ReadAll(plain)
	if err != nil {
		t.Fatalf("read decrypted archive: %v", err)
	}
	manifestBytes := readTarMember(t, archive, "manifest.json")
	manifestHash := sha256.Sum256(manifestBytes)
	if hex.EncodeToString(manifestHash[:]) != evidence.ManifestSHA256 {
		t.Fatal("evidence manifest checksum does not match archived manifest")
	}
	var value BackupManifest
	if err := json.Unmarshal(manifestBytes, &value); err != nil {
		t.Fatal(err)
	}
	if value.FormatVersion != 1 || !value.CreatedAt.Equal(runner.Now()) || len(value.Uploads) != 2 {
		t.Fatalf("unexpected archive manifest: %+v", value)
	}
	for _, member := range []string{"database.sqlite", "uploads/user-a/meal/meal.jpg", "uploads/user-a/calibration/sample.png"} {
		if got := readTarMember(t, archive, member); len(got) == 0 {
			t.Fatalf("archive member %q is empty", member)
		}
	}
}

func TestSetRunnerDoesNotReportSuccessWhenScratchCleanupFails(t *testing.T) {
	fx := newBackupFixture(t)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	runner := fixtureRunner(t, fx, identity.Recipient().String())
	runner.removeScratch = func(path string) error {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
		return errors.New("simulated scratch cleanup failure")
	}
	evidence, failure := runner.Run(context.Background(), Job{})
	if evidence != nil || failure == nil || failure.Code != "internal" {
		t.Fatalf("cleanup failure yielded evidence=%+v failure=%+v", evidence, failure)
	}
	entries, err := os.ReadDir(runner.SpoolDir)
	if err != nil || len(entries) != 2 {
		t.Fatalf("test did not reach artifact publication: entries=%d err=%v", len(entries), err)
	}
}

func TestCaptureBarrierWaitHonorsDeadlineWithoutQueuingWriter(t *testing.T) {
	barrier := &sync.RWMutex{}
	barrier.RLock()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := acquireCaptureBarrier(ctx, barrier); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("barrier wait returned %v, want deadline", err)
	}
	// A timed-out writer must not leave the barrier locked for the app.
	barrier.RUnlock()
	if !barrier.TryLock() {
		t.Fatal("timed-out backup left the barrier unavailable")
	}
	barrier.Unlock()
}

func TestSetRunnerAcceptsFreshDatabaseWithoutUploadsDirectory(t *testing.T) {
	fx := newBackupFixture(t)
	db, err := sql.Open("sqlite3", fx.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"food_meals", "food_calibration_samples"} {
		if _, err := db.Exec("DELETE FROM " + table); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(fx.uploads); err != nil {
		t.Fatal(err)
	}
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	runner := fixtureRunner(t, fx, identity.Recipient().String())
	evidence, failure := runner.Run(context.Background(), Job{})
	if failure != nil || evidence == nil {
		t.Fatalf("fresh database failed backup: evidence=%+v failure=%+v", evidence, failure)
	}
	if _, err := RestoreDrill(context.Background(), runner.SpoolDir, *evidence, []age.Identity{identity}); err != nil {
		t.Fatalf("fresh database did not restore: %v", err)
	}
}

func TestRestoreDrillIgnoresCallerTMPDIR(t *testing.T) {
	fx := newBackupFixture(t)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	runner := fixtureRunner(t, fx, identity.Recipient().String())
	evidence, failure := runner.Run(context.Background(), Job{})
	if failure != nil || evidence == nil {
		t.Fatalf("synthetic backup failed: %+v", failure)
	}
	// If the drill honored TMPDIR, it would fail to create a workspace here.
	// It must choose its own scratch root rather than the live data volume.
	t.Setenv("TMPDIR", filepath.Join(fx.uploads, "nonexistent"))
	if _, err := RestoreDrill(context.Background(), runner.SpoolDir, *evidence, []age.Identity{identity}); err != nil {
		t.Fatalf("restore drill used caller TMPDIR: %v", err)
	}
}

func TestSetRunnerFailsClosedBeforePublication(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*testing.T, *backupFixture, string) (*SetRunner, context.Context)
		wantCode  string
	}{
		{
			name: "referenced upload missing",
			configure: func(t *testing.T, fx *backupFixture, recipient string) (*SetRunner, context.Context) {
				if err := os.Remove(filepath.Join(fx.uploads, "user-a/meal/meal.jpg")); err != nil {
					t.Fatal(err)
				}
				return fixtureRunner(t, fx, recipient), context.Background()
			},
			wantCode: "snapshot_failed",
		},
		{
			name: "interrupted snapshot",
			configure: func(t *testing.T, fx *backupFixture, recipient string) (*SetRunner, context.Context) {
				runner := fixtureRunner(t, fx, recipient)
				runner.Snapshot = func(context.Context, string, string) error { return errors.New("private snapshot detail") }
				return runner, context.Background()
			},
			wantCode: "snapshot_failed",
		},
		{
			name: "encryption failure",
			configure: func(t *testing.T, fx *backupFixture, _ string) (*SetRunner, context.Context) {
				return fixtureRunner(t, fx, "invalid-age-recipient"), context.Background()
			},
			wantCode: "encryption_failed",
		},
		{
			name: "invalid spool configuration",
			configure: func(t *testing.T, fx *backupFixture, recipient string) (*SetRunner, context.Context) {
				runner := fixtureRunner(t, fx, recipient)
				if err := os.RemoveAll(runner.SpoolDir); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(runner.SpoolDir, []byte("not a directory"), 0o600); err != nil {
					t.Fatal(err)
				}
				return runner, context.Background()
			},
			wantCode: "storage_unconfigured",
		},
		{
			name: "cancelled snapshot",
			configure: func(t *testing.T, fx *backupFixture, recipient string) (*SetRunner, context.Context) {
				runner := fixtureRunner(t, fx, recipient)
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return runner, ctx
			},
			wantCode: "timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newBackupFixture(t)
			identity, err := age.GenerateX25519Identity()
			if err != nil {
				t.Fatal(err)
			}
			runner, ctx := tt.configure(t, fx, identity.Recipient().String())
			_, failure := runner.Run(ctx, Job{})
			if failure == nil || failure.Code != tt.wantCode {
				t.Fatalf("want failure %q, got %+v", tt.wantCode, failure)
			}
			if strings.Contains(failure.Message, "private snapshot detail") || strings.Contains(failure.Message, "fake-object-secret") {
				t.Fatalf("failure leaked sensitive details: %q", failure.Message)
			}
		})
	}
}

func TestRestoreDrillRoundTripsSyntheticBackupAndRejectsCorruption(t *testing.T) {
	fx := newBackupFixture(t)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	runner := fixtureRunner(t, fx, identity.Recipient().String())
	evidence, failure := runner.Run(context.Background(), Job{})
	if failure != nil {
		t.Fatalf("create synthetic backup: %s", failure.Code)
	}

	scratch := filepath.Join(t.TempDir(), "private-restore-scratch")
	if err := os.Mkdir(scratch, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", scratch)
	report, err := RestoreDrill(context.Background(), runner.SpoolDir, *evidence, []age.Identity{identity})
	if err != nil {
		t.Fatalf("restore drill: %v", err)
	}
	if !report.DatabaseOK || report.ManifestSHA256 != evidence.ManifestSHA256 || report.UploadCount != 2 || report.UploadBytes != int64(len("synthetic-meal-photo")+len("synthetic-calibration-photo")) {
		t.Fatalf("unexpected restore verification report: %+v", report)
	}
	entries, err := os.ReadDir(scratch)
	if err != nil || len(entries) != 0 {
		t.Fatalf("restore workspace was not removed: entries=%d err=%v", len(entries), err)
	}

	originalCiphertext, err := os.ReadFile(filepath.Join(runner.SpoolDir, evidence.ReadyFileID))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := age.Decrypt(bytes.NewReader(originalCiphertext), identity)
	if err != nil {
		t.Fatal(err)
	}
	var changedArchive bytes.Buffer
	changedTar := tar.NewWriter(&changedArchive)
	changedPhoto := false
	archiveReader := tar.NewReader(plain)
	for {
		header, err := archiveReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		member, err := io.ReadAll(archiveReader)
		if err != nil {
			t.Fatal(err)
		}
		if header.Name == "uploads/user-a/meal/meal.jpg" {
			member[0] ^= 0xff
			changedPhoto = true
		}
		if err := changedTar.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := changedTar.Write(member); err != nil {
			t.Fatal(err)
		}
	}
	if err := changedTar.Close(); err != nil {
		t.Fatal(err)
	}
	if !changedPhoto {
		t.Fatal("synthetic archive did not contain expected photo")
	}
	var changedCiphertext bytes.Buffer
	changedEncryptor, err := age.Encrypt(&changedCiphertext, identity.Recipient())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := changedEncryptor.Write(changedArchive.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := changedEncryptor.Close(); err != nil {
		t.Fatal(err)
	}
	changedHash := sha256.Sum256(changedCiphertext.Bytes())
	artifactPath := filepath.Join(runner.SpoolDir, evidence.ReadyFileID)
	changedEvidence := *evidence
	changedEvidence.CiphertextSHA256 = hex.EncodeToString(changedHash[:])
	changedEvidence.CiphertextSizeBytes = int64(changedCiphertext.Len())
	if err := os.WriteFile(artifactPath, changedCiphertext.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeReadySidecar(runner.SpoolDir, changedEvidence); err != nil {
		t.Fatal(err)
	}
	if _, err := RestoreDrill(context.Background(), runner.SpoolDir, changedEvidence, []age.Identity{identity}); err == nil || err.Error() != "backup archive member checksum mismatch" {
		t.Fatalf("archive member corruption accepted: %v", err)
	}
	entries, err = os.ReadDir(scratch)
	if err != nil || len(entries) != 0 {
		t.Fatalf("member checksum failure left workspace behind: entries=%d err=%v", len(entries), err)
	}

	// Restore the genuine object before checking ciphertext-level corruption.
	if err := os.WriteFile(artifactPath, originalCiphertext, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeReadySidecar(runner.SpoolDir, *evidence); err != nil {
		t.Fatal(err)
	}
	originalCiphertext[0] ^= 0xff
	if err := os.WriteFile(artifactPath, originalCiphertext, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := RestoreDrill(context.Background(), runner.SpoolDir, *evidence, []age.Identity{identity}); err == nil || err.Error() != "backup ciphertext verification failed" {
		t.Fatalf("corrupt remote ciphertext accepted: %v", err)
	}
	entries, err = os.ReadDir(scratch)
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed drill left workspace behind: entries=%d err=%v", len(entries), err)
	}
}

func TestRestoreDrillRejectsUnsafeArchiveMember(t *testing.T) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	manifest := BackupManifest{FormatVersion: 1, CreatedAt: time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC),
		Database: BackupManifestFile{Path: ArchiveDatabaseName, SizeBytes: 1, SHA256: strings.Repeat("0", 64)}}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	tarWriter := tar.NewWriter(&archive)
	if err := tarWriter.WriteHeader(&tar.Header{Name: ArchiveManifestName, Mode: 0o600, Size: int64(len(manifestBytes)), Typeflag: tar.TypeReg, Format: tar.FormatUSTAR}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(manifestBytes); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.WriteHeader(&tar.Header{Name: "../escaped", Mode: 0o600, Size: 1, Typeflag: tar.TypeReg, Format: tar.FormatUSTAR}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	var ciphertext bytes.Buffer
	encrypted, err := age.Encrypt(&ciphertext, identity.Recipient())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := encrypted.Write(archive.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := encrypted.Close(); err != nil {
		t.Fatal(err)
	}
	cipherHash := sha256.Sum256(ciphertext.Bytes())
	manifestHash := sha256.Sum256(manifestBytes)
	fileID := "6f0d5b9a-61dc-4c53-b15e-64d46273eec1.age"
	spool := filepath.Join(t.TempDir(), "spool")
	if err := os.Mkdir(spool, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(spool, fileID), ciphertext.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	evidence := Evidence{BackupSetType: "full", ManifestSHA256: hex.EncodeToString(manifestHash[:]), ReadyFileID: fileID,
		CiphertextSHA256: hex.EncodeToString(cipherHash[:]), CiphertextSizeBytes: int64(ciphertext.Len()), EncryptionKeyID: "synthetic-test-key"}
	if err := writeReadySidecar(spool, evidence); err != nil {
		t.Fatal(err)
	}
	scratch := filepath.Join(t.TempDir(), "private-restore-scratch")
	if err := os.Mkdir(scratch, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", scratch)
	if _, err := RestoreDrill(context.Background(), spool, evidence, []age.Identity{identity}); err == nil || !strings.Contains(err.Error(), "unsafe member") {
		t.Fatalf("unsafe archive member accepted: %v", err)
	}
	entries, err := os.ReadDir(scratch)
	if err != nil || len(entries) != 0 {
		t.Fatalf("unsafe archive escaped or left a temporary workspace: entries=%d err=%v", len(entries), err)
	}
	if _, err := os.Stat(filepath.Join(scratch, "escaped")); !os.IsNotExist(err) {
		t.Fatalf("unsafe archive wrote outside private drill directory: %v", err)
	}
}

func TestSetRunnerRejectsTamperedLocalArchive(t *testing.T) {
	fx := newBackupFixture(t)
	runner := fixtureRunner(t, fx, "unused")
	tmp := t.TempDir()
	set, err := runner.capture(context.Background(), tmp)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(set.archivePath, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyArchive(set.archivePath, set); err == nil {
		t.Fatal("tampered archive passed local verification")
	}
}

func TestSetRunnerRejectsMissingConfigurationAndUnsafePhotoPaths(t *testing.T) {
	fx := newBackupFixture(t)
	runner := fixtureRunner(t, fx, "")
	if _, failure := runner.Run(context.Background(), Job{}); failure == nil || failure.Code != "storage_unconfigured" {
		t.Fatalf("missing recipient should fail closed, got %+v", failure)
	}
	if safeRelativePath("../outside.jpg") || safeRelativePath("/outside.jpg") || safeRelativePath("user\\photo.jpg") {
		t.Fatal("unsafe paths accepted")
	}
}

func TestSetRunnerRejectsSpoolInsideUploadsDirectory(t *testing.T) {
	fx := newBackupFixture(t)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	runner := fixtureRunner(t, fx, identity.Recipient().String())
	runner.SpoolDir = filepath.Join(fx.uploads, "backups")
	if evidence, failure := runner.Run(context.Background(), Job{}); evidence != nil || failure == nil || failure.Code != "storage_unconfigured" {
		t.Fatalf("spool inside uploads was accepted: evidence=%+v failure=%+v", evidence, failure)
	}
	if _, err := os.Lstat(runner.SpoolDir); !os.IsNotExist(err) {
		t.Fatalf("runner created an invalid spool inside uploads: %v", err)
	}
}

func TestSetRunnerRejectsSymlinkedPersistentSpoolEscape(t *testing.T) {
	fx := newBackupFixture(t)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	persistentRoot := filepath.Join(t.TempDir(), "persistent")
	if err := os.Mkdir(persistentRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	external := t.TempDir()
	if err := os.Symlink(external, filepath.Join(persistentRoot, "redirect")); err != nil {
		t.Fatal(err)
	}
	runner := fixtureRunner(t, fx, identity.Recipient().String())
	runner.PersistentRoot = persistentRoot
	runner.SpoolDir = filepath.Join(persistentRoot, "redirect", "backups")
	if evidence, failure := runner.Run(context.Background(), Job{}); evidence != nil || failure == nil || failure.Code != "storage_unconfigured" {
		t.Fatalf("symlink escape was accepted: evidence=%+v failure=%+v", evidence, failure)
	}
}

func TestPublishArtifactWritesSidecarLastAndRejectsCiphertextMismatch(t *testing.T) {
	spool := filepath.Join(t.TempDir(), "spool")
	if err := os.Mkdir(spool, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "cipher.age")
	ciphertext := []byte("synthetic-encrypted-backup")
	if err := os.WriteFile(source, ciphertext, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(ciphertext)
	fileID := "6f0d5b9a-61dc-4c53-b15e-64d46273eec1.age"
	if err := publishArtifact(context.Background(), spool, source, fileID, int64(len(ciphertext)), hex.EncodeToString(digest[:])); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(spool)
	if err != nil || len(entries) != 2 || entries[0].Name() != fileID && entries[1].Name() != fileID {
		t.Fatalf("artifact and ready sidecar were not both published: %#v, %v", entries, err)
	}
	markerBytes, err := os.ReadFile(filepath.Join(spool, strings.TrimSuffix(fileID, ".age")+".ready.json"))
	if err != nil {
		t.Fatal(err)
	}
	var marker readyManifest
	if err := json.Unmarshal(markerBytes, &marker); err != nil || marker.ArtifactFile != fileID || marker.CiphertextSizeBytes != int64(len(ciphertext)) || marker.CiphertextSHA256 != hex.EncodeToString(digest[:]) {
		t.Fatalf("unexpected ready marker: %+v (%v)", marker, err)
	}

	other := filepath.Join(t.TempDir(), "different.age")
	if err := os.WriteFile(other, []byte("different"), 0o600); err != nil {
		t.Fatal(err)
	}
	badSpool := filepath.Join(t.TempDir(), "bad-spool")
	if err := os.Mkdir(badSpool, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := publishArtifact(context.Background(), badSpool, other, fileID, int64(len(ciphertext)), hex.EncodeToString(digest[:])); err == nil {
		t.Fatal("publication accepted ciphertext inconsistent with expected hash and size")
	}
	entries, err = os.ReadDir(badSpool)
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed publication left a ready artifact: %#v, %v", entries, err)
	}
}

func TestVerifyPublishedArtifactChecksBytesAtFinalPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "artifact.age")
	want := []byte("published ciphertext")
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(want)
	hash := hex.EncodeToString(digest[:])
	if err := verifyPublishedArtifact(path, int64(len(want)), hash); err != nil {
		t.Fatalf("valid published ciphertext failed verification: %v", err)
	}
	if err := os.WriteFile(path, []byte("corrupted ciphertext"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyPublishedArtifact(path, int64(len(want)), hash); err == nil {
		t.Fatal("corrupted bytes at the final artifact path passed verification")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "outside.age"), path); err != nil {
		t.Fatal(err)
	}
	if err := verifyPublishedArtifact(path, int64(len(want)), hash); err == nil {
		t.Fatal("symlink at the final artifact path passed verification")
	}
}

func TestEnsurePrivateSpoolCreatesDurablePrivateDirectoryTree(t *testing.T) {
	spool := filepath.Join(t.TempDir(), "persistent", "backups")
	if err := ensurePrivateSpool(spool); err != nil {
		t.Fatalf("ensurePrivateSpool: %v", err)
	}
	info, err := os.Lstat(spool)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("spool was not created as a private directory: info=%v err=%v", info, err)
	}
}

func TestSetRunnerWaitsForSharedPhotoOperationBarrier(t *testing.T) {
	fx := newBackupFixture(t)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	barrier := &sync.RWMutex{}
	barrier.RLock() // represents an HTTP photo operation still in flight
	enteredSnapshot := make(chan struct{})
	runner := fixtureRunner(t, fx, identity.Recipient().String())
	runner.Barrier = barrier
	runner.Snapshot = func(ctx context.Context, source, dest string) error {
		close(enteredSnapshot)
		return onlineSQLiteBackup(ctx, source, dest)
	}
	done := make(chan *Error, 1)
	go func() {
		_, failure := runner.Run(context.Background(), Job{})
		done <- failure
	}()
	select {
	case <-enteredSnapshot:
		t.Fatal("backup began while a shared photo operation was active")
	case <-time.After(50 * time.Millisecond):
	}
	barrier.RUnlock()
	select {
	case <-enteredSnapshot:
	case <-time.After(2 * time.Second):
		t.Fatal("backup did not begin after the shared operation ended")
	}
	select {
	case failure := <-done:
		if failure != nil {
			t.Fatalf("backup failed: %s", failure.Code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("backup did not finish")
	}
}

type backupFixture struct {
	dbPath  string
	uploads string
}

func newBackupFixture(t *testing.T) *backupFixture {
	t.Helper()
	root := t.TempDir()
	fx := &backupFixture{dbPath: filepath.Join(root, "health.sqlite"), uploads: filepath.Join(root, "uploads")}
	if err := os.MkdirAll(filepath.Join(fx.uploads, "user-a/meal"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(fx.uploads, "user-a/calibration"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fx.uploads, "user-a/meal/meal.jpg"), []byte("synthetic-meal-photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fx.uploads, "user-a/calibration/sample.png"), []byte("synthetic-calibration-photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite3", fx.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range []string{
		"CREATE TABLE health_records (value TEXT NOT NULL)",
		"INSERT INTO health_records(value) VALUES ('synthetic-health-row')",
		"CREATE TABLE food_meals (photo_path TEXT NOT NULL)",
		"INSERT INTO food_meals(photo_path) VALUES ('user-a/meal/meal.jpg')",
		"CREATE TABLE food_calibration_samples (photo_path TEXT NOT NULL)",
		"INSERT INTO food_calibration_samples(photo_path) VALUES ('user-a/calibration/sample.png')",
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	return fx
}

func fixtureRunner(t *testing.T, fx *backupFixture, recipient string) *SetRunner {
	t.Helper()
	return &SetRunner{DatabasePath: fx.dbPath, UploadsDir: fx.uploads, SpoolDir: filepath.Join(filepath.Dir(fx.dbPath), "private-backups"),
		PersistentRoot: filepath.Dir(fx.dbPath), AgeRecipient: recipient,
		EncryptionID: "age-recipient-test-v1"}
}

func readArtifact(t *testing.T, runner *SetRunner, evidence Evidence) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(runner.SpoolDir, evidence.ReadyFileID))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func writeReadySidecar(spoolDir string, evidence Evidence) error {
	marker := readyManifest{ArtifactFile: evidence.ReadyFileID, CiphertextSizeBytes: evidence.CiphertextSizeBytes, CiphertextSHA256: evidence.CiphertextSHA256}
	encoded, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(spoolDir, strings.TrimSuffix(evidence.ReadyFileID, ".age")+".ready.json"), encoded, 0o600)
}

func readTarMember(t *testing.T, archive []byte, name string) []byte {
	t.Helper()
	reader := tar.NewReader(bytes.NewReader(archive))
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Name == name {
			data, err := io.ReadAll(reader)
			if err != nil {
				t.Fatal(err)
			}
			return data
		}
	}
	t.Fatalf("archive member %q not found", name)
	return nil
}
