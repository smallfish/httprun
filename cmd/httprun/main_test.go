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

func TestRealMainHelpOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := realMain([]string{"--help"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "NAME") || !strings.Contains(output, "SYNOPSIS") || !strings.Contains(output, "COMMANDS") {
		t.Fatalf("expected man-style help sections, got %q", output)
	}
	if !strings.Contains(output, "httprun run [flags] <file.http> [more.http ...]") {
		t.Fatalf("expected run synopsis, got %q", output)
	}
	if !strings.Contains(output, "httprun validate [flags] <file.http> [more.http ...]") {
		t.Fatalf("expected validate synopsis, got %q", output)
	}
}

func TestRealMainRunHelpOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := realMain([]string{"run", "--help"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "httprun run - send requests from .http files") {
		t.Fatalf("expected run help heading, got %q", output)
	}
	if !strings.Contains(output, "--fail-http         Return non-zero when HTTP status is >= 400") {
		t.Fatalf("expected run-specific flag help, got %q", output)
	}
}

func TestRealMainValidateHelpOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := realMain([]string{"validate", "--help"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "httprun validate - check .http files without sending requests") {
		t.Fatalf("expected validate help heading, got %q", output)
	}
	if !strings.Contains(output, "--jobs <n>          Number of .http files to check at the same time") {
		t.Fatalf("expected validate jobs help, got %q", output)
	}
}

func TestRealMainUnknownCommandShowsManualStyleError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := realMain([]string{"nope"}, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", stdout.String())
	}

	output := stderr.String()
	if !strings.Contains(output, "unknown command: nope") {
		t.Fatalf("expected unknown command error, got %q", output)
	}
	if !strings.Contains(output, "SYNOPSIS") {
		t.Fatalf("expected usage after unknown command, got %q", output)
	}
}

func TestRealMainRunMissingPathShowsManualStyleError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := realMain([]string{"run"}, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", stdout.String())
	}

	output := stderr.String()
	if !strings.Contains(output, "missing .http file path for run") {
		t.Fatalf("expected missing path error, got %q", output)
	}
	if !strings.Contains(output, "httprun run [flags] <file.http> [more.http ...]") {
		t.Fatalf("expected run usage after missing path, got %q", output)
	}
}

func TestRealMainValidateMissingPathShowsManualStyleError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := realMain([]string{"validate"}, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", stdout.String())
	}

	output := stderr.String()
	if !strings.Contains(output, "missing .http file path for validate") {
		t.Fatalf("expected missing path error, got %q", output)
	}
	if !strings.Contains(output, "httprun validate [flags] <file.http> [more.http ...]") {
		t.Fatalf("expected validate usage after missing path, got %q", output)
	}
}

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
	if strings.Index(output, "== "+first+" ==") == -1 || strings.Index(output, "== "+second+" ==") == -1 {
		t.Fatalf("expected file headings, got %q", output)
	}
	if strings.Index(output, "1. first") == -1 || strings.Index(output, "1. second") == -1 {
		t.Fatalf("expected both request outputs, got %q", output)
	}
	if strings.Count(output, "Summary: 1 requests, 1 passed") != 2 {
		t.Fatalf("expected per-file summaries, got %q", output)
	}
	if strings.Index(output, "== "+first+" ==") > strings.Index(output, "== "+second+" ==") {
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

func TestRealMainRunSummarizesHTTPFailuresWithoutFailingByDefault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer server.Close()

	tempDir := t.TempDir()
	path := writeHTTPFile(t, tempDir, "bad.http", "# @name bad\nGET {{base}}/bad\n")

	var stdout, stderr bytes.Buffer
	code := realMain([]string{
		"run",
		"--var", "base=" + server.URL,
		path,
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "400 Bad Request") {
		t.Fatalf("expected HTTP error status in output, got %q", output)
	}
	if !strings.Contains(output, "Summary: 1 requests, 0 passed, 1 failed") {
		t.Fatalf("expected failed summary, got %q", output)
	}
}

func TestRealMainRunFailHTTPDoesNotRepeatHTTPErrorOnStderr(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer server.Close()

	tempDir := t.TempDir()
	path := writeHTTPFile(t, tempDir, "bad.http", "# @name bad\nGET {{base}}/bad\n")

	var stdout, stderr bytes.Buffer
	code := realMain([]string{
		"run",
		"--fail-http",
		"--var", "base=" + server.URL,
		path,
	}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr for HTTP failure, got %q", stderr.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "400 Bad Request") {
		t.Fatalf("expected HTTP error status in output, got %q", output)
	}
	if !strings.Contains(output, "Summary: 1 requests, 0 passed, 1 failed") {
		t.Fatalf("expected failed summary, got %q", output)
	}
}

func TestRealMainRunAssertionFailureDoesNotRepeatOnStderr(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"name":"demo"}}`))
	}))
	defer server.Close()

	tempDir := t.TempDir()
	path := writeHTTPFile(t, tempDir, "assert.http", strings.TrimSpace(`
###
# @name check
# @assert status == 200
# @assert json.data.name == "other"
GET {{base}}/check
`))

	var stdout, stderr bytes.Buffer
	code := realMain([]string{
		"run",
		"--var", "base=" + server.URL,
		path,
	}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr for assertion failure, got %q", stderr.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "Assertion Failures:") {
		t.Fatalf("expected assertion failures in output, got %q", output)
	}
	if !strings.Contains(output, "Summary: 1 requests, 0 passed, 1 failed") {
		t.Fatalf("expected failed summary, got %q", output)
	}
}

func TestRealMainRunAssertionFailureSummarizesSkippedRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		switch r.URL.Path {
		case "/first":
			_, _ = w.Write([]byte(`{"data":{"name":"demo"}}`))
		case "/second":
			_, _ = w.Write([]byte(`{"data":{"name":"second"}}`))
		}
	}))
	defer server.Close()

	tempDir := t.TempDir()
	path := writeHTTPFile(t, tempDir, "assert-skip.http", strings.TrimSpace(`
###
# @name first
# @assert json.data.name == "other"
GET {{base}}/first

###
# @name second
GET {{base}}/second
`))

	var stdout, stderr bytes.Buffer
	code := realMain([]string{
		"run",
		"--var", "base=" + server.URL,
		path,
	}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr for assertion failure, got %q", stderr.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "Summary: 1/2 executed, 0 passed, 1 failed, 1 skipped") {
		t.Fatalf("expected skipped summary, got %q", output)
	}
	if strings.Contains(output, "2. second") {
		t.Fatalf("did not expect second request output, got %q", output)
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
