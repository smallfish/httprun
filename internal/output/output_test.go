package output

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/smallfish/httprun/internal/executor"
	"github.com/smallfish/httprun/internal/resolver"
)

func TestWriteResultCompactOutput(t *testing.T) {
	result := executor.Result{
		Request: resolver.ResolvedRequest{
			Name:   "list-tokens",
			Method: http.MethodPost,
			URL:    "http://127.0.0.1:28080/v2/apitoken/tokens?limit=10",
		},
		Response: &http.Response{Status: "200 OK"},
		Body:     []byte(`{"ok":true}`),
		Duration: 42 * time.Millisecond,
	}

	var buffer bytes.Buffer
	if err := WriteResult(&buffer, 1, result, false, nil, nil); err != nil {
		t.Fatalf("WriteResult() error = %v", err)
	}

	output := buffer.String()
	if !strings.Contains(output, "1. list-tokens") {
		t.Fatalf("expected numbered title, got %q", output)
	}
	if !strings.Contains(output, "POST /v2/apitoken/tokens?limit=10") {
		t.Fatalf("expected compact request target, got %q", output)
	}
	if !strings.Contains(output, "200 OK  42ms  11 B") {
		t.Fatalf("expected status line with timing and size, got %q", output)
	}
	if strings.Contains(output, `{"ok":true}`) {
		t.Fatalf("did not expect response body in compact mode, got %q", output)
	}
}

func TestWriteResultVerboseOutputIsStableAndDetailed(t *testing.T) {
	result := executor.Result{
		Request: resolver.ResolvedRequest{
			Name:   "create-token",
			Method: http.MethodPost,
			URL:    "http://127.0.0.1:28080/v2/apitoken/create",
			Headers: http.Header{
				"X-Account-Id": []string{"123"},
				"Content-Type": []string{"application/json"},
			},
			Body: []byte("{\n  \"name\": \"demo\"\n}"),
		},
		Response: &http.Response{
			Status: "201 Created",
			Header: http.Header{
				"X-Request-Id": []string{"abc"},
				"Content-Type": []string{"application/json"},
			},
		},
		Body:     []byte("{\n  \"id\": 1\n}"),
		Duration: 125 * time.Millisecond,
	}

	var buffer bytes.Buffer
	if err := WriteResult(&buffer, 2, result, true, nil, nil); err != nil {
		t.Fatalf("WriteResult() error = %v", err)
	}

	output := buffer.String()
	if !strings.Contains(output, "2. create-token") {
		t.Fatalf("expected numbered title, got %q", output)
	}
	if !strings.Contains(output, "POST http://127.0.0.1:28080/v2/apitoken/create") {
		t.Fatalf("expected full url in verbose mode, got %q", output)
	}
	if !strings.Contains(output, "Request Headers:\n     Content-Type: application/json\n     X-Account-Id: 123") {
		t.Fatalf("expected sorted request headers, got %q", output)
	}
	if !strings.Contains(output, "Response Headers:\n     Content-Type: application/json\n     X-Request-Id: abc") {
		t.Fatalf("expected sorted response headers, got %q", output)
	}
	if !strings.Contains(output, "Request Body:\n     {\n       \"name\": \"demo\"\n     }") {
		t.Fatalf("expected request body section, got %q", output)
	}
	if !strings.Contains(output, "Response Body:\n     {\n       \"id\": 1\n     }") {
		t.Fatalf("expected response body section, got %q", output)
	}
}

