// self-browse — open a view of a self instance in the system browser.
//
//	self-browse [view [arg…]]
//
// Probes the self-serve port. If nothing is listening, it starts one detached
// beside this binary and waits for the port, then hands the URL to xdg-open
// (or `open` on Darwin). The browser is the client; the server stays
// stateless; the log stays the only state.
package main

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const defaultPort = "8377"

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}
	u := browseURL(port, os.Args[1:])

	if !listening(port) {
		if err := startServer(port); err != nil {
			fmt.Fprintf(os.Stderr, "self-browse: %s\n", err)
			os.Exit(1)
		}
	}

	if err := openURL(u); err != nil {
		fmt.Println(u)
	}
}

func browseURL(port string, args []string) string {
	u := "http://127.0.0.1:" + port
	if len(args) == 0 {
		return u
	}
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = url.PathEscape(a)
	}
	return u + "/view/" + strings.Join(parts, "/")
}

func listening(port string) bool {
	c, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 300*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// startServer detaches a self-serve next to this binary and waits for the
// port. Output goes nowhere: a server that cannot bind will not come up, and
// the wait below is the honest diagnostic.
func startServer(port string) error {
	bin, err := lookServe()
	if err != nil {
		return err
	}
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "PORT="+port)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	for i := 0; i < 100; i++ {
		if listening(port) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("self-serve did not come up on 127.0.0.1:%s", port)
}

func lookServe() (string, error) {
	if b := os.Getenv("SELF_SERVE"); b != "" {
		return b, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	bin := filepath.Join(filepath.Dir(exe), "self-serve")
	if _, err := os.Stat(bin); err == nil {
		return bin, nil
	}
	if p, err := exec.LookPath("self-serve"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("self-serve not found next to %s", exe)
}

func openURL(u string) error {
	for _, name := range []string{"xdg-open", "open"} {
		p, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		return exec.Command(p, u).Run()
	}
	return fmt.Errorf("no opener")
}
