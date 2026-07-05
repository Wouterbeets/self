//go:build linux

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// exec re-enters this binary as `self __jail` inside fresh user, mount, pid,
// and network namespaces; the child builds the jail and becomes bash.
func (p *playpen) exec(command string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/proc/self/exe", "__jail", p.root, command)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUSER | syscall.CLONE_NEWNS |
			syscall.CLONE_NEWPID | syscall.CLONE_NEWNET,
		UidMappings:                []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getuid(), Size: 1}},
		GidMappings:                []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getgid(), Size: 1}},
		GidMappingsEnableSetgroups: false,
		Pdeathsig:                  syscall.SIGKILL,
	}
	return cmd.CombinedOutput()
}

// cmdJail is the child side of the playpen: pid 1 of a fresh namespace set,
// root only within it. It assembles a throwaway filesystem view — system dirs
// read-only, the instance copy read-write at /body, no host paths beyond those —
// pivots into it, and becomes bash. The command sees SELF_HOME=/body, so
// candidate scripts run against the copied log exactly as they would for real.
func cmdJail(root, command string) error {
	jail := filepath.Join(root, "jail")
	body := filepath.Join(root, "body")
	if err := os.MkdirAll(jail, 0755); err != nil {
		return err
	}
	// unshare mount propagation, then make the jail dir a mount point
	if err := syscall.Mount("", "/", "", syscall.MS_REC|syscall.MS_PRIVATE, ""); err != nil {
		return fmt.Errorf("private /: %w", err)
	}
	if err := syscall.Mount(jail, jail, "", syscall.MS_BIND, ""); err != nil {
		return fmt.Errorf("bind jail: %w", err)
	}
	bindRO := func(src, dst string) error {
		if err := os.MkdirAll(dst, 0755); err != nil {
			return err
		}
		if err := syscall.Mount(src, dst, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
			return err
		}
		return syscall.Mount("", dst, "", syscall.MS_REMOUNT|syscall.MS_BIND|syscall.MS_RDONLY|syscall.MS_REC|syscall.MS_NOSUID, "")
	}
	for _, d := range []string{"/usr", "/etc", "/opt"} {
		if _, err := os.Stat(d); err == nil {
			if err := bindRO(d, filepath.Join(jail, d)); err != nil {
				return fmt.Errorf("bind %s: %w", d, err)
			}
		}
	}
	// merged-usr symlinks (/bin -> usr/bin and friends) are recreated, real
	// directories are bound read-only
	for _, l := range []string{"bin", "sbin", "lib", "lib64", "lib32"} {
		host := "/" + l
		fi, err := os.Lstat(host)
		if err != nil {
			continue
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			target, _ := os.Readlink(host)
			os.Symlink(target, filepath.Join(jail, l))
		} else if fi.IsDir() {
			if err := bindRO(host, filepath.Join(jail, l)); err != nil {
				return fmt.Errorf("bind %s: %w", host, err)
			}
		}
	}
	// /tmp lives on the jail's own tree: writes stay inside the playpen dir
	os.MkdirAll(filepath.Join(jail, "tmp"), 0777)
	os.MkdirAll(filepath.Join(jail, "dev"), 0755)
	if err := syscall.Mount("/dev", filepath.Join(jail, "dev"), "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("bind /dev: %w", err)
	}
	os.MkdirAll(filepath.Join(jail, "proc"), 0755)
	os.MkdirAll(filepath.Join(jail, "body"), 0755)
	if err := syscall.Mount(body, filepath.Join(jail, "body"), "", syscall.MS_BIND, ""); err != nil {
		return fmt.Errorf("bind body: %w", err)
	}
	// pivot: the host filesystem ends here
	old := filepath.Join(jail, ".host")
	if err := os.MkdirAll(old, 0755); err != nil {
		return err
	}
	if err := syscall.PivotRoot(jail, old); err != nil {
		return fmt.Errorf("pivot_root: %w", err)
	}
	if err := os.Chdir("/"); err != nil {
		return err
	}
	if err := syscall.Unmount("/.host", syscall.MNT_DETACH); err != nil {
		return fmt.Errorf("unmount host: %w", err)
	}
	os.Remove("/.host")
	syscall.Mount("proc", "/proc", "proc", 0, "")
	if err := os.Chdir("/body"); err != nil {
		return err
	}
	env := []string{
		"PATH=/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin",
		"HOME=/body", "SELF_HOME=/body", "SELF_PLAYPEN=1",
		"TERM=dumb", "LANG=C.UTF-8",
	}
	return syscall.Exec("/bin/bash", []string{"bash", "-c", command}, env)
}
