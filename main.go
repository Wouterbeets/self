// self — a local-first, event-sourced runtime with LLM-generated capabilities.
//
// One append-only event log (events.jsonl) is the only truth. Every view is a
// pure replay of it, rendered as HTML that you and your agent read identically.
// Capabilities are standalone scripts the kernel pipes events through, and code
// is never shipped — a mind process (SELF_MIND) authors every script from a
// declaration, for this receiver; the kernel holds no model of its own. A
// running capability can declare new capabilities and the
// kernel compiles them on the spot (the strange loop). Every compile is logged
// as a script.compiled receipt signed with a per-home secret; only kernel-signed
// receipts ever install, so `self rehydrate` rebuilds the derived instance from
// events.jsonl + .secret alone.
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
	case "think":
		err = cmdThink(home, strings.Join(args, " "))
	case "reflect":
		err = cmdReflect(home)
	case "show":
		if len(args) < 1 {
			err = fmt.Errorf("usage: self show <projection>")
		} else {
			err = cmdShow(home, args[0])
		}
	case "rehydrate":
		err = rehydrate(home)
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
