package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type Store struct {
	path string
	data map[string]string
}

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
	_, ok := s.data[key]
	return ok
}

func (s *Store) Mark(key string) error {
	s.data[key] = time.Now().Format(time.RFC3339)
	return s.save()
}

func (s *Store) Prune(keep time.Duration) error {
	cutoff := time.Now().Add(-keep)
	kept := make(map[string]string, len(s.data))
	for k, ts := range s.data {
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			continue
		}
		if !t.Before(cutoff) {
			kept[k] = ts
		}
	}
	s.data = kept
	return s.save()
}

func (s *Store) save() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(s.path, b, 0o600)
}
