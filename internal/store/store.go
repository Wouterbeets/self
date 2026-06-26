package store

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"self/internal/event"
)

type Store struct {
	path string
}

func Open(dir string) *Store {
	return &Store{path: filepath.Join(dir, "events.jsonl")}
}

func (s *Store) Append(e *event.Event) error {
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	seq, err := s.nextSeq()
	if err != nil {
		return err
	}
	e.Seq = seq

	if e.ID == "" {
		e.ID = event.NewID()
	}

	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(f, string(line))
	return err
}

func (s *Store) Read() ([]event.Event, error) {
	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var events []event.Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		var e event.Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("parse line: %w", err)
		}
		events = append(events, e)
	}
	return events, sc.Err()
}

func (s *Store) nextSeq() (int, error) {
	events, err := s.Read()
	if err != nil {
		return 0, err
	}
	if len(events) == 0 {
		return 1, nil
	}
	return events[len(events)-1].Seq + 1, nil
}
