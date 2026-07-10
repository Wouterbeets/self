// self — a local-first, event-sourced runtime with LLM-generated capabilities.
//
// One append-only event log (events.jsonl) is the only truth. Every view is a
// pure replay of it, rendered as HTML that you and your agent read identically.
// Capabilities are standalone scripts the kernel pipes events through, and code
// is never shipped — a brain process (SELF_BRAIN) authors every script from a
// declaration, for this receiver; the kernel holds no model of its own. A
// running capability can declare new capabilities and the
// kernel compiles them on the spot (the strange loop). Every compile is logged
// as a script.compiled receipt signed with a per-home secret; only kernel-signed
// receipts ever install, so `self rehydrate` rebuilds the whole instance from
// the log alone — an instance is just events.jsonl + .secret.
//
// This file is the whole kernel.
package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	home := homeDir()
	if len(os.Args) < 2 {
		err := ensureHome(home)
		if err == nil {
			err = rehydrate(home)
		}
		if err == nil {
			err = cmdServe(home)
		}
		if err != nil {
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
	case "grow":
		if len(args) < 1 {
			err = fmt.Errorf("usage: self grow <seed-dir>")
		} else {
			err = cmdGrow(home, args[0])
		}
	case "run":
		if len(args) < 1 {
			err = fmt.Errorf("usage: self run <command> [args...]")
		} else {
			err = cmdRun(home, args[0], args[1:])
		}
	case "think":
		err = cmdThink(home, strings.Join(args, " "))
	case "heartbeat":
		err = cmdHeartbeat(home)
	case "show":
		if len(args) < 1 {
			err = fmt.Errorf("usage: self show <projection>")
		} else {
			err = cmdShow(home, args[0])
		}
	case "rehydrate":
		err = rehydrate(home)
	case "share":
		if len(args) != 1 {
			err = fmt.Errorf("usage: self share <capability>  (the seed prints to stdout)")
		} else {
			err = cmdShare(home, args[0])
		}
	case "adopt":
		if len(args) != 1 {
			err = fmt.Errorf("usage: self adopt <seed.jsonl>")
		} else {
			err = cmdAdopt(home, args[0])
		}
	case "export":
		switch len(args) {
		case 2:
			err = cmdExport(home, args[0], args[1], "")
		case 3:
			err = cmdExport(home, args[0], args[1], args[2])
		default:
			err = fmt.Errorf("usage: self export <event-prefix> <dir> [<new-prefix>]")
		}
	case "revise":
		if len(args) < 2 {
			err = fmt.Errorf("usage: self revise command/<name> <change request>")
		} else {
			err = cmdRevise(home, args[0], args[1:])
		}
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
