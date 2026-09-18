package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type repositoryReservation struct {
	file       *os.File
	Path       string
	Identity   string
	Repository string
}

func repositoryIdentity(repo string) (string, string, error) {
	top, err := repoOutput(repo, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", fmt.Errorf("repository reservation needs a Git worktree: %w", err)
	}
	common, err := repoOutput(repo, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", "", err
	}
	canonicalRepo, err := filepath.EvalSymlinks(strings.TrimSpace(string(top)))
	if err != nil {
		return "", "", err
	}
	canonicalCommon, err := filepath.EvalSymlinks(strings.TrimSpace(string(common)))
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte("self.repository-reservation.v1\x00" + canonicalCommon))
	return canonicalRepo, hex.EncodeToString(sum[:]), nil
}

func reservationRootCandidate() (string, error) {
	root := os.Getenv("SELF_RESERVATION_DIR")
	if root == "" {
		if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
			root = filepath.Join(runtimeDir, "self", "reservations")
		} else {
			root = filepath.Join(os.TempDir(), "self-"+strconv.Itoa(os.Getuid()), "reservations")
		}
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	return canonicalPath(root)
}

func ensureReservationRoot(root string) (string, error) {
	for _, path := range []string{filepath.Dir(root), root} {
		if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("reservation directory %s is a symlink", path)
		}
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	if err := os.Chmod(root, 0700); err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(root)
}

func reservationPath(repo string) (string, string, string, error) {
	canonical, identity, err := repositoryIdentity(repo)
	if err != nil {
		return "", "", "", err
	}
	root, err := reservationRootCandidate()
	if err != nil {
		return "", "", "", err
	}
	if pathWithin(canonical, root) {
		return "", "", "", fmt.Errorf("reservation directory %s must be outside repository %s", root, canonical)
	}
	worktrees, listErr := registeredWorktrees(repo)
	if listErr != nil {
		return "", "", "", fmt.Errorf("enumerating repository worktrees for reservation placement: %w", listErr)
	}
	for _, worktree := range worktrees {
		canonicalWorktree, err := filepath.EvalSymlinks(worktree.Path)
		if err == nil && pathWithin(canonicalWorktree, root) {
			return "", "", "", fmt.Errorf("reservation directory %s must be outside every worktree of repository %s", root, canonical)
		}
	}
	root, err = ensureReservationRoot(root)
	if err != nil {
		return "", "", "", err
	}
	return filepath.Join(root, identity+".lock"), canonical, identity, nil
}

func acquireRepositoryReservation(repo, purpose string) (*repositoryReservation, error) {
	path, canonical, identity, err := reservationPath(repo)
	if err != nil {
		return nil, err
	}
	fd, err := syscall.Open(path, syscall.O_CREAT|syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		evidence, _ := os.ReadFile(path)
		file.Close()
		if err == syscall.EWOULDBLOCK || err == syscall.EAGAIN {
			return nil, fmt.Errorf("repository %s is reserved by another self writer (%s)", canonical, strings.TrimSpace(string(evidence)))
		}
		return nil, err
	}
	owner := fmt.Sprintf("pid=%d caller=%s purpose=%s repository=%s acquired=%s\n", os.Getpid(), callerClaim(), purpose, canonical, time.Now().UTC().Format(time.RFC3339Nano))
	if err := file.Truncate(0); err != nil {
		file.Close()
		return nil, err
	}
	if _, err := file.WriteAt([]byte(owner), 0); err != nil {
		file.Close()
		return nil, err
	}
	return &repositoryReservation{file: file, Path: path, Identity: identity, Repository: canonical}, nil
}

func (r *repositoryReservation) Release() error {
	if r == nil || r.file == nil {
		return nil
	}
	err := syscall.Flock(int(r.file.Fd()), syscall.LOCK_UN)
	closeErr := r.file.Close()
	r.file = nil
	if err != nil {
		return err
	}
	return closeErr
}

func runReservedChild(ctx context.Context, argv []string, stdin io.Reader, stdout, stderr io.Writer, reservation *repositoryReservation) error {
	if len(argv) == 0 {
		return fmt.Errorf("reservation needs a command after --")
	}
	cmd := commandWithGroup(ctx, argv[0], argv[1:]...)
	cmd.Env = append(os.Environ(), "SELF_REPOSITORY_RESERVATION="+reservation.Identity, "SELF_REPOSITORY_RESERVATION_FD=3")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	// The helper inherits the same open file description. If this wrapper is
	// killed, flock remains held until the dispatch helper itself exits.
	cmd.ExtraFiles = []*os.File{reservation.file}
	return cmd.Run()
}

func cmdReserve(home string, args []string, out, diag io.Writer) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: self reserve path|check|exec|dispatch <repository> [-- command...]")
	}
	op, repo := args[0], args[1]
	switch op {
	case "path":
		if len(args) != 2 {
			return fmt.Errorf("usage: self reserve path <repository>")
		}
		path, canonical, identity, err := reservationPath(repo)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "%s\t%s\t%s\n", identity, canonical, path)
		return err
	case "check":
		if len(args) != 2 {
			return fmt.Errorf("usage: self reserve check <repository>")
		}
		reservation, err := acquireRepositoryReservation(repo, "check")
		if err != nil {
			return err
		}
		defer reservation.Release()
		_, err = fmt.Fprintf(out, "available\t%s\t%s\n", reservation.Identity, reservation.Repository)
		return err
	case "exec", "dispatch":
		if len(args) < 4 || args[2] != "--" {
			return fmt.Errorf("usage: self reserve %s <repository> -- <command> [args...]", op)
		}
		reservation, err := acquireRepositoryReservation(repo, op)
		if err != nil {
			return err
		}
		defer reservation.Release()
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if op == "exec" {
			return runReservedChild(ctx, args[3:], os.Stdin, out, diag, reservation)
		}
		var wire boundedBuffer
		wire.Limit = lineLimit * 4
		if err := runReservedChild(ctx, args[3:], os.Stdin, &wire, diag, reservation); err != nil {
			return err
		}
		events, scripts, _, err := wireEvents(wire.Bytes())
		if err != nil {
			return err
		}
		if len(scripts) > 0 {
			return fmt.Errorf("reserved dispatch helper emitted capability authoring instead of dispatch evidence")
		}
		authoritative := false
		for _, event := range events {
			if event.Name != "agent.started" && event.Name != "agent.failed" {
				continue
			}
			var payload struct {
				Agent      string `json:"agent"`
				Repo       string `json:"repo"`
				Repository string `json:"repository"`
				Worktree   string `json:"worktree"`
			}
			if json.Unmarshal(event.Payload, &payload) != nil || payload.Agent == "" {
				continue
			}
			for _, path := range []string{payload.Repository, payload.Repo, payload.Worktree} {
				if path == "" {
					continue
				}
				_, identity, identityErr := repositoryIdentity(path)
				if identityErr == nil && identity == reservation.Identity {
					authoritative = true
					break
				}
			}
		}
		if !authoritative {
			return fmt.Errorf("reserved dispatch helper emitted no agent.started or agent.failed evidence")
		}
		// Commit dispatch evidence before unlocking, so a guarded contender can
		// never slip between agent start and agent.started becoming authoritative.
		if err := cmdHear(home, bytes.Clone(wire.Bytes()), io.Discard); err != nil {
			return fmt.Errorf("committing reserved dispatch evidence: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unknown reserve operation %q", op)
	}
}

func wireEvents(input []byte) ([]Event, []authored, []string, error) {
	return wire(string(input))
}
