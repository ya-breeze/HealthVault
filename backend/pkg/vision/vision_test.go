package vision_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ya-breeze/healthvault/pkg/vision"
)

func TestUnconfigured_RecognizeReturnsErrNotConfigured(t *testing.T) {
	var c vision.Unconfigured
	_, err := c.Recognize(context.Background(), []byte{1, 2, 3}, "image/jpeg", "")
	if !errors.Is(err, vision.ErrNotConfigured) {
		t.Errorf("err = %v, want ErrNotConfigured", err)
	}
}

func TestUnconfigured_SelectReturnsErrNotConfigured(t *testing.T) {
	var c vision.Unconfigured
	_, err := c.Select(context.Background(), []vision.ItemCandidates{{ItemIndex: 0}})
	if !errors.Is(err, vision.ErrNotConfigured) {
		t.Errorf("err = %v, want ErrNotConfigured", err)
	}
}

func TestFake_ZeroValueSucceedsWithEmptyResults(t *testing.T) {
	f := &vision.Fake{}
	rr, err := f.Recognize(context.Background(), nil, "image/jpeg", "")
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}
	if rr == nil || len(rr.Items) != 0 {
		t.Errorf("expected an empty RecognizeResult, got %+v", rr)
	}

	sr, err := f.Select(context.Background(), nil)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if sr == nil || len(sr.Selections) != 0 {
		t.Errorf("expected an empty SelectResult, got %+v", sr)
	}
}

func TestFake_RecordsCalls(t *testing.T) {
	f := &vision.Fake{}
	image := []byte{0xFF, 0xD8}
	if _, err := f.Recognize(context.Background(), image, "image/jpeg", "chicken not berries"); err != nil {
		t.Fatalf("Recognize: %v", err)
	}
	if len(f.RecognizeCalls) != 1 || f.RecognizeCalls[0].MimeType != "image/jpeg" {
		t.Errorf("expected one recorded Recognize call, got %+v", f.RecognizeCalls)
	}
	if f.RecognizeCalls[0].Hint != "chicken not berries" {
		t.Errorf("Hint = %q, want %q", f.RecognizeCalls[0].Hint, "chicken not berries")
	}

	ic := []vision.ItemCandidates{{ItemIndex: 0, Candidates: []vision.Candidate{{Description: "x"}}}}
	if _, err := f.Select(context.Background(), ic); err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(f.SelectCalls) != 1 || len(f.SelectCalls[0]) != 1 {
		t.Errorf("expected one recorded Select call, got %+v", f.SelectCalls)
	}
}

func TestFake_ReturnsConfiguredErrors(t *testing.T) {
	wantErr := errors.New("boom")
	f := &vision.Fake{RecognizeErr: wantErr, SelectErr: wantErr}

	if _, err := f.Recognize(context.Background(), nil, "", ""); !errors.Is(err, wantErr) {
		t.Errorf("Recognize err = %v, want %v", err, wantErr)
	}
	if _, err := f.Select(context.Background(), nil); !errors.Is(err, wantErr) {
		t.Errorf("Select err = %v, want %v", err, wantErr)
	}
}
