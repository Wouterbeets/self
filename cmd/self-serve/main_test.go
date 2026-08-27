package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveName(t *testing.T) {
	names := []string{"capture", "timer/set", "timer/cancel", "timer"}
	cases := []struct {
		segs []string
		name string
		args []string
	}{
		{[]string{"capture", "hello"}, "capture", []string{"hello"}},
		{[]string{"timer", "set", "n", "+30s"}, "timer/set", []string{"n", "+30s"}},
		{[]string{"timer", "cancel", "n"}, "timer/cancel", []string{"n"}},
		{[]string{"timer", "tick"}, "timer", []string{"tick"}},
		{[]string{"unknown", "x"}, "unknown", []string{"x"}},
		{[]string{"capture"}, "capture", nil},
	}
	for _, c := range cases {
		name, args := resolveName(c.segs, names)
		if name != c.name {
			t.Errorf("%v: name=%q want %q", c.segs, name, c.name)
		}
		if strings.Join(args, "\x00") != strings.Join(c.args, "\x00") {
			t.Errorf("%v: args=%q want %q", c.segs, args, c.args)
		}
	}
}

func TestNamesInSection(t *testing.T) {
	brief := `# self — /tmp/x

## commands — ` + "`self run <name> [args…]`" + `

- **capture** — Capture a task
- **timer/set** — Schedule an intention

## views — ` + "`self view <name> [args…]`" + `

- **menu** — HTML menu
- **board** — task board
- **log** — every event

## pending — declared, no script yet

- view/later (declared at seq 1)
`
	cmds := namesInSection(brief, "## commands")
	if strings.Join(cmds, ",") != "capture,timer/set" {
		t.Fatalf("commands: %v", cmds)
	}
	views := namesInSection(brief, "## views")
	if strings.Join(views, ",") != "menu,board,log" {
		t.Fatalf("views: %v", views)
	}
}

