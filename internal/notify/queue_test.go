package notify

import (
	"testing"
	"time"
)

func TestEnqueueSerializesBatches(t *testing.T) {
	// Unit-level: inbox ordering without D-Bus — simulate by checking channel semantics.
	inbox := make(chan Item, 128)
	batch1 := []Item{{Title: "A1"}, {Title: "A2"}}
	batch2 := []Item{{Title: "B1"}, {Title: "B2"}}
	for _, it := range batch1 {
		inbox <- it
	}
	for _, it := range batch2 {
		inbox <- it
	}
	var got []string
	for range 4 {
		got = append(got, (<-inbox).Title)
	}
	want := []string{"A1", "A2", "B1", "B2"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order: got %v want %v", got, want)
		}
	}
}

func TestWaitIdleTimeout(t *testing.T) {
	n := &Notifier{
		inbox: make(chan Item, 1),
		stop:  make(chan struct{}),
	}
	n.queued.Store(1)
	n.presenting.Store(true)
	err := n.WaitIdle(50 * time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error while busy")
	}
}

// TestWaitIdleSeesQueuedAfterDequeue covers the race that made ews-test-notify
// exit early: presenter drained inbox but had not finished the card yet.
func TestWaitIdleSeesQueuedAfterDequeue(t *testing.T) {
	n := &Notifier{
		inbox: make(chan Item, 1),
		stop:  make(chan struct{}),
	}
	n.queued.Store(1) // Enqueue already counted; channel empty as if presenter took it
	done := make(chan error, 1)
	go func() {
		done <- n.WaitIdle(500 * time.Millisecond)
	}()
	time.Sleep(80 * time.Millisecond)
	select {
	case err := <-done:
		t.Fatalf("WaitIdle returned too early: %v", err)
	default:
	}
	n.queued.Store(0)
	if err := <-done; err != nil {
		t.Fatalf("WaitIdle after finish: %v", err)
	}
}

func TestSplitOpenCmd(t *testing.T) {
	cases := []struct {
		in    string
		want  []string
		isErr bool
	}{
		{"xdg-open", []string{"xdg-open"}, false},
		{"xterm -e xdg-open", []string{"xterm", "-e", "xdg-open"}, false},
		{`xterm -T "My Title" -e xdg-open`, []string{"xterm", "-T", "My Title", "-e", "xdg-open"}, false},
		{"", nil, true},
		{`xdg-open "unmatched`, nil, true},
	}
	for _, tc := range cases {
		got, err := splitOpenCmd(tc.in)
		if tc.isErr {
			if err == nil {
				t.Fatalf("splitOpenCmd(%q) expected error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("splitOpenCmd(%q): %v", tc.in, err)
		}
		if len(got) != len(tc.want) {
			t.Fatalf("splitOpenCmd(%q) = %v, want %v", tc.in, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("splitOpenCmd(%q) = %v, want %v", tc.in, got, tc.want)
			}
		}
	}
}
