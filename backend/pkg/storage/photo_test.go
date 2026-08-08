package storage_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/ya-breeze/healthvault/pkg/storage"
)

var (
	jpegBytes = append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, bytes.Repeat([]byte{0}, 32)...)
	pngBytes  = append([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, bytes.Repeat([]byte{0}, 32)...)
	webpBytes = func() []byte {
		b := make([]byte, 40)
		copy(b[0:4], "RIFF")
		copy(b[8:12], "WEBP")
		return b
	}()
	heicBytes = func() []byte {
		b := make([]byte, 40)
		copy(b[4:8], "ftyp")
		copy(b[8:12], "heic")
		return b
	}()
)

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		name    string
		in      []byte
		want    string
		wantErr error
	}{
		{"jpeg", jpegBytes, storage.FormatJPEG, nil},
		{"png", pngBytes, storage.FormatPNG, nil},
		{"webp", webpBytes, storage.FormatWebP, nil},
		{"heic is named, not generic", heicBytes, "", storage.ErrHEIC},
		{"text is not an image", []byte("not an image at all"), "", storage.ErrUnsupportedFormat},
		{"empty", nil, "", storage.ErrUnsupportedFormat},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := storage.DetectFormat(tc.in)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("format = %q, want %q", got, tc.want)
			}
		})
	}
}

// Every stored photo must be a format the vision provider can read, or it would
// fail analysis and every retry forever.
func TestSave_RejectsHEIC(t *testing.T) {
	s := storage.New(t.TempDir())
	_, err := s.Save(bytes.NewReader(heicBytes), 1<<20, uuid.New(), storage.OwnerMeal, uuid.New())
	if !errors.Is(err, storage.ErrHEIC) {
		t.Fatalf("err = %v, want ErrHEIC", err)
	}
}

func TestSave_RejectsOversize(t *testing.T) {
	s := storage.New(t.TempDir())
	big := append(jpegBytes, bytes.Repeat([]byte{1}, 500)...)
	_, err := s.Save(bytes.NewReader(big), 100, uuid.New(), storage.OwnerMeal, uuid.New())
	if !errors.Is(err, storage.ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

func TestSave_NothingWrittenWhenRejected(t *testing.T) {
	dir := t.TempDir()
	s := storage.New(dir)
	_, err := s.Save(bytes.NewReader([]byte("nope")), 1<<20, uuid.New(), storage.OwnerMeal, uuid.New())
	if err == nil {
		t.Fatal("expected rejection")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("store root has %d entries after a rejected upload, want 0", len(entries))
	}
}

// The stored path is derived only from server-held IDs, so a hostile filename
// has no way into it.
func TestSave_PathIsServerGenerated(t *testing.T) {
	dir := t.TempDir()
	s := storage.New(dir)
	userID, mealID := uuid.New(), uuid.New()

	rel, err := s.Save(bytes.NewReader(jpegBytes), 1<<20, userID, storage.OwnerMeal, mealID)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	want := filepath.Join(userID.String(), storage.OwnerMeal, mealID.String()+".jpg")
	if rel != want {
		t.Errorf("rel = %q, want %q", rel, want)
	}
	if strings.Contains(rel, "..") {
		t.Error("stored path contains a traversal segment")
	}
	if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
		t.Errorf("stored file missing: %v", err)
	}
}

func TestSave_ExtensionFollowsSniffedFormat(t *testing.T) {
	s := storage.New(t.TempDir())
	rel, err := s.Save(bytes.NewReader(pngBytes), 1<<20, uuid.New(), storage.OwnerMeal, uuid.New())
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if filepath.Ext(rel) != ".png" {
		t.Errorf("ext = %q, want .png", filepath.Ext(rel))
	}
}

func TestRemove_MissingFileIsNotAnError(t *testing.T) {
	s := storage.New(t.TempDir())
	if err := s.Remove(filepath.Join("nobody", "meal", "gone.jpg")); err != nil {
		t.Errorf("Remove of a missing file returned %v, want nil", err)
	}
	if err := s.Remove(""); err != nil {
		t.Errorf("Remove of an empty path returned %v, want nil", err)
	}
}

func TestReadRoundTrip(t *testing.T) {
	s := storage.New(t.TempDir())
	rel, err := s.Save(bytes.NewReader(jpegBytes), 1<<20, uuid.New(), storage.OwnerCalibration, uuid.New())
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Read(rel)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !bytes.Equal(got, jpegBytes) {
		t.Error("round-tripped bytes differ from what was saved")
	}
}
