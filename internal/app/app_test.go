package app

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type capturedRequest struct {
	Method string
	Path   string
	Query  string
	Header http.Header
	Body   string
}

func TestRunExecutesAllCommonRequests(t *testing.T) {
	var captured []capturedRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		captured = append(captured, capturedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.RawQuery,
			Header: r.Header.Clone(),
			Body:   string(body),
		})

		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	testdataDir := filepath.Join("testdata")
	httpPath := filepath.Join(testdataDir, "all_methods.http")

	var stdout bytes.Buffer
	_, err := Run(context.Background(), &stdout, RunOptions{
		Path:            httpPath,
		EnvironmentName: "dev",
		CLIOverrides: map[string]string{
			"base": server.URL,
		},
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(captured) != 6 {
		t.Fatalf("expected 6 requests, got %d", len(captured))
	}

	assertRequest(t, captured[0], http.MethodGet, "/items", "page=1", "", map[string]string{
		"Accept":   "application/json",
		"X-Secret": "private-only",
	})
	assertRequest(t, captured[1], http.MethodPost, "/items", "", "{\"source\":\"public-env\",\"token\":\"public-token\",\"secret\":\"private-only\"}\n", map[string]string{
		"Content-Type": "application/json",
		"X-Token":      "public-token",
	})
	assertRequest(t, captured[2], http.MethodPut, "/items/42", "", `{"op":"replace","value":"updated-from-cli"}`, map[string]string{
		"Content-Type": "application/json",
	})
	assertRequest(t, captured[3], http.MethodPatch, "/items/42", "", `{"op":"patch"}`, map[string]string{
		"Content-Type": "application/json",
	})
	assertRequest(t, captured[4], http.MethodDelete, "/items/42", "", "", map[string]string{
		"X-Delete-Reason": "cleanup",
	})
	assertRequest(t, captured[5], http.MethodHead, "/health", "", "", nil)

	output := stdout.String()
	if strings.Count(output, "200 OK") != 5 || strings.Count(output, "204 No Content") != 1 {
		t.Fatalf("expected 6 responses in output, got %q", output)
	}
}

func TestValidateSupportsExternalBodyAndEnvFiles(t *testing.T) {
	err := Validate(ValidateOptions{
		Path:            filepath.Join("testdata", "all_methods.http"),
		EnvironmentName: "dev",
		CLIOverrides: map[string]string{
			"base": "https://example.com",
		},
	})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestRunRequestOptions(t *testing.T) {
	var paths []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)

		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, "/final", http.StatusFound)
		case "/final":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("redirected"))
		case "/login":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "shared", Path: "/"})
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("logged-in"))
		case "/whoami":
			if _, err := r.Cookie("session"); err == nil {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("cookie=present"))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("cookie=absent"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	httpPath := filepath.Join("testdata", "request_options.http")

	var redirectOut bytes.Buffer
	if _, err := Run(context.Background(), &redirectOut, RunOptions{
		Path:        httpPath,
		RequestName: "followsRedirect",
		CLIOverrides: map[string]string{
			"base": server.URL,
		},
		Timeout: 5 * time.Second,
	}); err != nil {
		t.Fatalf("Run() followsRedirect error = %v", err)
	}
	if !strings.Contains(redirectOut.String(), "200 OK") {
		t.Fatalf("expected redirect follow to end in 200, got %q", redirectOut.String())
	}

	var noRedirectOut bytes.Buffer
	if _, err := Run(context.Background(), &noRedirectOut, RunOptions{
		Path:        httpPath,
		RequestName: "noRedirect",
		CLIOverrides: map[string]string{
			"base": server.URL,
		},
		Timeout: 5 * time.Second,
	}); err != nil {
		t.Fatalf("Run() noRedirect error = %v", err)
	}
	if !strings.Contains(noRedirectOut.String(), "302 Found") {
		t.Fatalf("expected no-redirect request to return 302, got %q", noRedirectOut.String())
	}

	var cookieOut bytes.Buffer
	if _, err := Run(context.Background(), &cookieOut, RunOptions{
		Path: httpPath,
		CLIOverrides: map[string]string{
			"base": server.URL,
		},
		Timeout: 5 * time.Second,
		Verbose: true,
	}); err != nil {
		t.Fatalf("Run() cookie flow error = %v", err)
	}
	output := cookieOut.String()
	if !strings.Contains(output, "cookie=absent") {
		t.Fatalf("expected isolated request to avoid saving cookies, got %q", output)
	}
	if !strings.Contains(output, "cookie=present") {
		t.Fatalf("expected shared cookie jar to persist cookies, got %q", output)
	}
	if strings.Count(strings.Join(paths, ","), "/final") == 0 {
		t.Fatalf("expected redirect target to be hit, got %v", paths)
	}
}

func TestRunRequestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("slow"))
	}))
	defer server.Close()

	_, err := Run(context.Background(), &bytes.Buffer{}, RunOptions{
		Path:        filepath.Join("testdata", "timeout.http"),
		RequestName: "slowRequest",
		CLIOverrides: map[string]string{
			"base": server.URL,
		},
		Timeout: 5 * time.Second,
	})
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestRunNameSelectsOnlyTargetRequest(t *testing.T) {
	var calls []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	httpPath := filepath.Join("testdata", "name_selection.http")

	if _, err := Run(context.Background(), &bytes.Buffer{}, RunOptions{
		Path:        httpPath,
		RequestName: "good",
		CLIOverrides: map[string]string{
			"base": server.URL,
		},
		Timeout: 5 * time.Second,
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(calls) != 1 || calls[0] != "/good" {
		t.Fatalf("expected only /good to run, got %v", calls)
	}

	if err := Validate(ValidateOptions{
		Path:        httpPath,
		RequestName: "good",
		CLIOverrides: map[string]string{
			"base": server.URL,
		},
	}); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestRunNameMissingRequest(t *testing.T) {
	_, err := Run(context.Background(), &bytes.Buffer{}, RunOptions{
		Path:        filepath.Join("testdata", "name_selection.http"),
		RequestName: "doesNotExist",
		CLIOverrides: map[string]string{
			"base": "https://example.com",
		},
		Timeout: 5 * time.Second,
	})
	if err == nil {
		t.Fatalf("expected missing request error")
	}
	if !strings.Contains(err.Error(), `request "doesNotExist" not found`) {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestRunAssertionsFailRequestAndShowDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"hello","data":{"items":[{"name":"demo"}]}}`))
	}))
	defer server.Close()

	tempDir := t.TempDir()
	httpPath := filepath.Join(tempDir, "assert.http")
	if err := os.WriteFile(httpPath, []byte(strings.TrimSpace(`
###
# @name assertDemo
# @assert status == 200
# @assert body contains hello
# @assert json.data.items.0.name == "other"
GET {{base}}/check
`)), 0o644); err != nil {
		t.Fatalf("write http file: %v", err)
	}

	var stdout bytes.Buffer
	stats, err := Run(context.Background(), &stdout, RunOptions{
		Path: httpPath,
		CLIOverrides: map[string]string{
			"base": server.URL,
		},
		Timeout: 5 * time.Second,
	})
	if err == nil {
		t.Fatalf("expected assertion failure")
	}

	if !strings.Contains(err.Error(), `expected json.data.items.0.name == "other", got "demo"`) {
		t.Fatalf("unexpected error %v", err)
	}
	if stats.Executed != 1 || stats.Failed != 1 || stats.Passed != 0 {
		t.Fatalf("unexpected stats %+v", stats)
	}

	output := stdout.String()
	if !strings.Contains(output, "Assertion Failures:") {
		t.Fatalf("expected assertion failures in output, got %q", output)
	}
	if !strings.Contains(output, `"message":"hello"`) {
		t.Fatalf("expected response body in output, got %q", output)
	}
}

func TestRunAssertionsPassAcrossSubjectsAndOperators(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/status":
			w.WriteHeader(http.StatusNoContent)
		case "/text":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("hello"))
		case "/json":
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Trace-Id", "trace-123")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":{"name":"demo","count":2,"enabled":true,"items":["one","two"]}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	tempDir := t.TempDir()
	httpPath := filepath.Join(tempDir, "assert-pass.http")
	if err := os.WriteFile(httpPath, []byte(strings.TrimSpace(`
###
# @name statusNoBody
# @assert status == 204
# @assert body not_exists
GET {{base}}/status

###
# @name plainText
# @assert status != 500
# @assert body exists
# @assert body == hello
# @assert body not_contains error
GET {{base}}/text

###
# @name jsonAndHeaders
# @assert status >= 200
# @assert status < 300
# @assert header.X-Trace-Id exists
# @assert header.X-Trace-Id == trace-123
# @assert header.Content-Type contains application/json
# @assert header.X-Missing not_exists
# @assert json.data.name == "demo"
# @assert json.data.name != "other"
# @assert json.data.count >= 2
# @assert json.data.count <= 2
# @assert json.data.items exists
# @assert json.data.missing not_exists
GET {{base}}/json
`)), 0o644); err != nil {
		t.Fatalf("write http file: %v", err)
	}

	var stdout bytes.Buffer
	stats, err := Run(context.Background(), &stdout, RunOptions{
		Path: httpPath,
		CLIOverrides: map[string]string{
			"base": server.URL,
		},
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if stats.Selected != 3 || stats.Executed != 3 || stats.Passed != 3 || stats.Failed != 0 {
		t.Fatalf("unexpected stats %+v", stats)
	}

	output := stdout.String()
	if strings.Contains(output, "Assertion Failures:") {
		t.Fatalf("did not expect assertion failures, got %q", output)
	}
	if strings.Count(output, "204 No Content") != 1 || strings.Count(output, "200 OK") != 2 {
		t.Fatalf("unexpected output %q", output)
	}
}

func TestRunAssertionFailureStopsLaterRequests(t *testing.T) {
	var calls []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		switch r.URL.Path {
		case "/first":
			_, _ = w.Write([]byte(`{"value":"wrong"}`))
		case "/second":
			_, _ = w.Write([]byte(`{"value":"second"}`))
		default:
			_, _ = w.Write([]byte(`{"value":"unknown"}`))
		}
	}))
	defer server.Close()

	tempDir := t.TempDir()
	httpPath := filepath.Join(tempDir, "assert-stop.http")
	if err := os.WriteFile(httpPath, []byte(strings.TrimSpace(`
###
# @name first
# @assert json.value == "expected"
GET {{base}}/first

###
# @name second
GET {{base}}/second
`)), 0o644); err != nil {
		t.Fatalf("write http file: %v", err)
	}

	var stdout bytes.Buffer
	stats, err := Run(context.Background(), &stdout, RunOptions{
		Path: httpPath,
		CLIOverrides: map[string]string{
			"base": server.URL,
		},
		Timeout: 5 * time.Second,
	})
	if err == nil {
		t.Fatalf("expected assertion failure")
	}
	if !strings.Contains(err.Error(), `expected json.value == "expected", got "wrong"`) {
		t.Fatalf("unexpected error %v", err)
	}
	if stats.Selected != 2 || stats.Executed != 1 || stats.Passed != 0 || stats.Failed != 1 {
		t.Fatalf("unexpected stats %+v", stats)
	}
	if len(calls) != 1 || calls[0] != "/first" {
		t.Fatalf("expected only first request to run, got %v", calls)
	}
	if strings.Contains(stdout.String(), "2. second") {
		t.Fatalf("expected second request output to be skipped, got %q", stdout.String())
	}
}

func assertRequest(t *testing.T, got capturedRequest, wantMethod, wantPath, wantQuery, wantBody string, wantHeaders map[string]string) {
	t.Helper()

	if got.Method != wantMethod {
		t.Fatalf("expected method %s, got %s", wantMethod, got.Method)
	}
	if got.Path != wantPath {
		t.Fatalf("expected path %s, got %s", wantPath, got.Path)
	}
	if got.Query != wantQuery {
		t.Fatalf("expected query %q, got %q", wantQuery, got.Query)
	}
	if got.Body != wantBody {
		t.Fatalf("expected body %q, got %q", wantBody, got.Body)
	}

	for key, wantValue := range wantHeaders {
		if gotValue := got.Header.Get(key); gotValue != wantValue {
			t.Fatalf("expected header %s=%q, got %q", key, wantValue, gotValue)
		}
	}
}
