package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

func cmdMake(home string, args []string, script io.Reader, out io.Writer) error {
	if len(args) == 0 {
		if _, err := io.WriteString(out, protocolLayer("make")+"\n"); err != nil {
			return fmt.Errorf("print make examples: %w", err)
		}
		return nil
	}
	const usage = "usage: self make command|view <name> <summary…> [--consumes a.b,c.d] [--brief] < script — bare `self make` shows worked examples"
	if len(args) < 2 || (args[0] != kindCommand && args[0] != kindView) {
		return errors.New(usage)
	}
	typ, name := args[0], args[1]
	if !validCapability(typ, name) {
		return fmt.Errorf("%q cannot name a %s: use plain path segments, like notes or deploy/staging", name, typ)
	}
	d := decl{Name: name}
	var words []string
	for rest := args[2:]; len(rest) > 0; rest = rest[1:] {
		switch {
		case rest[0] == "--brief" && typ == kindView:
			d.Brief = &briefSpec{}
		case rest[0] == "--brief":
			return fmt.Errorf("--brief is for views: a view is what `self` can print on arrival")
		case rest[0] == "--consumes" && len(rest) > 1:
			for _, c := range strings.Split(rest[1], ",") {
				if c = strings.TrimSpace(c); c != "" {
					d.Consumes = append(d.Consumes, c)
				}
			}
			rest = rest[1:]
		case strings.HasPrefix(rest[0], "--consumes="):
			d.Consumes = append(d.Consumes, strings.Split(strings.TrimPrefix(rest[0], "--consumes="), ",")...)
		default:
			words = append(words, rest[0])
		}
	}
	d.Summary = strings.Join(words, " ")
	d.Description = d.Summary
	if strings.TrimSpace(d.Summary) == "" {
		return fmt.Errorf("say in a line what %s is for, after its name: that line is all a later session sees of it\n%s", name, usage)
	}

	if f, ok := script.(*os.File); ok {
		if info, err := f.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
			return fmt.Errorf("the script arrives on stdin, and stdin is a terminal\n%s", usage)
		}
	}
	body, err := io.ReadAll(script)
	if err != nil {
		return fmt.Errorf("read the script from stdin: %w", err)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return fmt.Errorf("no script arrived on stdin\n%s", usage)
	}
	if !bytes.HasPrefix(body, []byte("#!")) {
		body = append([]byte("#!/bin/sh\n"), body...)
	}

	payload, err := json.Marshal(d)
	if err != nil {
		return fmt.Errorf("encode the %s declaration: %w", typ, err)
	}
	by := callerClaim()
	e := newEvent(typ+".declared", payload)
	e.Via, e.By = doorCLI, by
	if err := ingest(home, []Event{e}, []authored{{Type: typ, Name: name, Script: string(body)}}, nil, by, io.Discard); err != nil {
		return fmt.Errorf("make %s/%s: %w", typ, name, err)
	}
	verb := map[string]string{kindCommand: "run", kindView: "view"}[typ]
	if _, err := fmt.Fprintf(out, "made %s/%s — try it now: `self %s %s`. Every later session finds it in the brief.\n", typ, name, verb, name); err != nil {
		return fmt.Errorf("report what was made: %w", err)
	}
	return nil
}
