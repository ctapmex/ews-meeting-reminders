package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStore_LegacyShownKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shown.json")
	// Simulate pre-upgrade state file.
	if err := os.WriteFile(path, []byte(`{"meet-1:5":"2026-07-29T10:00:00Z"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Seen("meet-1:5") {
		t.Fatal("legacy key should be seen")
	}
	if s.IsDismissed("meet-1") {
		t.Fatal("should not be dismissed")
	}
	if _, ok := s.SnoozeUntil("meet-1"); ok {
		t.Fatal("should have no snooze")
	}
}

func TestStore_DismissAndSnooze(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "shown.json"))
	if err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(5 * time.Minute).Truncate(time.Second)
	if err := s.SetSnooze("m1", until); err != nil {
		t.Fatal(err)
	}
	got, ok := s.SnoozeUntil("m1")
	if !ok || !got.Equal(until) {
		t.Fatalf("snooze: got %v %v want %v", got, ok, until)
	}
	if err := s.MarkDismissed("m1"); err != nil {
		t.Fatal(err)
	}
	if !s.IsDismissed("m1") {
		t.Fatal("expected dismissed")
	}
	if _, ok := s.SnoozeUntil("m1"); ok {
		t.Fatal("dismiss should clear snooze")
	}
}

func TestStore_PruneKeepsFutureSnooze(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "shown.json"))
	if err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(10 * time.Minute).Truncate(time.Second)
	if err := s.SetSnooze("m1", until); err != nil {
		t.Fatal(err)
	}
	// Old shown entry
	s.mu.Lock()
	s.data["old:5"] = time.Now().Add(-48 * time.Hour).Format(time.RFC3339)
	_ = s.saveLocked()
	s.mu.Unlock()

	if err := s.Prune(24 * time.Hour); err != nil {
		t.Fatal(err)
	}
	if s.Seen("old:5") {
		t.Fatal("old entry should be pruned")
	}
	got, ok := s.SnoozeUntil("m1")
	if !ok || !got.Equal(until) {
		t.Fatalf("future snooze should be kept: %v %v", got, ok)
	}
}
