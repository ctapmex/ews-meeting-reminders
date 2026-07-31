package app

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"ews-meeting-reminders/internal/ews"
	"ews-meeting-reminders/internal/state"
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
	due := collectDueReminders(meetings, []int{5, 0}, now, 90, nil, nil)
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
	due := collectDueReminders(meetings, []int{5, 0}, now, 90, nil, nil)
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

type mapGate struct {
	seen        map[string]bool
	dismissed   map[string]bool
	snoozeUntil map[string]time.Time
}

func (g mapGate) Seen(key string) bool       { return g.seen[key] }
func (g mapGate) IsDismissed(id string) bool { return g.dismissed[id] }
func (g mapGate) SnoozeUntil(id string) (time.Time, bool) {
	t, ok := g.snoozeUntil[id]
	return t, ok
}

func TestCollectDueReminders_SkipsAlreadySeen(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.Local)
	meetings := []ews.Meeting{
		{ID: "m1", Subject: "A", Start: now.Add(5 * time.Minute)},
		{ID: "m2", Subject: "B", Start: now.Add(5 * time.Minute)},
		{ID: "m3", Subject: "C", Start: now.Add(5 * time.Minute)},
	}
	gate := mapGate{seen: map[string]bool{"m1:5": true}}
	due := collectDueReminders(meetings, []int{5}, now, 90, gate, nil)
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
	due := collectDueReminders(meetings, []int{5, 0}, now, 90, nil, nil)
	if len(due) != 1 || due[0].Meeting.ID != "soon" || due[0].Offset != 5 {
		t.Fatalf("want only soon@5, got %+v", due)
	}
}

func TestCollectDueReminders_DismissedSuppressesAll(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.Local)
	meetings := []ews.Meeting{
		{ID: "m1", Subject: "A", Start: now.Add(5 * time.Minute)},
		{ID: "m2", Subject: "B", Start: now.Add(5 * time.Minute)},
	}
	gate := mapGate{dismissed: map[string]bool{"m1": true}}
	due := collectDueReminders(meetings, []int{5, 0}, now, 90, gate, nil)
	if len(due) != 1 || due[0].Meeting.ID != "m2" {
		t.Fatalf("want only m2, got %+v", due)
	}
}

func TestCollectDueReminders_ActiveSnoozeSuppressesOffsets(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.Local)
	meetings := []ews.Meeting{
		{ID: "m1", Subject: "A", Start: now.Add(5 * time.Minute)},
	}
	gate := mapGate{snoozeUntil: map[string]time.Time{"m1": now.Add(3 * time.Minute)}}
	due := collectDueReminders(meetings, []int{5, 0}, now, 90, gate, nil)
	if len(due) != 0 {
		t.Fatalf("want no reminders while snoozed, got %+v", due)
	}
}

func TestCollectDueReminders_ExpiredSnoozeFiresOnce(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.Local)
	until := now.Add(-1 * time.Minute)
	meetings := []ews.Meeting{
		{ID: "m1", Subject: "A", Start: now.Add(4 * time.Minute)},
	}
	gate := mapGate{snoozeUntil: map[string]time.Time{"m1": until}}
	due := collectDueReminders(meetings, []int{5, 0}, now, 90, gate, nil)
	if len(due) != 1 || !due[0].Snooze {
		t.Fatalf("want one snooze reminder, got %+v", due)
	}
	wantKey := state.SnoozeCycleKey("m1", until)
	if due[0].Key != wantKey {
		t.Fatalf("key: got %q want %q", due[0].Key, wantKey)
	}
}