func TestHTTPOverKernel(t *testing.T) {
	stub := writeStub(t)
	t.Setenv("SELF_BIN", stub)

	srv := httptest.NewServer(handler())
	t.Cleanup(srv.Close)
	client := srv.Client()

	t.Run("index is the brief, views are links", func(t *testing.T) {
		res := get(t, client, srv.URL+"/")
		defer res.Body.Close()
		body := read(t, res)
		if res.StatusCode != 200 {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		if !strings.Contains(body, `href="/view/menu"`) {
			t.Fatalf("index did not link views:\n%s", body)
		}
		if !strings.Contains(body, "<strong>menu</strong>") {
			t.Fatalf("index lost the view name:\n%s", body)
		}
		if strings.Contains(body, `href="/run/capture"`) {
			t.Fatal("index must not turn commands into GET links")
		}
		if !strings.Contains(body, "<strong>capture</strong>") {
			t.Fatal("commands lost their names")
		}
		if strings.Contains(body, "**capture**") {
			t.Fatal("command names still wearing markdown")
		}
		if res.Header.Get("ETag") == "" {
			t.Fatal("missing etag")
		}
	})

	t.Run("view index is self view with no name", func(t *testing.T) {
		res := get(t, client, srv.URL+"/view/")
		defer res.Body.Close()
		body := read(t, res)
		if !strings.Contains(body, `href="/view/board"`) {
			t.Fatalf("view index did not link board:\n%s", body)
		}
	})

	t.Run("html view passes through", func(t *testing.T) {
		res := get(t, client, srv.URL+"/view/menu")
		defer res.Body.Close()
		body := read(t, res)
		if !strings.Contains(body, "<h1>menu-doc</h1>") {
			t.Fatalf("html view wrapped or lost: %s", body)
		}
		if !strings.Contains(body, "setInterval") {
			t.Fatal("refresh script missing on html view")
		}
	})

	t.Run("text view is wrapped", func(t *testing.T) {
		res := get(t, client, srv.URL+"/view/board")
		defer res.Body.Close()
		body := read(t, res)
		if !strings.Contains(body, "now (1)") {
			t.Fatalf("text lost: %s", body)
		}
		if !strings.Contains(body, "<pre>") {
			t.Fatal("text view was not wrapped")
		}
	})

	t.Run("silent view is named empty", func(t *testing.T) {
		res := get(t, client, srv.URL+"/view/quiet")
		defer res.Body.Close()
		body := read(t, res)
		if !strings.Contains(body, "Nothing.") {
			t.Fatalf("silence: %s", body)
		}
	})

	t.Run("view args ride the path", func(t *testing.T) {
		res := get(t, client, srv.URL+"/view/echo/one/two")
		defer res.Body.Close()
		body := read(t, res)
		if !strings.Contains(body, "one two") {
			t.Fatalf("args not passed: %s", body)
		}
	})

	t.Run("etag 304 when the kernel bytes did not move", func(t *testing.T) {
		res := get(t, client, srv.URL+"/view/board")
		etag := res.Header.Get("ETag")
		res.Body.Close()
		if etag == "" {
			t.Fatal("no etag")
		}
		req, _ := http.NewRequest("GET", srv.URL+"/view/board", nil)
		req.Header.Set("If-None-Match", etag)
		res2, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res2.Body.Close()
		if res2.StatusCode != http.StatusNotModified {
			t.Fatalf("got %d want 304", res2.StatusCode)
		}
	})

	t.Run("slash command is one name", func(t *testing.T) {
		res := get(t, client, srv.URL+"/run/timer/set/bell/+30s")
		defer res.Body.Close()
		body := read(t, res)
		if res.StatusCode != 200 {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		if strings.TrimSpace(body) != "run timer/set bell +30s" {
			t.Fatalf("argv: %q", body)
		}
	})

	t.Run("ordinary command", func(t *testing.T) {
		res := get(t, client, srv.URL+"/run/capture/hello")
		defer res.Body.Close()
		body := read(t, res)
		if strings.TrimSpace(body) != "run capture hello" {
			t.Fatalf("argv: %q", body)
		}
	})

	t.Run("caller is the browser", func(t *testing.T) {
		t.Setenv("SELF_CALLER", "someone-else")
		res := get(t, client, srv.URL+"/run/capture/x")
		res.Body.Close()
		got, err := os.ReadFile(filepath.Join(filepath.Dir(stub), "caller"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(string(got)) != "browser" {
			t.Fatalf("SELF_CALLER=%q", got)
		}
	})
}

func TestUnknownPathIs404(t *testing.T) {
	t.Setenv("SELF_BIN", writeStub(t))
	srv := httptest.NewServer(handler())
	t.Cleanup(srv.Close)
	res := get(t, srv.Client(), srv.URL+"/nope")
	defer res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func writeStub(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "self")
	// No backticks: a Go raw string cannot hold them, and the stub does
	// not need command substitution.
	script := `#!/bin/sh
dir=${0%/*}
printf '%s\n' "$SELF_CALLER" > "$dir/caller"
printf '%s\n' "$*" > "$dir/argv"
case "$1" in
  brief)
    cat <<'EOF'
# self — /tmp/x

## commands

- **capture** — Capture a task
- **timer/set** — Schedule an intention

## views

- **menu** — HTML menu
- **board** — task board
- **echo** — echo argv
- **quiet** — silence
- **log** — every event
EOF
    ;;
  view)
    if [ -z "$2" ]; then
      cat <<'EOF'
usage: self view <name> [args...]

views on this instance:
- menu — HTML menu
- board — task board
- echo — echo argv
- quiet — silence
- log — every event
EOF
      exit 0
    fi
    case "$2" in
      menu) printf '%s\n' '<!doctype html><html><body><h1>menu-doc</h1></body></html>' ;;
      board) printf '%s\n' 'now (1)' ;;
      quiet) ;;
      echo) shift 2; printf '%s\n' "$*" ;;
      *) echo "no view $2" >&2; exit 1 ;;
    esac
    ;;
  run)
    shift
    printf 'run %s\n' "$*"
    ;;
  *)
    echo "unexpected: $*" >&2
    exit 2
    ;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func get(t *testing.T, c *http.Client, url string) *http.Response {
	t.Helper()
	res, err := c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func read(t *testing.T, res *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
