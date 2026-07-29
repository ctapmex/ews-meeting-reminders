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

func TestDrainMeetingKeepsOtherMeetings(t *testing.T) {
	var dropped []string
	n := &Notifier{
		inbox: make(chan Item, 128),
		stop:  make(chan struct{}),
		opts: Options{
			OnDropped: func(key string) { dropped = append(dropped, key) },
		},
	}
	// Simulate: while m1@5 banner is open, poll enqueued m1@0; m2 and m3 wait too.
	for _, it := range []Item{
		{MeetingID: "m1", Key: "m1:0", Title: "m1-0"},
		{MeetingID: "m2", Key: "m2:5", Title: "m2"},
		{MeetingID: "m1", Key: "m1:extra", Title: "m1-extra"},
		{MeetingID: "m3", Key: "m3:5", Title: "m3"},
	} {
		n.queued.Add(1)
		n.inbox <- it
	}

	got := n.drainMeeting("m1")
	if got != 2 {
		t.Fatalf("dropped count: got %d want 2", got)
	}
	if len(dropped) != 2 || dropped[0] != "m1:0" || dropped[1] != "m1:extra" {
		t.Fatalf("OnDropped keys: %v", dropped)
	}
	if n.queued.Load() != 2 {
		t.Fatalf("queued: got %d want 2", n.queued.Load())
	}

	var left []string
	for range 2 {
		left = append(left, (<-n.inbox).MeetingID)
	}
	if left[0] != "m2" || left[1] != "m3" {
		t.Fatalf("remaining inbox: %v", left)
	}
	select {
	case it := <-n.inbox:
		t.Fatalf("inbox should be empty, got %+v", it)
	default:
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
