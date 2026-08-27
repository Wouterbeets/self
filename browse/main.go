// self-browse — open a view of a self instance in the system browser.
//
//	self-browse [view]
//
// Probes the self-serve port. If nothing is listening, it starts one detached
// beside this binary and waits for the port, then hands the URL to xdg-open.
// The browser is the client; the server stays stateless; the log stays the
// only state.
package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

const defaultPort = "8377"

func main() {
	view := ""
	if len(os.Args) > 1 {
		view = os.Args[1]
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}
	url := "http://127.0.0.1:" + port
	if view != "" {
		url += "/view/" + view
	}

	if !listening(port) {
		if err := startServer(port); err != nil {
			fmt.Fprintf(os.Stderr, "self-browse: %s\n", err)
			os.Exit(1)
		}
	}

	if err := exec.Command("xdg-open", url).Run(); err != nil {
		fmt.Println(url)
	}
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
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	bin := filepath.Join(filepath.Dir(exe), "self-serve")
	if _, err := os.Stat(bin); err != nil {
		bin, err = exec.LookPath("self-serve")
		if err != nil {
			return fmt.Errorf("self-serve not found next to %s", exe)
		}
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
