package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

func head(events []Event) string {
	if len(events) == 0 {
		return "empty"
	}
	return events[len(events)-1].ID
}

// Watch returns the first matching batch. Its cursor is an event ID, not a
// sequence number that can be silently reused after a replaced or damaged log.
func cmdWatch(home string, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("watch", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	after := flags.String("after", "", "event ID; empty for all, omitted for current head")
	timeout := flags.Duration("timeout", 10*time.Minute, "maximum wait")
	if err := flags.Parse(args); err != nil || flags.NArg() > 1 || *timeout <= 0 {
		return fmt.Errorf("usage: self watch [--after <id|empty>] [--timeout 10m] [event-prefix]")
	}
	deadline := time.Now().Add(*timeout)
	var previous os.FileInfo
	var events []Event
	for {
		info, err := os.Stat(logPath(home))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if info == nil || previous == nil || !os.SameFile(info, previous) || info.Size() != previous.Size() || !info.ModTime().Equal(previous.ModTime()) {
			events, err = readEvents(home)
			if err != nil {
				return err
			}
			previous = info
		}
		if *after == "" {
			*after = head(events)
		}
		start := 0
		if *after != "empty" {
			for start < len(events) && events[start].ID != *after {
				start++
			}
			if start == len(events) {
				return fmt.Errorf("watch cursor %q missing; reread the log", *after)
			}
			start++
		}
		found := false
		for _, e := range events[start:] {
			if strings.HasPrefix(e.Name, flags.Arg(0)) {
				if err := json.NewEncoder(out).Encode(e); err != nil {
					return err
				}
				found = true
			}
		}
		if found {
			return nil
		}
		*after = head(events)
		if remaining := time.Until(deadline); remaining <= 0 {
			return fmt.Errorf("watch timed out without matching events")
		} else {
			time.Sleep(min(remaining, 250*time.Millisecond))
		}
	}
}
