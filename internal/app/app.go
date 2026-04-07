package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/smallfish/httprun/internal/assert"
	"github.com/smallfish/httprun/internal/ast"
	"github.com/smallfish/httprun/internal/envfile"
	"github.com/smallfish/httprun/internal/executor"
	"github.com/smallfish/httprun/internal/output"
	"github.com/smallfish/httprun/internal/parser"
	"github.com/smallfish/httprun/internal/resolver"
	"github.com/smallfish/httprun/internal/validate"
)

type RunOptions struct {
	Path            string
	RequestName     string
	EnvironmentName string
	CLIOverrides    map[string]string
	Timeout         time.Duration
	Verbose         bool
	FailOnHTTPError bool
}

type ValidateOptions struct {
	Path            string
	RequestName     string
	EnvironmentName string
	CLIOverrides    map[string]string
}

type RunStats struct {
	Selected int
	Executed int
	Passed   int
	Failed   int
}

type HTTPStatusError struct {
	RequestName string
	StatusCode  int
	Status      string
}

func (e HTTPStatusError) Error() string {
	if e.RequestName != "" {
		return fmt.Sprintf("%s failed with %s", e.RequestName, e.Status)
	}
	return fmt.Sprintf("request failed with %s", e.Status)
}

type AssertionError struct {
	RequestName string
	Failures    []string
}

func (e AssertionError) Error() string {
	if len(e.Failures) == 0 {
		if e.RequestName != "" {
			return fmt.Sprintf("%s failed assertions", e.RequestName)
		}
		return "request failed assertions"
	}

	message := strings.Join(e.Failures, "; ")
	if e.RequestName != "" {
		return fmt.Sprintf("%s failed assertions: %s", e.RequestName, message)
	}
	return fmt.Sprintf("request failed assertions: %s", message)
}

func Run(ctx context.Context, stdout io.Writer, options RunOptions) (RunStats, error) {
	doc, variables, err := load(options.Path, options.EnvironmentName, options.CLIOverrides)
	if err != nil {
		return RunStats{}, err
	}

	if err := validate.Document(doc, options.RequestName, variables); err != nil {
		return RunStats{}, qualifyPath(doc.Path, err)
	}

	requests, err := validate.SelectRequests(doc.Requests, options.RequestName)
	if err != nil {
		return RunStats{}, qualifyPath(doc.Path, err)
	}
	stats := RunStats{Selected: len(requests)}

	execConfig := executor.Config{Timeout: options.Timeout}
	session, err := executor.NewSession(execConfig)
	if err != nil {
		return stats, err
	}
	resolveOptions := resolver.ResolveOptions{BaseDir: filepath.Dir(doc.Path)}
	for idx, request := range requests {
		resolved, err := resolver.ResolveRequest(request, variables, resolveOptions)
		if err != nil {
			return stats, qualifyPath(doc.Path, err)
		}

		result, err := session.Execute(ctx, resolved)
		if err != nil {
			stats.Executed++
			stats.Failed++
			return stats, qualifyPath(doc.Path, fmt.Errorf("line %d: %w", request.Pos.Line, err))
		}
		stats.Executed++
		assertionFailures := assert.Check(result.Response, result.Body, request.Assertions)

		if result.Response.StatusCode >= http.StatusBadRequest || len(assertionFailures) > 0 {
			stats.Failed++
		} else {
			stats.Passed++
		}

		if err := output.WriteResult(stdout, idx+1, result, options.Verbose, assertionFailures); err != nil {
			return stats, err
		}

		if len(assertionFailures) > 0 {
			return stats, AssertionError{
				RequestName: request.Name,
				Failures:    assertionFailures,
			}
		}

		if options.FailOnHTTPError && result.Response.StatusCode >= http.StatusBadRequest {
			return stats, HTTPStatusError{
				RequestName: request.Name,
				StatusCode:  result.Response.StatusCode,
				Status:      result.Response.Status,
			}
		}
	}

	return stats, nil
}

func Validate(options ValidateOptions) error {
	doc, variables, err := load(options.Path, options.EnvironmentName, options.CLIOverrides)
	if err != nil {
		return err
	}
	if err := validate.Document(doc, options.RequestName, variables); err != nil {
		return qualifyPath(doc.Path, err)
	}
	return nil
}

func load(path, envName string, overrides map[string]string) (ast.Document, map[string]string, error) {
	doc, err := parser.ParseFile(path)
	if err != nil {
		return ast.Document{}, nil, err
	}

	absPath, err := filepath.Abs(path)
	if err == nil {
		doc.Path = absPath
	}

	loadedEnv, err := envfile.LoadForRequestFile(path, envName)
	if err != nil {
		return ast.Document{}, nil, qualifyPath(doc.Path, err)
	}

	variables := resolver.MergeVariables(doc.Variables, loadedEnv.Public, loadedEnv.Secret, overrides)
	return doc, variables, nil
}

func qualifyPath(path string, err error) error {
	return fmt.Errorf("%s: %w", path, err)
}
