package database_test

import (
	"log/slog"
	"os"
	"testing"

	"github.com/ya-breeze/healthvault/pkg/database"
)

func TestSeedUsers(t *testing.T) {
	db, err := database.Open(slog.New(slog.NewTextHandler(os.Stderr, nil)), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	store := database.NewStorage(db)

	if err := database.SeedUsers(db, "TestFamily:alice:pass1,TestFamily:bob:pass2"); err != nil {
		t.Fatalf("SeedUsers: %v", err)
	}

	alice, err := store.FindUserByName("alice")
	if err != nil {
		t.Fatalf("find alice: %v", err)
	}
	bob, err := store.FindUserByName("bob")
	if err != nil {
		t.Fatalf("find bob: %v", err)
	}
	if alice.FamilyID != bob.FamilyID {
		t.Error("alice and bob should be in the same family")
	}

	// Idempotent — seed again should not error or duplicate
	if err := database.SeedUsers(db, "TestFamily:alice:pass1"); err != nil {
		t.Fatalf("re-seed: %v", err)
	}
	users, _ := store.FindUsersByFamilyID(alice.FamilyID)
	if len(users) != 2 {
		t.Errorf("want 2 users, got %d", len(users))
	}
}

func TestSeedUsersEmptySpec(t *testing.T) {
	db, err := database.Open(slog.New(slog.NewTextHandler(os.Stderr, nil)), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SeedUsers(db, ""); err != nil {
		t.Fatalf("empty spec should not error: %v", err)
	}
}

func TestSeedUsersInvalidEntry(t *testing.T) {
	db, err := database.Open(slog.New(slog.NewTextHandler(os.Stderr, nil)), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SeedUsers(db, "badentry"); err == nil {
		t.Fatal("expected error for invalid entry format")
	}
}

func TestSeedUsersDifferentFamilies(t *testing.T) {
	db, err := database.Open(slog.New(slog.NewTextHandler(os.Stderr, nil)), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	store := database.NewStorage(db)

	if err := database.SeedUsers(db, "FamilyA:alice:pass1,FamilyB:carol:pass3"); err != nil {
		t.Fatalf("SeedUsers: %v", err)
	}

	alice, err := store.FindUserByName("alice")
	if err != nil {
		t.Fatalf("find alice: %v", err)
	}
	carol, err := store.FindUserByName("carol")
	if err != nil {
		t.Fatalf("find carol: %v", err)
	}
	if alice.FamilyID == carol.FamilyID {
		t.Error("alice and carol should be in different families")
	}
}
