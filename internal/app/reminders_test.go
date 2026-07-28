package app

import (
	"testing"
	"time"

	"ews-meeting-reminders/internal/ews"
)

func TestCollectDueReminders_TwoSimultaneousMeetings(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.Local)
	meetings := []ews.Meeting{
		{
			ID:       "m1",
			Subject:  "Standup A",
			Start:    now.Add(5 * time.Minute),
			JoinURL:  "https://x.ktalk.ru/a",
			Response: "Accept",
		},
		{
			ID:       "m2",
			Subject:  "Standup B",
			Start:    now.Add(5 * time.Minute), // same start
			JoinURL:  "https://trueconf.x.com/c/b",
			Response: "Organizer",
		},
	}
	due := collectDueReminders(meetings, []int{5, 0}, now, 90, nil)
	if len(due) != 2 {
		t.Fatalf("want 2 due reminders (offset 5 for each), got %d: %+v", len(due), due)
	}
	got := map[string]string{}
	for _, d := range due {
		if d.Offset != 5 {
			t.Fatalf("unexpected offset %d for %s", d.Offset, d.Meeting.ID)
		}
		got[d.Meeting.ID] = d.Meeting.JoinURL
	}
	if got["m1"] != "https://x.ktalk.ru/a" || got["m2"] != "https://trueconf.x.com/c/b" {
		t.Fatalf("urls: %+v", got)
	}
}

func TestCollectDueReminders_ThreeSimultaneousMeetings(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.Local)
	start := now // offset 0 window
	meetings := []ews.Meeting{
		{ID: "a", Subject: "A", Start: start, JoinURL: "https://x.ktalk.ru/1"},
		{ID: "b", Subject: "B", Start: start, JoinURL: "https://trueconf.x.com/c/2"},
		{ID: "c", Subject: "C", Start: start, JoinURL: "https://zoom.us/j/3"},
	}
	due := collectDueReminders(meetings, []int{5, 0}, now, 90, nil)
	// At start: offset 0 fires for all three; offset 5 also matches remaining<=300
	// (remaining=0 <= 300 and > 300-90=210? 0 > 210 is false) so only offset 0.
	if len(due) != 3 {
		t.Fatalf("want 3 due at offset 0, got %d: %+v", len(due), due)
	}
	for _, d := range due {
		if d.Offset != 0 {
			t.Fatalf("want offset 0, got %d for %s", d.Offset, d.Meeting.ID)
		}
	}
}

func TestCollectDueReminders_SkipsAlreadySeen(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.Local)
	meetings := []ews.Meeting{
		{ID: "m1", Subject: "A", Start: now.Add(5 * time.Minute)},
		{ID: "m2", Subject: "B", Start: now.Add(5 * time.Minute)},
		{ID: "m3", Subject: "C", Start: now.Add(5 * time.Minute)},
	}
	seen := map[string]bool{"m1:5": true}
	due := collectDueReminders(meetings, []int{5}, now, 90, func(k string) bool {
		return seen[k]
	})
	if len(due) != 2 {
		t.Fatalf("want 2 (m2,m3), got %d: %+v", len(due), due)
	}
	for _, d := range due {
		if d.Meeting.ID == "m1" {
			t.Fatal("m1 should be skipped as already seen")
		}
	}
}

func TestCollectDueReminders_OnlyOneInWindowAmongThree(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.Local)
	meetings := []ews.Meeting{
		{ID: "soon", Subject: "Soon", Start: now.Add(5 * time.Minute)},
		{ID: "later", Subject: "Later", Start: now.Add(30 * time.Minute)},
		{ID: "far", Subject: "Far", Start: now.Add(2 * time.Hour)},
	}
	due := collectDueReminders(meetings, []int{5, 0}, now, 90, nil)
	if len(due) != 1 || due[0].Meeting.ID != "soon" || due[0].Offset != 5 {
		t.Fatalf("want only soon@5, got %+v", due)
	}
}

func TestFormatNotification_NoDuplicateCountdown(t *testing.T) {
	start := time.Date(2026, 7, 27, 12, 20, 0, 0, time.Local)
	m := ews.Meeting{
		Subject:  "Standup",
		Start:    start,
		Location: "https://trueconf.nexign.com/c/1",
		JoinURL:  "https://trueconf.nexign.com/c/1",
	}
	title, body := formatNotification(m, 5)
	if title != "Через 5 мин: Standup" {
		t.Fatalf("title: %q", title)
	}
	wantBody := "12:20\nhttps://trueconf.nexign.com/c/1"
	if body != wantBody {
		t.Fatalf("body: %q want %q", body, wantBody)
	}
	title0, body0 := formatNotification(ews.Meeting{Subject: "X", Start: start}, 0)
	if title0 != "Начало: X" || body0 != "12:20" {
		t.Fatalf("offset 0: title=%q body=%q", title0, body0)
	}
}