func TestWriteResultCompactOutputIncludesFailedTextBody(t *testing.T) {
	result := executor.Result{
		Request: resolver.ResolvedRequest{
			Method: http.MethodPost,
			URL:    "http://127.0.0.1:28080/v2/apitoken/create",
		},
		Response: &http.Response{
			Status:     "400 Bad Request",
			StatusCode: http.StatusBadRequest,
			Header: http.Header{
				"Content-Type": []string{"application/json"},
			},
		},
		Body:     []byte(`{"code":"INVALID","message":"bad input"}`),
		Duration: 900 * time.Microsecond,
	}

	var buffer bytes.Buffer
	if err := WriteResult(&buffer, 2, result, false, nil, nil); err != nil {
		t.Fatalf("WriteResult() error = %v", err)
	}

	output := buffer.String()
	if !strings.Contains(output, "400 Bad Request  <1ms  40 B") {
		t.Fatalf("expected failed summary line, got %q", output)
	}
	if !strings.Contains(output, "Response Body:\n     {\"code\":\"INVALID\",\"message\":\"bad input\"}") {
		t.Fatalf("expected failed response body, got %q", output)
	}
}

func TestWriteResultCompactOutputSkipsFailedBinaryBody(t *testing.T) {
	result := executor.Result{
		Request: resolver.ResolvedRequest{
			Method: http.MethodGet,
			URL:    "http://127.0.0.1:28080/download",
		},
		Response: &http.Response{
			Status:     "500 Internal Server Error",
			StatusCode: http.StatusInternalServerError,
			Header: http.Header{
				"Content-Type": []string{"application/octet-stream"},
			},
		},
		Body:     []byte{0xff, 0xfe, 0xfd, 0xfc},
		Duration: 10 * time.Millisecond,
	}

	var buffer bytes.Buffer
	if err := WriteResult(&buffer, 1, result, false, nil, nil); err != nil {
		t.Fatalf("WriteResult() error = %v", err)
	}

	output := buffer.String()
	if strings.Contains(output, "Response Body") {
		t.Fatalf("did not expect binary response body, got %q", output)
	}
}

func TestWriteResultCompactOutputIncludesAssertionFailuresAndBody(t *testing.T) {
	result := executor.Result{
		Request: resolver.ResolvedRequest{
			Name:   "check-json",
			Method: http.MethodGet,
			URL:    "http://127.0.0.1:28080/data",
		},
		Response: &http.Response{
			Status:     "200 OK",
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"application/json"},
			},
		},
		Body:     []byte(`{"name":"demo"}`),
		Duration: 8 * time.Millisecond,
	}

	var buffer bytes.Buffer
	if err := WriteResult(&buffer, 1, result, false, []string{`line 4: expected json.name == "other", got "demo"`}, nil); err != nil {
		t.Fatalf("WriteResult() error = %v", err)
	}

	output := buffer.String()
	if !strings.Contains(output, "Assertion Failures:\n     - line 4: expected json.name == \"other\", got \"demo\"") {
		t.Fatalf("expected assertion failure section, got %q", output)
	}
	if !strings.Contains(output, "Response Body:\n     {\"name\":\"demo\"}") {
		t.Fatalf("expected response body for assertion failure, got %q", output)
	}
}

func TestWriteResultCompactOutputIncludesCaptureFailuresAndBody(t *testing.T) {
	result := executor.Result{
		Request: resolver.ResolvedRequest{
			Name:   "create-resource",
			Method: http.MethodPost,
			URL:    "http://127.0.0.1:28080/resources",
		},
		Response: &http.Response{
			Status:     "201 Created",
			StatusCode: http.StatusCreated,
			Header: http.Header{
				"Content-Type": []string{"application/json"},
			},
		},
		Body:     []byte(`{"data":{}}`),
		Duration: 8 * time.Millisecond,
	}

	var buffer bytes.Buffer
	if err := WriteResult(&buffer, 1, result, false, nil, []string{`line 3: capture "test_id": "json.data.id" not found`}); err != nil {
		t.Fatalf("WriteResult() error = %v", err)
	}

	output := buffer.String()
	if !strings.Contains(output, "Capture Failures:\n     - line 3: capture \"test_id\": \"json.data.id\" not found") {
		t.Fatalf("expected capture failure section, got %q", output)
	}
	if !strings.Contains(output, "Response Body:\n     {\"data\":{}}") {
		t.Fatalf("expected response body for capture failure, got %q", output)
	}
}
