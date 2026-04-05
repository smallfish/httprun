package output

import (
	"fmt"
	"io"
	"net/http"

	"github.com/smallfish/httprun/internal/executor"
)

func WriteResult(w io.Writer, result executor.Result, verbose bool) error {
	if result.Request.Name != "" {
		if _, err := fmt.Fprintf(w, "==> %s\n", result.Request.Name); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "%s %s\n", result.Request.Method, result.Request.URL); err != nil {
		return err
	}

	if verbose {
		if err := writeHeaders(w, result.Request.Headers); err != nil {
			return err
		}
		if len(result.Request.Body) > 0 {
			if _, err := fmt.Fprintf(w, "\n%s\n", result.Request.Body); err != nil {
				return err
			}
		}
	}

	if _, err := fmt.Fprintf(w, "<== %s\n", result.Response.Status); err != nil {
		return err
	}

	if verbose {
		if err := writeHeaders(w, result.Response.Header); err != nil {
			return err
		}
	}

	if len(result.Body) > 0 {
		if _, err := fmt.Fprintf(w, "\n%s\n", result.Body); err != nil {
			return err
		}
	}

	_, err := fmt.Fprintln(w)
	return err
}

func writeHeaders(w io.Writer, headers http.Header) error {
	for key, values := range headers {
		for _, value := range values {
			if _, err := fmt.Fprintf(w, "%s: %s\n", key, value); err != nil {
				return err
			}
		}
	}
	return nil
}
