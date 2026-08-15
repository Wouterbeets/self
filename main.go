// self — a small, local-first runtime. One append-only event log
// (events.jsonl) is the only truth; every view is a pure replay of it. The
// kernel holds no model: intelligence enters through the shell pipe, where
// `self` is a filter and the mind is whatever sits between two invocations of
// it:
//
//	echo "whats going on today?" | self | claude -p | self
//
// The first `self` situates the ask (orientation, conversation, pending work,
// the answer contract); the mind answers in event JSONL; the second `self`
// hears it — events append, authored scripts install under receipts signed
// with a per-home secret. Only kernel-signed receipts ever install, so
// `self rehydrate` rebuilds the derived instance from events.jsonl + .secret
// alone. A capability that declares new capabilities feeds the same loop —
// one shell pass at a time, until quiet (the strange loop).
package main

import (
	"fmt"
	"os"
)

func main() {
	home := homeDir()
	if len(os.Args) < 2 {
		if err := cmdPipe(home); err != nil {
			fmt.Fprintf(os.Stderr, "self: %s\n", err)
			os.Exit(1)
		}
		return
	}

	cmd, args := os.Args[1], os.Args[2:]

	var err error
	if cmd != "help" && wantsHelp(args) {
		if text, ok := commandHelp(cmd); ok {
			fmt.Fprint(os.Stdout, text)
			return
		}
	}

	switch cmd {
	case "serve":
		err = ensureHome(home)
		if err == nil {
			err = rehydrate(home)
		}
		if err == nil {
			err = cmdServe(home)
		}
	case "learn":
		if len(args) < 1 {
			err = fmt.Errorf("usage: self learn <account-dir>")
		} else {
			err = cmdLearn(home, args[0])
		}
	case "give":
		if len(args) != 2 {
			err = fmt.Errorf("usage: self give <event-prefix | command/<name> | projector/<name>> <dir>")
		} else {
			err = cmdGive(home, args[0], args[1])
		}
	case "run":
		if len(args) < 1 {
			err = fmt.Errorf("usage: self run <command> [args...]")
		} else {
			err = cmdRun(home, args[0], args[1:])
		}
	case "show":
		if len(args) < 1 {
			err = fmt.Errorf("usage: self show <projection>")
		} else {
			err = cmdShow(home, args[0])
		}
	case "rehydrate":
		err = rehydrate(home)
	case "retire":
		if len(args) != 1 {
			err = fmt.Errorf("usage: self retire command/<name> | projector/<name>")
		} else {
			err = cmdRetire(home, args[0])
		}
	case "protocol":
		fmt.Fprint(os.Stdout, protocolText())
	case "help", "--help", "-h":
		if len(args) == 0 {
			usage()
		} else if text, ok := commandHelp(args[0]); ok {
			fmt.Fprint(os.Stdout, text)
		} else {
			err = fmt.Errorf("unknown help topic %q", args[0])
		}
	default:
		fmt.Fprintf(os.Stderr, "self: unknown command %q\n", cmd)
		usage()
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "self: %s\n", err)
		os.Exit(1)
	}
}
