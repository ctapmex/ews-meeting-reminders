package app

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"sort"
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

	log.Printf("connected as %s; offsets=%v poll=%ds", cfg.Email, cfg.OffsetsMinutes, cfg.PollSeconds)

	if opts.Once {
		fired, err := processOnce(client, cfg, store, n)
		if err != nil {
			return err
		}
		log.Printf("pass done, notifications=%d", fired)
		return nil
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	for {
		if err := store.Prune(time.Duration(cfg.StateKeepHours) * time.Hour); err != nil {
			log.Printf("state prune: %v", err)
		}
		if _, err := processOnce(client, cfg, store, n); err != nil {
			log.Printf("ews fetch: %v", err)
		}
		timer := time.NewTimer(time.Duration(cfg.PollSeconds) * time.Second)
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

// RunTestNotify shows reminders one-by-one (Open / Skip) — GNOME/Cinnamon only show one banner.
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
			Subject:  fmt.Sprintf("Тестовая встреча %d", i+1),
			Start:    now.Add(5 * time.Minute),
			Location: u,
			JoinURL:  u,
		}
		title, body := formatNotification(m, 5, now)
		items = append(items, notify.Item{Title: title, Body: body, URL: u})
		fmt.Fprintf(os.Stderr, "[%d/%d] %s\n  url: %s\n", i+1, count, title, u)
	}
	fmt.Fprintf(os.Stderr, "Очередь из %d: «%s» / «%s» / «%s» (таймаут %s на карточку)\n",
		count, cfg.OpenActionLabel, cfg.SkipActionLabel, cfg.SkipAllActionLabel, wait)
	return n.PresentQueue(items, wait)
}

// dueReminder is a meeting+offset pair that should notify now.
type dueReminder struct {
	Meeting ews.Meeting
	Offset  int
	Key     string
}

// collectDueReminders returns reminders that should fire for the given meetings/now.
// seen(key) reports whether that reminder was already shown.
func collectDueReminders(
	meetings []ews.Meeting,
	offsets []int,
	now time.Time,
	graceAfter int,
	seen func(string) bool,
) []dueReminder {
	var out []dueReminder
	for _, m := range meetings {
		for _, offset := range offsets {
			key := fmt.Sprintf("%s:%d", m.ID, offset)
			if seen != nil && seen(key) {
				continue
			}
			if !shouldFire(m.Start, offset, now, graceAfter) {
				continue
			}
			out = append(out, dueReminder{Meeting: m, Offset: offset, Key: key})
		}
	}
	return out
}

func processOnce(client *ews.Client, cfg *config.Settings, store *state.Store, n *notify.Notifier) (int, error) {
	now := time.Now()
	end := now.Add(time.Duration(cfg.LookaheadHours) * time.Hour)
	meetings, err := client.CalendarView(now, end)
	if err != nil {
		return 0, err
	}
	due := collectDueReminders(meetings, cfg.OffsetsMinutes, now, cfg.GraceAfterSeconds, store.Seen)
	if len(due) == 0 {
		return 0, nil
	}

	items := make([]notify.Item, 0, len(due))
	for _, d := range due {
		title, body := formatNotification(d.Meeting, d.Offset, now)
		items = append(items, notify.Item{Title: title, Body: body, URL: d.Meeting.JoinURL, Key: d.Key})
		log.Printf("queued %q offset=%d url=%q", d.Meeting.Subject, d.Offset, d.Meeting.JoinURL)
	}

	// Single global queue: if a banner is already up, new meetings wait in line.
	// Items are marked as shown by the notifier when it actually presents them.
	n.Enqueue(items)
	return len(due), nil
}

func shouldFire(start time.Time, offsetMin int, now time.Time, graceAfter int) bool {
	remaining := start.Sub(now).Seconds()
	threshold := float64(offsetMin * 60)
	return remaining > (threshold-float64(graceAfter)) && remaining <= threshold
}

func formatNotification(m ews.Meeting, offsetMin int, now time.Time) (title, body string) {
	if offsetMin == 0 {
		title = "Начало: " + m.Subject
	} else {
		title = fmt.Sprintf("Через %d мин: %s", offsetMin, m.Subject)
	}
	body = formatWhen(m.Start, now)
	if m.Location != "" {
		body += "\n" + m.Location
	} else if m.JoinURL != "" {
		body += "\n" + m.JoinURL
	}
	return title, body
}

func formatWhen(start, now time.Time) string {
	delta := int(start.Sub(now).Minutes())
	clock := start.Format("15:04")
	switch {
	case delta <= 0:
		return fmt.Sprintf("сейчас (%s)", clock)
	case delta == 1:
		return fmt.Sprintf("через 1 минуту (%s)", clock)
	case delta < 5:
		return fmt.Sprintf("через %d минуты (%s)", delta, clock)
	default:
		return fmt.Sprintf("через %d минут (%s)", delta, clock)
	}
}

func notifyOptions(cfg *config.Settings, store *state.Store) notify.Options {
	opts := notify.Options{
		AppName:            cfg.AppName,
		Urgency:            cfg.Urgency,
		OpenActionLabel:    cfg.OpenActionLabel,
		SkipActionLabel:    cfg.SkipActionLabel,
		SkipAllActionLabel: cfg.SkipAllActionLabel,
		OpenURLCmd:         cfg.OpenURLCmd,
		WaitPerItem:        config.DefaultWaitPerItem,
	}
	if store != nil {
		opts.OnShown = func(key string) {
			if err := store.Mark(key); err != nil {
				log.Printf("state: %v", err)
			}
		}
		opts.OnDropped = func(key string) {
			if err := store.Mark(key); err != nil {
				log.Printf("state: %v", err)
			}
		}
	}
	return opts
}
