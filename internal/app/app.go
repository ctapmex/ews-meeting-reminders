package app

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"ews-meeting-reminders/internal/config"
	"ews-meeting-reminders/internal/ews"
	"ews-meeting-reminders/internal/notify"
	"ews-meeting-reminders/internal/state"
)

type Options struct {
	ConfigPath string
	StatePath  string
	Once       bool
	List       bool // print upcoming meetings (24h) to stdout and exit
}

// TestNotifyOptions configures the standalone notification smoke-test.
type TestNotifyOptions struct {
	ConfigPath string
	URL        string
	Wait       time.Duration
	Count      int // how many queued test notifications to show (1-5)
}

func Run(opts Options) error {
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return err
	}

	client := ews.New(
		cfg.Server, cfg.Email, cfg.Username, cfg.Password, cfg.Auth,
		cfg.VerifySSL, cfg.JoinHosts, cfg.IncludeResponseTypes,
	)

	if opts.List {
		return ListUpcoming(client, os.Stdout, 24*time.Hour)
	}

	store, err := state.Open(opts.StatePath)
	if err != nil {
		return err
	}

	n, err := notify.New(notifyOptions(cfg, store))
	if err != nil {
		return err
	}
	defer n.Close()

	log.Printf("connected as %s; offsets=%v poll=%ds snooze=%dm",
		cfg.Email, cfg.OffsetsMinutes, cfg.PollSeconds, cfg.SnoozeMinutes)

	if opts.Once {
		var known meetingMemory
		fired, err := processOnce(client, cfg, store, n, &known)
		if err != nil {
			return err
		}
		log.Printf("pass done, notifications=%d", fired)
		return nil
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	var known meetingMemory
	hasOffset0 := false
	for _, o := range cfg.OffsetsMinutes {
		if o == 0 {
			hasOffset0 = true
			break
		}
	}
	for {
		if err := store.Prune(time.Duration(cfg.StateKeepHours) * time.Hour); err != nil {
			log.Printf("state prune: %v", err)
		}
		if _, err := processOnce(client, cfg, store, n, &known); err != nil {
			log.Printf("ews fetch: %v", err)
		}
		wait := time.Duration(cfg.PollSeconds) * time.Second
		if d := nextOffset0Wait(time.Now(), wait, &known, hasOffset0, store); d > 0 {
			wait = d
			log.Printf("next poll in %s (aligned to offset 0)", wait.Round(time.Millisecond))
		}
		timer := time.NewTimer(wait)
		select {
		case <-stop:
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

// ListUpcoming fetches meetings in [now, now+window] and prints them to w.
func ListUpcoming(client *ews.Client, w io.Writer, window time.Duration) error {
	now := time.Now()
	end := now.Add(window)
	meetings, err := client.CalendarView(now, end)
	if err != nil {
		return err
	}
	sort.Slice(meetings, func(i, j int) bool {
		return meetings[i].Start.Before(meetings[j].Start)
	})

	fmt.Fprintf(w, "Встречи на ближайшие %d часа (с %s по %s), всего: %d\n",
		int(window.Hours()), now.Format("15:04 02.01"), end.Format("15:04 02.01"), len(meetings))
	if len(meetings) == 0 {
		return nil
	}
	fmt.Fprintln(w)
	for _, m := range meetings {
		url := m.JoinURL
		if url == "" {
			url = "—"
		}
		fmt.Fprintf(w, "%s  %s\n", m.Start.Format("02.01 15:04"), m.Subject)
		fmt.Fprintf(w, "         url: %s\n", url)
		if m.Location != "" && m.Location != m.JoinURL {
			fmt.Fprintf(w, "         место: %s\n", m.Location)
		}
		fmt.Fprintln(w)
	}
	return nil
}

// RunTestNotify shows reminders one-by-one (Open / Snooze / Stop) — GNOME/Cinnamon only show one banner.
func RunTestNotify(opts TestNotifyOptions) error {
	cfg, err := config.LoadNotifyPrefs(opts.ConfigPath)
	if err != nil {
		return err
	}
	n, err := notify.New(notifyOptions(cfg, nil))
	if err != nil {
		return err
	}
	defer n.Close()

	url := opts.URL
	if url == "" {
		url = "https://x.ktalk.ru/111"
	}
	wait := opts.Wait
	if wait <= 0 {
		wait = 5 * time.Minute
	}
	count := min(max(opts.Count, 1), 5)

	now := time.Now()
	urls := []string{
		url,
		"https://trueconf.x.com/c/test-2",
		"https://us05web.zoom.us/j/111111111",
		"https://x.ktalk.ru/room-3",
		"https://trueconf.x.com/c/test-5",
	}
	items := make([]notify.Item, 0, count)
	for i := range count {
		u := urls[i%len(urls)]
		if i == 0 {
			u = url
		}
		m := ews.Meeting{
			ID:       fmt.Sprintf("test-%d", i+1),
			Subject:  fmt.Sprintf("Тестовая встреча %d", i+1),
			Start:    now.Add(5 * time.Minute),
			Location: u,
			JoinURL:  u,
		}
		title, body := formatNotification(m, 5, false)
		items = append(items, notify.Item{
			Title: title, Body: body, URL: u, MeetingID: m.ID, Start: m.Start,
		})
		fmt.Fprintf(os.Stderr, "[%d/%d] %s\n  url: %s\n", i+1, count, title, u)
	}
	fmt.Fprintf(os.Stderr, "Очередь из %d: «%s» / «%s» / «%s» / «%s» (таймаут %s на карточку)\n",
		count, cfg.OpenActionLabel, cfg.SkipActionLabel, cfg.StopActionLabel, cfg.SkipAllActionLabel, wait)
	return n.PresentQueue(items, wait)
}

// dueReminder is a meeting+offset pair that should notify now.
type dueReminder struct {
	Meeting ews.Meeting
	Offset  int // offset minutes; -1 for a snooze follow-up
	Key     string
	Snooze  bool
}

// reminderGate abstracts state checks used when collecting due reminders.
type reminderGate interface {
	Seen(key string) bool
	IsDismissed(meetingID string) bool
	SnoozeUntil(meetingID string) (time.Time, bool)
}

// collectDueReminders returns reminders that should fire for the given meetings/now.
// known reports meetings seen in a previous EWS poll; for those, offset 0 uses the
// start of the local minute (seconds=0) as the fire threshold.
func collectDueReminders(
	meetings []ews.Meeting,
	offsets []int,
	now time.Time,
	graceAfter int,
	gate reminderGate,
	known func(id string) bool,
) []dueReminder {
	var out []dueReminder
	for _, m := range meetings {
		if gate != nil && gate.IsDismissed(m.ID) {
			continue
		}
		if gate != nil {
			if until, ok := gate.SnoozeUntil(m.ID); ok {
				if now.Before(until) {
					// Waiting for snooze — suppress offset reminders too.
					continue
				}
				// Snooze expired: deliver one follow-up (ClearSnooze runs on show).
				key := state.SnoozeCycleKey(m.ID, until)
				if !gate.Seen(key) {
					out = append(out, dueReminder{
						Meeting: m,
						Offset:  -1,
						Key:     key,
						Snooze:  true,
					})
					continue
				}
				// Already delivered but schedule key lingered — allow offsets.
			}
		}
		for _, offset := range offsets {
			key := fmt.Sprintf("%s:%d", m.ID, offset)
			if gate != nil && gate.Seen(key) {
				continue
			}
			fireStart := m.Start
			if offset == 0 && known != nil && known(m.ID) {
				fireStart = truncateToLocalMinute(m.Start)
			}
			if !shouldFire(fireStart, offset, now, graceAfter) {
				continue
			}
			out = append(out, dueReminder{Meeting: m, Offset: offset, Key: key})
		}
	}
	return out
}

// processOnce fetches meetings, enqueues due reminders, and refreshes known-meeting
// memory from this poll (so the next poll can align offset 0 to the minute).
func processOnce(client *ews.Client, cfg *config.Settings, store *state.Store, n *notify.Notifier, known *meetingMemory) (int, error) {
	now := time.Now()
	end := now.Add(time.Duration(cfg.LookaheadHours) * time.Hour)
	meetings, err := client.CalendarView(now, end)
	if err != nil {
		return 0, err
	}
	var knownFn func(string) bool
	if known != nil {
		knownFn = known.Known
	}
	due := collectDueReminders(meetings, cfg.OffsetsMinutes, now, cfg.GraceAfterSeconds, store, knownFn)
	if len(due) > 0 {
		items := make([]notify.Item, 0, len(due))
		for _, d := range due {
			title, body := formatNotification(d.Meeting, d.Offset, d.Snooze)
			items = append(items, notify.Item{
				Title:     title,
				Body:      body,
				URL:       d.Meeting.JoinURL,
				Key:       d.Key,
				MeetingID: d.Meeting.ID,
				Start:     d.Meeting.Start,
			})
			if d.Snooze {
				log.Printf("queued snooze %q url=%q", d.Meeting.Subject, d.Meeting.JoinURL)
			} else {
				log.Printf("queued %q offset=%d url=%q", d.Meeting.Subject, d.Offset, d.Meeting.JoinURL)
			}
		}
		n.Enqueue(items)
	}
	if known != nil {
		known.Refresh(meetings)
	}
	return len(due), nil
}

// nextOffset0Wait returns time until the soonest known meeting's offset-0 minute
// boundary, or 0 if none is sooner than maxWait.
func nextOffset0Wait(now time.Time, maxWait time.Duration, known *meetingMemory, hasOffset0 bool, store *state.Store) time.Duration {
	if !hasOffset0 || known == nil {
		return 0
	}
	fireAt, ok := known.NextOffset0FireAt(now, store)
	if !ok {
		return 0
	}
	d := fireAt.Sub(now)
	if d > 0 && d < maxWait {
		return d
	}
	return 0
}

func shouldFire(start time.Time, offsetMin int, now time.Time, graceAfter int) bool {
	remaining := start.Sub(now).Seconds()
	threshold := float64(offsetMin * 60)
	return remaining > (threshold-float64(graceAfter)) && remaining <= threshold
}

// truncateToLocalMinute returns t with seconds and nanoseconds cleared in t's location.
func truncateToLocalMinute(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, t.Location())
}

// meetingMemory tracks starts from the previous EWS poll so offset 0 can fire on
// the minute for meetings we already know about.
type meetingMemory struct {
	starts map[string]time.Time
}

func (m *meetingMemory) Known(id string) bool {
	if m == nil || m.starts == nil {
		return false
	}
	_, ok := m.starts[id]
	return ok
}

func (m *meetingMemory) Refresh(meetings []ews.Meeting) {
	next := make(map[string]time.Time, len(meetings))
	for _, mtg := range meetings {
		next[mtg.ID] = mtg.Start
	}
	m.starts = next
}

// NextOffset0FireAt returns the soonest future minute-aligned start among known
// meetings that still need an offset-0 reminder.
func (m *meetingMemory) NextOffset0FireAt(now time.Time, store *state.Store) (time.Time, bool) {
	if m == nil || len(m.starts) == 0 {
		return time.Time{}, false
	}
	var best time.Time
	found := false
	for id, start := range m.starts {
		if store != nil {
			if store.IsDismissed(id) || store.Seen(fmt.Sprintf("%s:0", id)) {
				continue
			}
			if until, ok := store.SnoozeUntil(id); ok && now.Before(until) {
				continue
			}
		}
		fireAt := truncateToLocalMinute(start)
		if !fireAt.After(now) {
			continue
		}
		if !found || fireAt.Before(best) {
			best = fireAt
			found = true
		}
	}
	return best, found
}

// formatNotification builds one reminder card: title carries the offset label,
// body is only start clock + place/URL (no second "через N минут" — GNOME shows
// summary and body as two peer lines and that looked like two meetings).
func formatNotification(m ews.Meeting, offsetMin int, snooze bool) (title, body string) {
	switch {
	case snooze:
		title = "Напоминание: " + m.Subject
	case offsetMin == 0:
		title = "Начало: " + m.Subject
	default:
		title = fmt.Sprintf("Через %d мин: %s", offsetMin, m.Subject)
	}
	body = m.Start.Format("15:04")
	extra := strings.TrimSpace(m.Location)
	if extra == "" {
		extra = m.JoinURL
	}
	if extra != "" {
		body += "\n" + extra
	}
	return title, body
}

func notifyOptions(cfg *config.Settings, store *state.Store) notify.Options {
	opts := notify.Options{
		AppName:            cfg.AppName,
		Urgency:            cfg.Urgency,
		OpenActionLabel:    cfg.OpenActionLabel,
		SkipActionLabel:    cfg.SkipActionLabel,
		SkipAllActionLabel: cfg.SkipAllActionLabel,
		StopActionLabel:    cfg.StopActionLabel,
		OpenURLCmd:         cfg.OpenURLCmd,
		WaitPerItem:        config.DefaultWaitPerItem,
	}
	if store != nil {
		snoozeFor := time.Duration(cfg.SnoozeMinutes) * time.Minute
		opts.OnShown = func(key string) {
			if err := store.Mark(key); err != nil {
				log.Printf("state: %v", err)
			}
			// After a snooze follow-up is delivered, clear the schedule so
			// offset reminders can resume on later polls.
			if mid, ok := meetingIDFromSnoozeCycleKey(key); ok {
				if err := store.ClearSnooze(mid); err != nil {
					log.Printf("state clear snooze: %v", err)
				}
			}
		}
		opts.OnDropped = func(key string) {
			if err := store.Mark(key); err != nil {
				log.Printf("state: %v", err)
			}
		}
		opts.OnSnooze = func(meetingID string, start time.Time) {
			now := time.Now()
			until := computeSnoozeUntil(now, start, snoozeFor, cfg.OffsetsMinutes, func(offset int) bool {
				return store.Seen(fmt.Sprintf("%s:%d", meetingID, offset))
			})
			if err := store.SetSnooze(meetingID, until); err != nil {
				log.Printf("state snooze: %v", err)
				return
			}
			// Consume schedule offsets that would fire at or before the snooze
			// re-fire time, so the follow-up does not double with those offsets.
			if !start.IsZero() {
				if err := markOffsetsCoveredBySnooze(store, meetingID, start, cfg.OffsetsMinutes, until); err != nil {
					log.Printf("state mark covered offsets: %v", err)
				}
			}
			log.Printf("snoozed meeting=%s until %s", meetingID, until.Format(time.RFC3339))
		}
		opts.OnStop = func(meetingID string) {
			if err := store.MarkDismissed(meetingID); err != nil {
				log.Printf("state dismiss: %v", err)
				return
			}
			log.Printf("stopped reminders for meeting=%s", meetingID)
		}
	}
	return opts
}

// computeSnoozeUntil returns when a snoozed reminder should re-fire.
//
// Priority:
//  1. If there is a still-unseen offsets_minutes fire time strictly after now
//     and at or before meeting start — snooze until the soonest such time.
//  2. Else if the meeting has not started and less than snoozeFor remains —
//     clamp to meeting start.
//  3. Else now + snoozeFor (including after the meeting has started).
func computeSnoozeUntil(now, start time.Time, snoozeFor time.Duration, offsets []int, offsetSeen func(offset int) bool) time.Time {
	if !start.IsZero() {
		if next, ok := nextOffsetFireAt(now, start, offsets, offsetSeen); ok {
			return next
		}
	}
	until := now.Add(snoozeFor)
	if start.IsZero() {
		return until
	}
	if start.After(now) && start.Before(until) {
		return start
	}
	return until
}

// nextOffsetFireAt finds the earliest unseen offset fire moment in (now, start].
func nextOffsetFireAt(now, start time.Time, offsets []int, offsetSeen func(offset int) bool) (time.Time, bool) {
	var best time.Time
	found := false
	for _, offset := range offsets {
		if offsetSeen != nil && offsetSeen(offset) {
			continue
		}
		fireAt := start.Add(-time.Duration(offset) * time.Minute)
		if !fireAt.After(now) || fireAt.After(start) {
			continue
		}
		if !found || fireAt.Before(best) {
			best = fireAt
			found = true
		}
	}
	return best, found
}

// markOffsetsCoveredBySnooze marks offset keys whose fire moment
// (start - offset) is at or before until, so they will not duplicate the
// snooze follow-up or fire during the snooze wait for it ends.
func markOffsetsCoveredBySnooze(store *state.Store, meetingID string, start time.Time, offsets []int, until time.Time) error {
	for _, offset := range offsets {
		fireAt := start.Add(-time.Duration(offset) * time.Minute)
		if fireAt.After(until) {
			continue
		}
		if err := store.Mark(fmt.Sprintf("%s:%d", meetingID, offset)); err != nil {
			return err
		}
	}
	return nil
}

// meetingIDFromSnoozeCycleKey parses "<id>:snooze:<unix>".
func meetingIDFromSnoozeCycleKey(key string) (string, bool) {
	const mark = ":snooze:"
	i := strings.LastIndex(key, mark)
	if i <= 0 {
		return "", false
	}
	// Ensure the suffix after mark is all digits (unix).
	rest := key[i+len(mark):]
	if rest == "" {
		return "", false
	}
	for _, c := range rest {
		if c < '0' || c > '9' {
			return "", false
		}
	}
	return key[:i], true
}
