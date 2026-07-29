package notify

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/godbus/dbus/v5"
)

type Options struct {
	AppName            string
	Urgency            string // low|normal|critical
	OpenActionLabel    string
	SkipActionLabel    string // snooze
	SkipAllActionLabel string // snooze all queued
	StopActionLabel    string // dismiss this meeting
	OpenURLCmd         string // command to open join URLs
	WaitPerItem        time.Duration

	// OnShown is called after a notification is actually presented.
	OnShown func(key string)
	// OnDropped is called for items removed from the queue without being presented
	// (e.g. drained after stop/snooze-all for another card).
	OnDropped func(key string)
	// OnSnooze is called when the user snoozes a presented reminder (or snooze-all).
	// start is the meeting start time (may be zero for ad-hoc test notifications).
	OnSnooze func(meetingID string, start time.Time)
	// OnStop is called when the user stops reminders for a meeting.
	OnStop func(meetingID string)
}

// Item is one meeting reminder card in a sequential queue.
type Item struct {
	Title     string
	Body      string
	URL       string
	Key       string // state key; used by OnShown/OnDropped
	MeetingID string // meeting id for snooze/stop
	Start     time.Time
}

type Notifier struct {
	opts Options
	conn *dbus.Conn

	inbox      chan Item
	stop       chan struct{}
	wg         sync.WaitGroup
	presenting atomic.Bool
	queued     atomic.Int64 // items enqueued but not finished (incl. currently showing)
	waitPer    atomic.Int64 // nanoseconds
}

func New(opts Options) (*Notifier, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("session bus: %w (need DBUS_SESSION_BUS_ADDRESS)", err)
	}
	n := &Notifier{
		opts:  opts,
		conn:  conn,
		inbox: make(chan Item, 128),
		stop:  make(chan struct{}),
	}
	n.waitPer.Store(int64(opts.WaitPerItem))

	if err := n.conn.AddMatchSignal(
		dbus.WithMatchInterface("org.freedesktop.Notifications"),
		dbus.WithMatchMember("ActionInvoked"),
	); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := n.conn.AddMatchSignal(
		dbus.WithMatchInterface("org.freedesktop.Notifications"),
		dbus.WithMatchMember("NotificationClosed"),
	); err != nil {
		_ = conn.Close()
		return nil, err
	}

	n.wg.Add(1)
	go n.presenter()
	return n, nil
}

func (n *Notifier) Close() {
	select {
	case <-n.stop:
	default:
		close(n.stop)
	}
	n.wg.Wait()
	if n.conn != nil {
		_ = n.conn.Close()
	}
}

func (n *Notifier) SetWaitPerItem(d time.Duration) {
	if d > 0 {
		n.waitPer.Store(int64(d))
	}
}

// Enqueue appends reminders to the single global presenter queue.
// Safe while another reminder is on screen: new items wait in line.
func (n *Notifier) Enqueue(items []Item) {
	for _, it := range items {
		// Count before send so WaitIdle cannot observe an empty inbox between
		// receive and presenting.Store(true) and treat the queue as idle.
		n.queued.Add(1)
		select {
		case <-n.stop:
			n.queued.Add(-1)
			return
		case n.inbox <- it:
			log.Printf("notify: enqueued %q (inbox=%d queued=%d)", it.Title, len(n.inbox), n.queued.Load())
		}
	}
}

// EnqueueAndWait enqueues items and blocks until the queue is idle (test helper).
func (n *Notifier) EnqueueAndWait(items []Item, waitPerItem time.Duration) error {
	n.SetWaitPerItem(waitPerItem)
	n.Enqueue(items)
	return n.WaitIdle(time.Duration(len(items)+1) * (waitPerItem + time.Second))
}

// WaitIdle waits until nothing is showing and inbox is empty.
func (n *Notifier) WaitIdle(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if n.queued.Load() == 0 {
			return nil
		}
		select {
		case <-n.stop:
			return nil
		case <-time.After(100 * time.Millisecond):
		}
	}
	return fmt.Errorf("queue still busy after %s (queued=%d presenting=%v inbox=%d)",
		timeout, n.queued.Load(), n.presenting.Load(), len(n.inbox))
}

