package capture

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"

	"github.com/smallfish/httprun/internal/ast"
	"github.com/smallfish/httprun/internal/jsonpath"
)

func Apply(response *http.Response, body []byte, captures []ast.Capture) (map[string]string, []string) {
	if len(captures) == 0 {
		return nil, nil
	}

	values := make(map[string]string, len(captures))
	var (
		failures []string
		decoded  any
		jsonErr  error
		jsonRead bool
	)

	for _, capture := range captures {
		value, err := extractValue(response, body, capture, &decoded, &jsonErr, &jsonRead)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		values[capture.Name] = value
	}

	if len(failures) > 0 {
		return nil, failures
	}
	return values, nil
}

func extractValue(response *http.Response, body []byte, capture ast.Capture, decoded *any, jsonErr *error, jsonRead *bool) (string, error) {
	switch capture.Subject {
	case ast.CaptureSubjectStatus:
		if response == nil {
			return "", lineError(capture, "response status is unavailable")
		}
		return strconv.Itoa(response.StatusCode), nil
	case ast.CaptureSubjectBody:
		return string(body), nil
	case ast.CaptureSubjectHeader:
		if response == nil {
			return "", lineError(capture, "response headers are unavailable")
		}
		name := textproto.CanonicalMIMEHeaderKey(capture.Path)
		values := response.Header.Values(name)
		if len(values) == 0 {
			return "", lineError(capture, "header %q not found", name)
		}
		return strings.Join(values, ", "), nil
	case ast.CaptureSubjectJSON:
		if !*jsonRead {
			*jsonRead = true
			*jsonErr = json.Unmarshal(body, decoded)
		}
		if *jsonErr != nil {
			return "", lineError(capture, "expected JSON response body for %q", formatSource(capture))
		}
		value, found, err := jsonpath.Lookup(*decoded, capture.Path)
		if err != nil {
			return "", lineError(capture, "%v", err)
		}
		if !found {
			return "", lineError(capture, "%q not found", formatSource(capture))
		}
		return formatValue(value), nil
	default:
		return "", lineError(capture, "unsupported capture source %q", capture.Subject)
	}
}

func formatValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return "null"
	case string:
		return typed
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprintf("%v", typed)
		}
		return string(data)
	}
}

func formatSource(capture ast.Capture) string {
	if capture.Path == "" {
		return string(capture.Subject)
	}
	return fmt.Sprintf("%s.%s", capture.Subject, capture.Path)
}

func lineError(capture ast.Capture, format string, args ...any) error {
	return fmt.Errorf("line %d: capture %q: %s", capture.Pos.Line, capture.Name, fmt.Sprintf(format, args...))
}
