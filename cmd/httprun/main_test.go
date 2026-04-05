package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRealMainRunSupportsMultipleFilesAndJobs(t *testing.T) {
	var (
		mu        sync.Mutex
		active    int
		maxActive int
		ready     = make(chan struct{})
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		active++
		if active > maxActive {
			maxActive = active
		}
		if active == 2 {
			select {
			case <-ready:
			default:
				close(ready)
			}
		}
		mu.Unlock()

		select {
		case <-ready:
		case <-time.After(300 * time.Millisecond):
		}

		fmt.Fprintf(w, `{"path":%q}`, r.URL.Path)

		mu.Lock()
		active--
		mu.Unlock()
	}))
	defer server.Close()

	tempDir := t.TempDir()
	first := writeHTTPFile(t, tempDir, "first.http", "# @name first\nGET {{base}}/first\n")
	second := writeHTTPFile(t, tempDir, "second.http", "# @name second\nGET {{base}}/second\n")

	var stdout, stderr bytes.Buffer
	code := realMain([]string{
		"run",
		"--jobs", "2",
		"--var", "base=" + server.URL,
		first,
		second,
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}

	output := stdout.String()
	if maxActive != 2 {
		t.Fatalf("expected concurrent execution across files, maxActive=%d", maxActive)
	}
	if strings.Index(output, "==> first") == -1 || strings.Index(output, "==> second") == -1 {
		t.Fatalf("expected both request outputs, got %q", output)
	}
	if strings.Index(output, "==> first") > strings.Index(output, "==> second") {
		t.Fatalf("expected outputs to preserve input order, got %q", output)
	}
}

func TestRealMainValidateSupportsMultipleFilesAndJobs(t *testing.T) {
	tempDir := t.TempDir()
	first := writeHTTPFile(t, tempDir, "first.http", "# @name first\nGET {{base}}/first\n")
	second := writeHTTPFile(t, tempDir, "second.http", "# @name second\nGET {{base}}/second\n")

	var stdout, stderr bytes.Buffer
	code := realMain([]string{
		"validate",
		"--jobs", "2",
		"--var", "base=https://example.com",
		first,
		second,
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", stdout.String())
	}

	output := stderr.String()
	if !strings.Contains(output, first+": OK") {
		t.Fatalf("expected first file OK, got %q", output)
	}
	if !strings.Contains(output, second+": OK") {
		t.Fatalf("expected second file OK, got %q", output)
	}
	if strings.Index(output, first+": OK") > strings.Index(output, second+": OK") {
		t.Fatalf("expected validation output in input order, got %q", output)
	}
}

func TestRealMainRejectsInvalidJobs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := realMain([]string{"run", "--jobs", "0", "demo.http"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "--jobs must be greater than 0") {
		t.Fatalf("unexpected stderr %q", stderr.String())
	}
}

func writeHTTPFile(t *testing.T, dir, name, body string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}