func (n *Notifier) presenter() {
	defer n.wg.Done()
	sigCh := make(chan *dbus.Signal, 32)
	n.conn.Signal(sigCh)

	var replaces uint32
	for {
		select {
		case <-n.stop:
			return
		case item := <-n.inbox:
			n.presenting.Store(true)
			pendingAfter := len(n.inbox)
			title := item.Title
			if pendingAfter > 0 {
				title = fmt.Sprintf("%s · ещё %d", title, pendingAfter)
			}
			id, err := n.show(title, item.Body, item.URL, replaces, true, pendingAfter > 0)
			if err != nil {
				log.Printf("notify: show: %v", err)
				n.presenting.Store(false)
				n.queued.Add(-1)
				continue
			}
			if item.Key != "" && n.opts.OnShown != nil {
				n.opts.OnShown(item.Key)
			}
			replaces = id
			wait := time.Duration(n.waitPer.Load())
			log.Printf("notify: showing id=%d url=%q inbox=%d wait=%s", id, item.URL, pendingAfter, wait)
			action, _ := n.waitOn(sigCh, id, wait)
			switch action {
			case "open":
				if item.URL != "" {
					if err := OpenURL(item.URL, n.opts.OpenURLCmd); err != nil {
						log.Printf("notify: open url %q: %v", item.URL, err)
					} else {
						log.Printf("notify: opened %s", item.URL)
					}
				} else {
					log.Printf("notify: open action but no url for id=%d", id)
				}
			case "skip": // snooze
				if item.MeetingID != "" && n.opts.OnSnooze != nil {
					n.opts.OnSnooze(item.MeetingID, item.Start)
				}
				// Drop other cards for the same meeting already waiting in the
				// inbox (e.g. a later offset enqueued while this banner sat open).
				dropped := n.drainMeeting(item.MeetingID)
				log.Printf("notify: snooze meeting=%q — dropped %d queued for same meeting", item.MeetingID, dropped)
			case "stop":
				if item.MeetingID != "" && n.opts.OnStop != nil {
					n.opts.OnStop(item.MeetingID)
				}
				dropped := n.drainMeeting(item.MeetingID)
				log.Printf("notify: stop meeting=%q — dropped %d queued for same meeting", item.MeetingID, dropped)
			case "skip_all": // snooze all (current + queued)
				if item.MeetingID != "" && n.opts.OnSnooze != nil {
					n.opts.OnSnooze(item.MeetingID, item.Start)
				}
				dropped := n.drainInboxSnooze()
				log.Printf("notify: snooze_all — snoozed/dropped %d queued", dropped)
			default:
				log.Printf("notify: done action=%q → continue", action)
			}
			n.presenting.Store(false)
			n.queued.Add(-1)
		}
	}
}

// PresentQueue is a compatibility wrapper: enqueue and wait until drained.
func (n *Notifier) PresentQueue(items []Item, waitPerItem time.Duration) error {
	return n.EnqueueAndWait(items, waitPerItem)
}

// Show fire-and-forget via the same queue (keeps ordering with other reminders).
func (n *Notifier) Show(title, body, joinURL string) error {
	n.Enqueue([]Item{{Title: title, Body: body, URL: joinURL}})
	return nil
}

// drainInboxSnooze removes all queued items, snoozing each meeting and marking keys shown.
func (n *Notifier) drainInboxSnooze() int {
	dropped := 0
	seenMeetings := map[string]struct{}{}
	for {
		select {
		case item := <-n.inbox:
			dropped++
			n.queued.Add(-1)
			if item.MeetingID != "" {
				if _, ok := seenMeetings[item.MeetingID]; !ok {
					seenMeetings[item.MeetingID] = struct{}{}
					if n.opts.OnSnooze != nil {
						n.opts.OnSnooze(item.MeetingID, item.Start)
					}
				}
			}
			if item.Key != "" && n.opts.OnDropped != nil {
				n.opts.OnDropped(item.Key)
			}
		default:
			return dropped
		}
	}
}