func TestCollectDueReminders_SnoozeFrom10GoesToNextOffset(t *testing.T) {
	// offsets [10,8,5,0]: snooze on the T-10 card → until next unseen offset (T-8),
	// not snooze_minutes (which would be T-5).
	start := time.Date(2026, 7, 23, 12, 10, 0, 0, time.Local)
	offsets := []int{10, 8, 5, 0}
	m := ews.Meeting{ID: "m1", Subject: "A", Start: start}

	t10 := start.Add(-10 * time.Minute)
	until := computeSnoozeUntil(t10, start, 5*time.Minute, offsets, func(o int) bool {
		return o == 10
	})
	t8 := start.Add(-8 * time.Minute)
	if !until.Equal(t8) {
		t.Fatalf("until: got %v want T-8 %v", until, t8)
	}

	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "shown.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Mark("m1:10"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetSnooze("m1", until); err != nil {
		t.Fatal(err)
	}
	if err := markOffsetsCoveredBySnooze(store, "m1", start, offsets, until); err != nil {
		t.Fatal(err)
	}

	// During wait before T-8: nothing
	if due := collectDueReminders([]ews.Meeting{m}, offsets, t8.Add(-time.Minute), 90, store, nil); len(due) != 0 {
		t.Fatalf("before T-8 want 0, got %+v", due)
	}

	// At T-8: snooze follow-up only (offset 8 covered)
	due := collectDueReminders([]ews.Meeting{m}, offsets, t8, 90, store, nil)
	if len(due) != 1 || !due[0].Snooze {
		t.Fatalf("at T-8 want one snooze, got %+v", due)
	}
	if err := store.Mark(due[0].Key); err != nil {
		t.Fatal(err)
	}
	if err := store.ClearSnooze("m1"); err != nil {
		t.Fatal(err)
	}

	// Later offset 5 still fires
	t5 := start.Add(-5 * time.Minute)
	due = collectDueReminders([]ews.Meeting{m}, offsets, t5, 90, store, nil)
	if len(due) != 1 || due[0].Offset != 5 {
		t.Fatalf("at T-5 want offset 5, got %+v", due)
	}

	// At start: offset 0
	_ = store.Mark("m1:5")
	due = collectDueReminders([]ews.Meeting{m}, offsets, start, 90, store, nil)
	if len(due) != 1 || due[0].Offset != 0 {
		t.Fatalf("at start want offset 0, got %+v", due)
	}
}

func TestMarkOffsetsCoveredBySnooze(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "shown.json"))
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 7, 23, 12, 10, 0, 0, time.Local)
	until := start.Add(-5 * time.Minute)
	if err := markOffsetsCoveredBySnooze(store, "m1", start, []int{10, 8, 5, 0}, until); err != nil {
		t.Fatal(err)
	}
	for _, o := range []int{10, 8, 5} {
		if !store.Seen(fmt.Sprintf("m1:%d", o)) {
			t.Fatalf("offset %d should be covered", o)
		}
	}
	if store.Seen("m1:0") {
		t.Fatal("offset 0 should remain for later")
	}
}

func TestCollectDueReminders_SnoozeAfterStartStillFires(t *testing.T) {
	// Offset 0 at meeting start → snooze 5m → follow-up after start is still due
	// as long as the meeting remains in the fetched list.
	start := time.Date(2026, 7, 23, 12, 0, 0, 0, time.Local)
	offsets := []int{5, 0}
	m := ews.Meeting{ID: "m1", Subject: "A", Start: start}

	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "shown.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Mark("m1:0"); err != nil {
		t.Fatal(err)
	}
	until := start.Add(5 * time.Minute) // T+5
	if err := store.SetSnooze("m1", until); err != nil {
		t.Fatal(err)
	}
	if err := markOffsetsCoveredBySnooze(store, "m1", start, offsets, until); err != nil {
		t.Fatal(err)
	}

	// While waiting past start: nothing
	tPlus2 := start.Add(2 * time.Minute)
	if due := collectDueReminders([]ews.Meeting{m}, offsets, tPlus2, 90, store, nil); len(due) != 0 {
		t.Fatalf("at T+2 want 0, got %+v", due)
	}

	// At T+5: snooze follow-up even though meeting already started
	due := collectDueReminders([]ews.Meeting{m}, offsets, until, 90, store, nil)
	if len(due) != 1 || !due[0].Snooze {
		t.Fatalf("at T+5 want snooze after start, got %+v", due)
	}
}

