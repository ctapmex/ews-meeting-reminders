package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Store persists reminder progress in a flat string map (shown.json).
//
// Keys (backward compatible):
//   - "<meetingID>:<offset>" — offset reminder was shown/dropped (legacy + current)
//   - "<meetingID>:dismissed" — user stopped reminders for this meeting
//   - "<meetingID>:snooze" — RFC3339 time when a snoozed reminder should fire again
//   - "<meetingID>:snooze:<unix>" — a delivered snooze cycle (dedupe)
type Store struct {
	path string
	mu   sync.Mutex
	data map[string]string
}

const (
	suffixDismissed = "dismissed"
	suffixSnooze    = "snooze"
)

func Open(path string) (*Store, error) {
	s := &Store{path: path, data: map[string]string{}}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &s.data); err != nil {
			s.data = map[string]string{}
		}
	}
	return s, nil
}

func (s *Store) Seen(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.data[key]
	return ok
}

func (s *Store) Mark(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = time.Now().Format(time.RFC3339)
	return s.saveLocked()
}

func dismissedKey(meetingID string) string {
	return meetingID + ":" + suffixDismissed
}

func snoozeKey(meetingID string) string {
	return meetingID + ":" + suffixSnooze
}

func snoozeCycleKey(meetingID string, until time.Time) string {
	return fmt.Sprintf("%s:%s:%d", meetingID, suffixSnooze, until.Unix())
}

func (s *Store) IsDismissed(meetingID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.data[dismissedKey(meetingID)]
	return ok
}

// MarkDismissed stops all further reminders for the meeting and clears a pending snooze.
func (s *Store) MarkDismissed(meetingID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[dismissedKey(meetingID)] = time.Now().Format(time.RFC3339)
	delete(s.data, snoozeKey(meetingID))
	return s.saveLocked()
}

// SnoozeUntil returns the scheduled re-fire time, if any.
func (s *Store) SnoozeUntil(meetingID string) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, ok := s.data[snoozeKey(meetingID)]
	if !ok {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// EarliestSnoozeUntil returns the soonest pending snooze fire time strictly after after.
func (s *Store) EarliestSnoozeUntil(after time.Time) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var best time.Time
	found := false
	for k, raw := range s.data {
		if !isSnoozeScheduleKey(k) {
			continue
		}
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil || !t.After(after) {
			continue
		}
		if !found || t.Before(best) {
			best = t
			found = true
		}
	}
	return best, found
}

// SetSnooze schedules a follow-up reminder at until.
func (s *Store) SetSnooze(meetingID string, until time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[snoozeKey(meetingID)] = until.Format(time.RFC3339)
	return s.saveLocked()
}

// ClearSnooze removes a pending snooze schedule (e.g. after it was delivered).
func (s *Store) ClearSnooze(meetingID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, snoozeKey(meetingID))
	return s.saveLocked()
}

// SnoozeCycleKey is the dedupe key for one snooze delivery.
func SnoozeCycleKey(meetingID string, until time.Time) string {
	return snoozeCycleKey(meetingID, until)
}

func (s *Store) Prune(keep time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Now().Add(-keep)
	kept := make(map[string]string, len(s.data))
	for k, ts := range s.data {
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			continue
		}
		// Keep future snooze schedules even if "written" earlier than cutoff
		// would suggest — the value is the fire-at time, not shown-at.
		if isSnoozeScheduleKey(k) && t.After(time.Now()) {
			kept[k] = ts
			continue
		}
		if !t.Before(cutoff) {
			kept[k] = ts
		}
	}
	s.data = kept
	return s.saveLocked()
}

func isSnoozeScheduleKey(k string) bool {
	// "...:snooze" but not "...:snooze:<unix>"
	if len(k) < len(suffixSnooze)+1 {
		return false
	}
	// find last colon segment
	i := len(k) - 1
	for i >= 0 && k[i] != ':' {
		i--
	}
	if i < 0 {
		return false
	}
	return k[i+1:] == suffixSnooze
}

func (s *Store) saveLocked() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(s.path, b, 0o600)
}
