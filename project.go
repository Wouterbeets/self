package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// project keeps two opt-in trees derived from the log, like cap/: view/<name>
// holds what `self view <name>` prints; bin/<name> runs `self run <name>`.
// Each exists only if its directory does. Failures are reported, never fatal.
func project(home string, full bool) {
	var st *state
	load := func() (*state, error) {
		if st != nil {
			return st, nil
		}
		var err error
		st, err = loadState(home)
		return st, err
	}
	if err := errors.Join(refresh(home, kindView, 0644, full, load, views), refresh(home, "bin", 0755, full, load, shims)); err != nil {
		fmt.Fprintf(os.Stderr, "self: projecting the log: %s\n", err)
	}
}

// refresh syncs one tree under .head, a lock holding the last projected seq,
// loading the log inside the first lock and skipping a tree already projected
// past it, so the last refresher projects the latest.
func refresh(home, tree string, mode os.FileMode, full bool, load func() (*state, error), entries func(string, *state, int) map[string][]byte) error {
	dir, mark := filepath.Join(home, tree), filepath.Join(home, tree, ".head")
	unlock, err := lock(mark)
	if os.IsNotExist(err) {
		return nil // no directory, no tree
	} else if err != nil {
		return err
	}
	defer unlock()
	st, err := load()
	if err != nil {
		return err
	}
	since := 0
	if seen, err := os.ReadFile(mark); err == nil && !full {
		since, _ = strconv.Atoi(string(seen))
	}
	if n := len(st.Events); n > 0 && since > st.Events[n-1].Seq {
		return nil
	}
	want := entries(home, st, since)
	keep := map[string]bool{dir: true, mark: true}
	for name := range want {
		keep[filepath.Join(dir, name)] = true
		for p := filepath.Dir(filepath.Join(dir, name)); p != dir; p = filepath.Dir(p) {
			if fi, err := os.Lstat(p); err == nil && fi.IsDir() {
				keep[p] = true // a parent directory, never a stale file in the way
			}
		}
	}
	_, err = prune(dir, keep)
	for name, data := range want {
		path := filepath.Join(dir, name)
		if have, rerr := os.ReadFile(path); data != nil && (rerr != nil || !bytes.Equal(have, data)) {
			err = errors.Join(err, writeFileAtomic(path, data, mode, os.Rename)) // a name clash, a beside a/b, fails here
		}
	}
	if n := len(st.Events); n > 0 {
		os.WriteFile(mark, []byte(strconv.Itoa(st.Events[n-1].Seq)), 0644)
	}
	return err
}

// views reruns what events after since could change, or a missing file; nil keeps the file.
func views(home string, st *state, since int) map[string][]byte {
	fresh := slices.DeleteFunc(slices.Clone(st.Events), func(e Event) bool { return e.Seq <= since })
	want := map[string][]byte{}
	if st.cap(kindView, "log") == nil {
		want["log"] = builtinLogView(st, false)
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, runtime.GOMAXPROCS(0))
	for _, c := range st.list(kindView) {
		if c.Receipt == nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(home, kindView, c.Name)); err == nil && c.RcptSeq <= since && len(consumed(fresh, c.Receipt.Consumes)) == 0 {
			want[c.Name] = nil
			continue
		}
		wg.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()
			out, err := runViewDiag(home, st, c.Name, io.Discard)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				fmt.Fprintf(os.Stderr, "self: view/%s: %s (no file)\n", c.Name, err)
				return
			}
			want[c.Name] = out
		})
	}
	wg.Wait()
	return want
}

func shims(home string, st *state, _ int) map[string][]byte {
	want := map[string][]byte{}
	for _, c := range st.list(kindCommand) {
		if c.Receipt != nil {
			want[c.Name] = []byte("#!/bin/sh\nSELF_HOME=" + shellQuote(home) + " exec self run " + shellQuote(c.Name) + " \"$@\"\n")
		}
	}
	return want
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