func TestCollectDueReminders_ChainedSnoozeAfterStart(t *testing.T) {
	// Snooze at T+5, then snooze again → another follow-up at T+10.
	start := time.Date(2026, 7, 23, 12, 0, 0, 0, time.Local)
	offsets := []int{5, 0}
	m := ews.Meeting{ID: "m1", Subject: "A", Start: start}

	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "shown.json"))
	if err != nil {
		t.Fatal(err)
	}
	_ = store.Mark("m1:5")
	_ = store.Mark("m1:0")

	until1 := start.Add(5 * time.Minute)
	if err := store.SetSnooze("m1", until1); err != nil {
		t.Fatal(err)
	}
	due := collectDueReminders([]ews.Meeting{m}, offsets, until1, 90, store, nil)
	if len(due) != 1 || !due[0].Snooze {
		t.Fatalf("first snooze: %+v", due)
	}
	_ = store.Mark(due[0].Key)
	_ = store.ClearSnooze("m1")

	// Second snooze from the follow-up card
	until2 := until1.Add(5 * time.Minute) // T+10
	if err := store.SetSnooze("m1", until2); err != nil {
		t.Fatal(err)
	}
	if err := markOffsetsCoveredBySnooze(store, "m1", start, offsets, until2); err != nil {
		t.Fatal(err)
	}

	tPlus7 := start.Add(7 * time.Minute)
	if due := collectDueReminders([]ews.Meeting{m}, offsets, tPlus7, 90, store, nil); len(due) != 0 {
		t.Fatalf("during second wait want 0, got %+v", due)
	}

	due = collectDueReminders([]ews.Meeting{m}, offsets, until2, 90, store, nil)
	if len(due) != 1 || !due[0].Snooze {
		t.Fatalf("chained snooze at T+10: %+v", due)
	}
	if due[0].Key == state.SnoozeCycleKey("m1", until1) {
		t.Fatal("second cycle must use a distinct key from the first")
	}
	if due[0].Key != state.SnoozeCycleKey("m1", until2) {
		t.Fatalf("key: got %q want %q", due[0].Key, state.SnoozeCycleKey("m1", until2))
	}
}

func TestCollectDueReminders_LegacySeenKeysStillWork(t *testing.T) {
	// Old installs only have "id:offset" shown timestamps — no dismissed/snooze keys.
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.Local)
	meetings := []ews.Meeting{
		{ID: "m1", Subject: "A", Start: now.Add(5 * time.Minute)},
	}
	gate := mapGate{seen: map[string]bool{"m1:5": true}}
	due := collectDueReminders(meetings, []int{5, 0}, now, 90, gate, nil)
	if len(due) != 0 {
		t.Fatalf("legacy seen key should suppress offset 5, got %+v", due)
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
	title, body := formatNotification(m, 5, false)
	if title != "Через 5 мин: Standup" {
		t.Fatalf("title: %q", title)
	}
	wantBody := "12:20\nhttps://trueconf.nexign.com/c/1"
	if body != wantBody {
		t.Fatalf("body: %q want %q", body, wantBody)
	}
	title0, body0 := formatNotification(ews.Meeting{Subject: "X", Start: start}, 0, false)
	if title0 != "Начало: X" || body0 != "12:20" {
		t.Fatalf("offset 0: title=%q body=%q", title0, body0)
	}
	titleS, _ := formatNotification(m, -1, true)
	if titleS != "Напоминание: Standup" {
		t.Fatalf("snooze title: %q", titleS)
	}
}

