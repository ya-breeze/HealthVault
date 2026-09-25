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

type memoryObjectStore struct {
	objects    map[string][]byte
	putErr     error
	getErr     error
	corruptGet bool
	puts       int
}

func (s *memoryObjectStore) Put(_ context.Context, key string, body io.Reader, size int64) error {
	s.puts++
	if s.putErr != nil {
		return s.putErr
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	if int64(len(data)) != size {
		return errors.New("wrong upload size")
	}
	if s.objects == nil {
		s.objects = make(map[string][]byte)
	}
	s.objects[key] = data
	return nil
}

func (s *memoryObjectStore) Get(_ context.Context, key string) (io.ReadCloser, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	data, ok := s.objects[key]
	if !ok {
		return nil, os.ErrNotExist
	}
	data = append([]byte(nil), data...)
	if s.corruptGet && len(data) > 0 {
		data[len(data)/2] ^= 0xff
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func TestSetRunnerCreatesEncryptedConsistentSetAndVerifiesRemoteBytes(t *testing.T) {
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
	store := &memoryObjectStore{}
	runner := fixtureRunner(fx, store, identity.Recipient().String())
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
	if len(store.objects) != 1 || store.puts != 1 {
		t.Fatalf("expected one verified object, got puts=%d objects=%d", store.puts, len(store.objects))
	}
	ciphertext := store.objects[evidence.RemoteObjectID]
	if bytes.Contains(ciphertext, []byte("synthetic-meal-photo")) || bytes.Contains(ciphertext, []byte("synthetic-health-row")) || bytes.Contains(ciphertext, []byte("fake-object-secret")) {
		t.Fatal("remote object contains plaintext or store credentials")
	}
	hash := sha256.Sum256(ciphertext)
	if hex.EncodeToString(hash[:]) != evidence.CiphertextSHA256 || int64(len(ciphertext)) != evidence.CiphertextSizeBytes {
		t.Fatal("evidence does not match remotely read ciphertext")
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
	store := &memoryObjectStore{}
	runner := fixtureRunner(fx, store, identity.Recipient().String())
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
	if store.puts != 1 {
		t.Fatalf("test did not reach remote verification: puts=%d", store.puts)
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
	store := &memoryObjectStore{}
	runner := fixtureRunner(fx, store, identity.Recipient().String())
	evidence, failure := runner.Run(context.Background(), Job{})
	if failure != nil || evidence == nil {
		t.Fatalf("fresh database failed backup: evidence=%+v failure=%+v", evidence, failure)
	}
	if _, err := RestoreDrill(context.Background(), store, evidence.RemoteObjectID, *evidence, []age.Identity{identity}); err != nil {
		t.Fatalf("fresh database did not restore: %v", err)
	}
}

func TestRestoreDrillIgnoresCallerTMPDIR(t *testing.T) {
	fx := newBackupFixture(t)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	store := &memoryObjectStore{}
	evidence, failure := fixtureRunner(fx, store, identity.Recipient().String()).Run(context.Background(), Job{})
	if failure != nil || evidence == nil {
		t.Fatalf("synthetic backup failed: %+v", failure)
	}
	// If the drill honored TMPDIR, it would fail to create a workspace here.
	// It must choose its own scratch root rather than the live data volume.
	t.Setenv("TMPDIR", filepath.Join(fx.uploads, "nonexistent"))
	if _, err := RestoreDrill(context.Background(), store, evidence.RemoteObjectID, *evidence, []age.Identity{identity}); err != nil {
		t.Fatalf("restore drill used caller TMPDIR: %v", err)
	}
}

func TestSetRunnerFailsClosedBeforeUpload(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*testing.T, *backupFixture, *memoryObjectStore, string) (*SetRunner, context.Context)
		wantCode  string
	}{
		{
			name: "referenced upload missing",
			configure: func(t *testing.T, fx *backupFixture, store *memoryObjectStore, recipient string) (*SetRunner, context.Context) {
				if err := os.Remove(filepath.Join(fx.uploads, "user-a/meal/meal.jpg")); err != nil {
					t.Fatal(err)
				}
				return fixtureRunner(fx, store, recipient), context.Background()
			},
			wantCode: "snapshot_failed",
		},
		{
			name: "interrupted snapshot",
			configure: func(t *testing.T, fx *backupFixture, store *memoryObjectStore, recipient string) (*SetRunner, context.Context) {
				runner := fixtureRunner(fx, store, recipient)
				runner.Snapshot = func(context.Context, string, string) error { return errors.New("private snapshot detail") }
				return runner, context.Background()
			},
			wantCode: "snapshot_failed",
		},
		{
			name: "encryption failure",
			configure: func(t *testing.T, fx *backupFixture, store *memoryObjectStore, _ string) (*SetRunner, context.Context) {
				return fixtureRunner(fx, store, "invalid-age-recipient"), context.Background()
			},
			wantCode: "encryption_failed",
		},
		{
			name: "upload failure",
			configure: func(t *testing.T, fx *backupFixture, store *memoryObjectStore, recipient string) (*SetRunner, context.Context) {
				store.putErr = errors.New("fake-object-secret upload detail")
				return fixtureRunner(fx, store, recipient), context.Background()
			},
			wantCode: "upload_failed",
		},
		{
			name: "remote ciphertext mismatch",
			configure: func(t *testing.T, fx *backupFixture, store *memoryObjectStore, recipient string) (*SetRunner, context.Context) {
				store.corruptGet = true
				return fixtureRunner(fx, store, recipient), context.Background()
			},
			wantCode: "remote_verification_failed",
		},
		{
			name: "cancelled snapshot",
			configure: func(t *testing.T, fx *backupFixture, store *memoryObjectStore, recipient string) (*SetRunner, context.Context) {
				runner := fixtureRunner(fx, store, recipient)
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
			store := &memoryObjectStore{}
			runner, ctx := tt.configure(t, fx, store, identity.Recipient().String())
			_, failure := runner.Run(ctx, Job{})
			if failure == nil || failure.Code != tt.wantCode {
				t.Fatalf("want failure %q, got %+v", tt.wantCode, failure)
			}
			if strings.Contains(failure.Message, "private snapshot detail") || strings.Contains(failure.Message, "fake-object-secret") {
				t.Fatalf("failure leaked sensitive details: %q", failure.Message)
			}
			if tt.name == "referenced upload missing" || tt.name == "interrupted snapshot" || tt.name == "encryption failure" || tt.name == "cancelled snapshot" {
				if store.puts != 0 {
					t.Fatalf("failed before upload but performed %d uploads", store.puts)
				}
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
	store := &memoryObjectStore{}
	runner := fixtureRunner(fx, store, identity.Recipient().String())
	evidence, failure := runner.Run(context.Background(), Job{})
	if failure != nil {
		t.Fatalf("create synthetic backup: %s", failure.Code)
	}

	scratch := filepath.Join(t.TempDir(), "private-restore-scratch")
	if err := os.Mkdir(scratch, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", scratch)
	report, err := RestoreDrill(context.Background(), store, evidence.RemoteObjectID, *evidence, []age.Identity{identity})
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

	originalCiphertext := append([]byte(nil), store.objects[evidence.RemoteObjectID]...)
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
	store.objects[evidence.RemoteObjectID] = changedCiphertext.Bytes()
	changedEvidence := *evidence
	changedEvidence.CiphertextSHA256 = hex.EncodeToString(changedHash[:])
	changedEvidence.CiphertextSizeBytes = int64(changedCiphertext.Len())
	if _, err := RestoreDrill(context.Background(), store, changedEvidence.RemoteObjectID, changedEvidence, []age.Identity{identity}); err == nil || err.Error() != "backup archive member checksum mismatch" {
		t.Fatalf("archive member corruption accepted: %v", err)
	}
	entries, err = os.ReadDir(scratch)
	if err != nil || len(entries) != 0 {
		t.Fatalf("member checksum failure left workspace behind: entries=%d err=%v", len(entries), err)
	}

	// Restore the genuine object before checking ciphertext-level corruption.
	store.objects[evidence.RemoteObjectID] = originalCiphertext
	store.objects[evidence.RemoteObjectID][0] ^= 0xff
	if _, err := RestoreDrill(context.Background(), store, evidence.RemoteObjectID, *evidence, []age.Identity{identity}); err == nil || err.Error() != "backup ciphertext verification failed" {
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
	objectID := "6f0d5b9a-61dc-4c53-b15e-64d46273eec1"
	store := &memoryObjectStore{objects: map[string][]byte{objectID: ciphertext.Bytes()}}
	evidence := Evidence{BackupSetType: "full", ManifestSHA256: hex.EncodeToString(manifestHash[:]), RemoteObjectID: objectID,
		CiphertextSHA256: hex.EncodeToString(cipherHash[:]), CiphertextSizeBytes: int64(ciphertext.Len()), EncryptionKeyID: "synthetic-test-key"}
	scratch := filepath.Join(t.TempDir(), "private-restore-scratch")
	if err := os.Mkdir(scratch, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", scratch)
	if _, err := RestoreDrill(context.Background(), store, objectID, evidence, []age.Identity{identity}); err == nil || !strings.Contains(err.Error(), "unsafe member") {
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
	runner := fixtureRunner(fx, &memoryObjectStore{}, "unused")
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
	store := &memoryObjectStore{}
	runner := fixtureRunner(fx, store, "")
	if _, failure := runner.Run(context.Background(), Job{}); failure == nil || failure.Code != "storage_unconfigured" {
		t.Fatalf("missing recipient should fail closed, got %+v", failure)
	}
	if safeRelativePath("../outside.jpg") || safeRelativePath("/outside.jpg") || safeRelativePath("user\\photo.jpg") {
		t.Fatal("unsafe paths accepted")
	}
}

func TestS3CompatibleStoreRequiresCredentialFreeHTTPSOrigin(t *testing.T) {
	for _, endpoint := range []string{
		"http://objects.example.invalid",
		"https://user:password@objects.example.invalid",
		"https://objects.example.invalid/prefix",
		"https://objects.example.invalid/?token=secret",
	} {
		if _, err := NewS3CompatibleStore(endpoint, "bucket", "access", "secret", "region"); err == nil {
			t.Errorf("unsafe endpoint accepted: %q", endpoint)
		}
	}
	if _, err := NewS3CompatibleStore("https://objects.example.invalid", "bucket", "access", "secret", "region"); err != nil {
		t.Fatalf("valid HTTPS S3 origin rejected: %v", err)
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
	runner := fixtureRunner(fx, &memoryObjectStore{}, identity.Recipient().String())
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

func fixtureRunner(fx *backupFixture, store *memoryObjectStore, recipient string) *SetRunner {
	return &SetRunner{DatabasePath: fx.dbPath, UploadsDir: fx.uploads, AgeRecipient: recipient,
		EncryptionID: "age-recipient-test-v1", Store: store}
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