// drainMeeting drops queued items for the same meeting (already stopped via OnStop).
func (n *Notifier) drainMeeting(meetingID string) int {
	if meetingID == "" {
		return 0
	}
	var keep []Item
	dropped := 0
	for {
		select {
		case item := <-n.inbox:
			if item.MeetingID == meetingID {
				dropped++
				n.queued.Add(-1)
				if item.Key != "" && n.opts.OnDropped != nil {
					n.opts.OnDropped(item.Key)
				}
			} else {
				keep = append(keep, item)
			}
		default:
			for _, it := range keep {
				select {
				case n.inbox <- it:
				default:
					// inbox was full of other items; shouldn't happen with buffer 128
					log.Printf("notify: requeue failed for %q", it.Title)
					n.queued.Add(-1)
				}
			}
			return dropped
		}
	}
}

func (n *Notifier) ShowID(title, body, joinURL string) (uint32, error) {
	return n.show(title, body, joinURL, 0, false, false)
}

func (n *Notifier) ShowAndWait(title, body, joinURL string, wait time.Duration) (string, error) {
	if err := n.EnqueueAndWait([]Item{{Title: title, Body: body, URL: joinURL}}, wait); err != nil {
		return "", err
	}
	return "done", nil
}

func (n *Notifier) waitOn(ch <-chan *dbus.Signal, id uint32, wait time.Duration) (string, error) {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		select {
		case <-n.stop:
			return "stopped", nil
		case <-timer.C:
			return "", nil
		case sig := <-ch:
			switch sig.Name {
			case "org.freedesktop.Notifications.ActionInvoked":
				if len(sig.Body) < 2 {
					continue
				}
				sid, _ := sig.Body[0].(uint32)
				act, _ := sig.Body[1].(string)
				if sid != id {
					continue
				}
				return act, nil
			case "org.freedesktop.Notifications.NotificationClosed":
				if len(sig.Body) < 1 {
					continue
				}
				sid, _ := sig.Body[0].(uint32)
				if sid != id {
					continue
				}
				return "closed", nil
			}
		}
	}
}

func (n *Notifier) show(title, body, joinURL string, replaces uint32, withActions, withSkipAll bool) (uint32, error) {
	obj := n.conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications")

	var actions []string
	if joinURL != "" {
		actions = append(actions, "open", n.opts.OpenActionLabel)
	}
	if withActions {
		actions = append(actions, "skip", n.opts.SkipActionLabel)
		if n.opts.StopActionLabel != "" {
			actions = append(actions, "stop", n.opts.StopActionLabel)
		}
	}
	if withSkipAll {
		actions = append(actions, "skip_all", n.opts.SkipAllActionLabel)
	}
	hints := map[string]dbus.Variant{
		"urgency": dbus.MakeVariant(urgencyByte(n.opts.Urgency)),
	}

	var id uint32
	call := obj.Call(
		"org.freedesktop.Notifications.Notify",
		0,
		n.opts.AppName,
		replaces,
		"x-office-calendar",
		title,
		body,
		actions,
		hints,
		int32(0),
	)
	if call.Err != nil {
		return 0, call.Err
	}
	if err := call.Store(&id); err != nil {
		return 0, err
	}
	return id, nil
}

func urgencyByte(u string) byte {
	switch u {
	case "low":
		return 0
	case "critical":
		return 2
	default:
		return 1
	}
}

func OpenURL(url, openCmd string) error {
	parts, err := splitOpenCmd(openCmd)
	if err != nil {
		return err
	}
	cmd := exec.Command(parts[0], append(parts[1:], url)...)
	// Detach from the service process group so the browser keeps running
	// after the child exits and inherits the desktop session when possible.
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %q: %w", cmd.Path, err)
	}
	go func() {
		if err := cmd.Wait(); err != nil {
			log.Printf("notify: %s exited: %v", cmd.Path, err)
		}
	}()
	return nil
}

// splitOpenCmd splits a command string like "xterm -e xdg-open" into parts,
// respecting double quotes. The URL will be appended as the last argument.
func splitOpenCmd(s string) ([]string, error) {
	var parts []string
	var b strings.Builder
	inQuote := false
	for _, r := range s {
		switch r {
		case '"':
			inQuote = !inQuote
		case ' ', '\t':
			if inQuote {
				b.WriteRune(r)
			} else if b.Len() > 0 {
				parts = append(parts, b.String())
				b.Reset()
			}
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		parts = append(parts, b.String())
	}
	if inQuote {
		return nil, fmt.Errorf("unmatched quote in open command %q", s)
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("open command is empty")
	}
	return parts, nil
}
