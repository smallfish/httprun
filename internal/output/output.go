package output

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/smallfish/httprun/internal/executor"
)

func WriteResult(w io.Writer, index int, result executor.Result, verbose bool) error {
	title := result.Request.Name
	if title == "" {
		title = fmt.Sprintf("%s %s", result.Request.Method, compactURL(result.Request.URL))
	}
	if _, err := fmt.Fprintf(w, "%d. %s\n", index, title); err != nil {
		return err
	}

	target := compactURL(result.Request.URL)
	if verbose {
		target = result.Request.URL
	}
	if result.Request.Name != "" {
		if _, err := fmt.Fprintf(w, "   %s %s\n", result.Request.Method, target); err != nil {
			return err
		}
	} else if verbose {
		if _, err := fmt.Fprintf(w, "   %s %s\n", result.Request.Method, target); err != nil {
			return err
		}
	}

	if verbose {
		if err := writeHeaderSection(w, "Request Headers", result.Request.Headers); err != nil {
			return err
		}
		if len(result.Request.Body) > 0 {
			if err := writeBodySection(w, "Request Body", string(result.Request.Body)); err != nil {
				return err
			}
		}
	}

	if _, err := fmt.Fprintf(w, "   %s  %s  %s\n", result.Response.Status, formatDuration(result.Duration), formatBytes(len(result.Body))); err != nil {
		return err
	}

	if verbose {
		if err := writeHeaderSection(w, "Response Headers", result.Response.Header); err != nil {
			return err
		}
		if len(result.Body) > 0 {
			if err := writeBodySection(w, "Response Body", string(result.Body)); err != nil {
				return err
			}
		}
	} else if failedResponse(result.Response) {
		body, truncated := formatFailedBody(result.Response.Header, result.Body)
		if body != "" {
			label := "Response Body"
			if truncated {
				label = "Response Body (truncated)"
			}
			if err := writeBodySection(w, label, body); err != nil {
				return err
			}
		}
	}

	_, err := fmt.Fprintln(w)
	return err
}

func compactURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return raw
	}
	requestURI := parsed.RequestURI()
	if requestURI == "" {
		return "/"
	}
	return requestURI
}

func writeHeaderSection(w io.Writer, label string, headers http.Header) error {
	if len(headers) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w, "   %s:\n", label); err != nil {
		return err
	}

	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		values := headers[key]
		for _, value := range values {
			if _, err := fmt.Fprintf(w, "     %s: %s\n", key, value); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeBodySection(w io.Writer, label, body string) error {
	if body == "" {
		return nil
	}
	if _, err := fmt.Fprintf(w, "   %s:\n", label); err != nil {
		return err
	}
	for _, line := range strings.Split(body, "\n") {
		if _, err := fmt.Fprintf(w, "     %s\n", line); err != nil {
			return err
		}
	}
	return nil
}

func formatDuration(duration time.Duration) string {
	if duration < time.Millisecond {
		return "<1ms"
	}
	return duration.Round(time.Millisecond).String()
}

func formatBytes(size int) string {
	const (
		kb = 1024
		mb = 1024 * kb
	)

	switch {
	case size >= mb:
		return fmt.Sprintf("%.1f MB", float64(size)/mb)
	case size >= kb:
		return fmt.Sprintf("%.1f KB", float64(size)/kb)
	default:
		return fmt.Sprintf("%d B", size)
	}
}

func failedResponse(response *http.Response) bool {
	return response != nil && response.StatusCode >= http.StatusBadRequest
}

func formatFailedBody(headers http.Header, body []byte) (string, bool) {
	if len(body) == 0 || !isTextResponse(headers, body) {
		return "", false
	}

	const maxBodyBytes = 4 * 1024
	if len(body) <= maxBodyBytes {
		return string(body), false
	}

	truncated := body[:maxBodyBytes]
	for len(truncated) > 0 && !utf8.Valid(truncated) {
		truncated = truncated[:len(truncated)-1]
	}
	return string(truncated), true
}

func isTextResponse(headers http.Header, body []byte) bool {
	contentType := headers.Get("Content-Type")
	if contentType != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err == nil {
			if strings.HasPrefix(mediaType, "text/") {
				return true
			}
			switch mediaType {
			case "application/json", "application/problem+json", "application/xml", "application/yaml", "application/x-yaml":
				return true
			}
		}
	}

	return utf8.Valid(body)
}