func TestComputeSnoozeUntil(t *testing.T) {
	snoozeFor := 5 * time.Minute
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.Local)
	noneSeen := func(int) bool { return false }

	t.Run("next unseen offset wins over snooze_minutes", func(t *testing.T) {
		start := now.Add(10 * time.Minute)
		offsets := []int{10, 8, 5, 0}
		got := computeSnoozeUntil(now, start, snoozeFor, offsets, func(o int) bool {
			return o == 10 // just shown
		})
		want := start.Add(-8 * time.Minute)
		if !got.Equal(want) {
			t.Fatalf("got %v want next offset %v", got, want)
		}
	})

	t.Run("no remaining offsets uses full snooze when enough time", func(t *testing.T) {
		// Only offset was 10, already seen; now at T-9 with start in 9m — no future offsets.
		start := now.Add(9 * time.Minute)
		got := computeSnoozeUntil(now, start, snoozeFor, []int{10}, func(o int) bool {
			return true
		})
		want := now.Add(snoozeFor)
		if !got.Equal(want) {
			t.Fatalf("got %v want %v", got, want)
		}
	})

	t.Run("no remaining offsets clamps to start when less than snooze", func(t *testing.T) {
		start := now.Add(3 * time.Minute)
		got := computeSnoozeUntil(now, start, snoozeFor, []int{10, 5}, func(o int) bool {
			return true
		})
		if !got.Equal(start) {
			t.Fatalf("got %v want start %v", got, start)
		}
	})

	t.Run("last offset zero is next when earlier seen", func(t *testing.T) {
		start := now.Add(3 * time.Minute)
		got := computeSnoozeUntil(now, start, snoozeFor, []int{5, 0}, func(o int) bool {
			return o == 5
		})
		if !got.Equal(start) {
			t.Fatalf("got %v want start %v", got, start)
		}
	})

	t.Run("after start keeps full snooze", func(t *testing.T) {
		start := now.Add(-2 * time.Minute)
		got := computeSnoozeUntil(now, start, snoozeFor, []int{5, 0}, noneSeen)
		want := now.Add(snoozeFor)
		if !got.Equal(want) {
			t.Fatalf("got %v want %v", got, want)
		}
	})

	t.Run("zero start keeps full snooze", func(t *testing.T) {
		got := computeSnoozeUntil(now, time.Time{}, snoozeFor, []int{5, 0}, noneSeen)
		want := now.Add(snoozeFor)
		if !got.Equal(want) {
			t.Fatalf("got %v want %v", got, want)
		}
	})
}

func TestCollectDueReminders_SnoozeClampedToStart(t *testing.T) {
	// Snooze at T-3: next unseen offset is 0 → until = start.
	start := time.Date(2026, 7, 23, 12, 0, 0, 0, time.Local)
	now := start.Add(-3 * time.Minute)
	offsets := []int{5, 0}
	until := computeSnoozeUntil(now, start, 5*time.Minute, offsets, func(o int) bool {
		return o == 5
	})
	if !until.Equal(start) {
		t.Fatalf("until: got %v want %v", until, start)
	}

	m := ews.Meeting{ID: "m1", Subject: "A", Start: start}
	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "shown.json"))
	if err != nil {
		t.Fatal(err)
	}
	_ = store.Mark("m1:5")
	if err := store.SetSnooze("m1", until); err != nil {
		t.Fatal(err)
	}
	if err := markOffsetsCoveredBySnooze(store, "m1", start, offsets, until); err != nil {
		t.Fatal(err)
	}

	// While waiting (T-1): nothing
	if due := collectDueReminders([]ews.Meeting{m}, offsets, start.Add(-time.Minute), 90, store, nil); len(due) != 0 {
		t.Fatalf("during wait want 0, got %+v", due)
	}

	// At start: snooze follow-up (offset 0 covered, not a second card)
	due := collectDueReminders([]ews.Meeting{m}, offsets, start, 90, store, nil)
	if len(due) != 1 || !due[0].Snooze {
		t.Fatalf("at start want one snooze, got %+v", due)
	}
}

func TestTruncateToLocalMinute(t *testing.T) {
	in := time.Date(2026, 7, 23, 12, 30, 45, 123, time.Local)
	got := truncateToLocalMinute(in)
	want := time.Date(2026, 7, 23, 12, 30, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestCollectDueReminders_KnownOffset0FiresAtMinuteStart(t *testing.T) {
	// Meeting starts at 12:30:45; known from a previous poll → offset 0 at 12:30:00.
	start := time.Date(2026, 7, 23, 12, 30, 45, 0, time.Local)
	m := ews.Meeting{ID: "m1", Subject: "A", Start: start}
	known := func(id string) bool { return id == "m1" }

	atMinute := time.Date(2026, 7, 23, 12, 30, 0, 0, time.Local)
	due := collectDueReminders([]ews.Meeting{m}, []int{5, 0}, atMinute, 90, nil, known)
	if len(due) != 1 || due[0].Offset != 0 {
		t.Fatalf("known at :00 want offset 0, got %+v", due)
	}

	// Same meeting first seen this poll (not known): not yet at start → no offset 0.
	due = collectDueReminders([]ews.Meeting{m}, []int{5, 0}, atMinute, 90, nil, nil)
	if len(due) != 0 {
		t.Fatalf("unknown at :00 want 0, got %+v", due)
	}

	// Unknown fires at actual start.
	due = collectDueReminders([]ews.Meeting{m}, []int{0}, start, 90, nil, nil)
	if len(due) != 1 || due[0].Offset != 0 {
		t.Fatalf("unknown at actual start want offset 0, got %+v", due)
	}
}

func TestNextPollWait(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 29, 40, 0, time.Local)
	start := time.Date(2026, 7, 23, 12, 30, 15, 0, time.Local)
	var mem meetingMemory
	mem.Refresh([]ews.Meeting{{ID: "m1", Start: start}})

	maxWait := 30 * time.Second
	d := nextPollWait(now, maxWait, &mem, true, nil)
	want := time.Date(2026, 7, 23, 12, 30, 0, 0, time.Local).Sub(now) // 20s
	if d != want {
		t.Fatalf("offset0 wait: got %v want %v", d, want)
	}

	// Already past minute start → no shortened wait
	d = nextPollWait(time.Date(2026, 7, 23, 12, 30, 5, 0, time.Local), maxWait, &mem, true, nil)
	if d != 0 {
		t.Fatalf("past minute want 0, got %v", d)
	}
}

func TestNextPollWait_SnoozeTakesPriority(t *testing.T) {
	// Active snooze suppresses offset-0 wake for that meeting; snooze until must still wake us.
	now := time.Date(2026, 7, 23, 12, 29, 40, 0, time.Local)
	start := time.Date(2026, 7, 23, 12, 30, 0, 0, time.Local)
	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "shown.json"))
	if err != nil {
		t.Fatal(err)
	}
	var mem meetingMemory
	mem.Refresh([]ews.Meeting{{ID: "m1", Start: start}})
	if err := store.SetSnooze("m1", start); err != nil {
		t.Fatal(err)
	}

	maxWait := 60 * time.Second
	d := nextPollWait(now, maxWait, &mem, true, store)
	want := start.Sub(now) // 20s to snooze-until (== start)
	if d != want {
		t.Fatalf("snooze wait: got %v want %v", d, want)
	}
}

func TestMeetingIDFromSnoozeCycleKey(t *testing.T) {
	id, ok := meetingIDFromSnoozeCycleKey("abc:snooze:1710000000")
	if !ok || id != "abc" {
		t.Fatalf("got %q %v", id, ok)
	}
	if _, ok := meetingIDFromSnoozeCycleKey("abc:5"); ok {
		t.Fatal("offset key should not parse as snooze cycle")
	}
	if _, ok := meetingIDFromSnoozeCycleKey("abc:snooze"); ok {
		t.Fatal("schedule key should not parse as snooze cycle")
	}
}
