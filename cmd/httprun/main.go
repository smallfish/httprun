package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/smallfish/httprun/internal/app"
)

func main() {
	os.Exit(realMain(os.Args[1:], os.Stdout, os.Stderr))
}

func realMain(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	switch args[0] {
	case "run":
		return runCommand(args[1:], stdout, stderr)
	case "validate":
		return validateCommand(args[1:], stderr)
	case "-h", "--help", "help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func runCommand(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)

	vars := newVarsFlag()
	name := fs.String("name", "", "execute a named request")
	envName := fs.String("env", "", "environment name from http-client.env.json")
	jobs := fs.Int("jobs", 1, "number of files to process concurrently")
	timeout := fs.Duration("timeout", 30*time.Second, "request timeout")
	verbose := fs.Bool("verbose", false, "print expanded request and response details")
	failHTTP := fs.Bool("fail-http", false, "return non-zero on HTTP status >= 400")
	fs.Var(vars, "var", "override variable (key=value), may be repeated")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(stderr, "run requires at least one .http file path")
		return 2
	}
	if *jobs <= 0 {
		fmt.Fprintln(stderr, "--jobs must be greater than 0")
		return 2
	}

	results := executeInParallel(fs.Args(), *jobs, func(path string) commandResult {
		var buffer bytes.Buffer
		stats, err := app.Run(context.Background(), &buffer, app.RunOptions{
			Path:            path,
			RequestName:     *name,
			EnvironmentName: *envName,
			CLIOverrides:    vars.Values(),
			Timeout:         *timeout,
			Verbose:         *verbose,
			FailOnHTTPError: *failHTTP,
		})
		return commandResult{
			path:   path,
			output: buffer.String(),
			stats:  stats,
			err:    err,
		}
	})

	exitCode := 0
	multiFile := len(results) > 1
	for _, result := range results {
		if multiFile && (result.output != "" || shouldPrintSummary(result)) {
			fmt.Fprintf(stdout, "== %s ==\n\n", result.path)
		}
		if result.output != "" {
			_, _ = io.WriteString(stdout, result.output)
		}
		if summary := formatRunSummary(result); summary != "" {
			fmt.Fprintln(stdout, summary)
			fmt.Fprintln(stdout)
		}
		if result.err == nil {
			continue
		}

		var httpErr app.HTTPStatusError
		var assertionErr app.AssertionError
		if errors.As(result.err, &httpErr) || errors.As(result.err, &assertionErr) {
			exitCode = 1
		} else {
			fmt.Fprintln(stderr, result.err)
			exitCode = 1
		}
	}
	return exitCode
}

func validateCommand(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(stderr)

	vars := newVarsFlag()
	name := fs.String("name", "", "validate a named request only")
	envName := fs.String("env", "", "environment name from http-client.env.json")
	jobs := fs.Int("jobs", 1, "number of files to process concurrently")
	fs.Var(vars, "var", "override variable (key=value), may be repeated")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(stderr, "validate requires at least one .http file path")
		return 2
	}
	if *jobs <= 0 {
		fmt.Fprintln(stderr, "--jobs must be greater than 0")
		return 2
	}

	results := executeInParallel(fs.Args(), *jobs, func(path string) commandResult {
		err := app.Validate(app.ValidateOptions{
			Path:            path,
			RequestName:     *name,
			EnvironmentName: *envName,
			CLIOverrides:    vars.Values(),
		})
		return commandResult{
			path: path,
			err:  err,
		}
	})

	exitCode := 0
	for _, result := range results {
		if result.err != nil {
			fmt.Fprintln(stderr, result.err)
			exitCode = 1
			continue
		}
		fmt.Fprintf(stderr, "%s: OK\n", result.path)
	}
	return exitCode
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  httprun run [flags] <file.http> [more.http ...]")
	fmt.Fprintln(w, "  httprun validate [flags] <file.http> [more.http ...]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --name <request>    Execute or validate a named request")
	fmt.Fprintln(w, "  --env <env>         Load variables from http-client.env.json files")
	fmt.Fprintln(w, "  --var key=value     Override a variable, may be repeated")
	fmt.Fprintln(w, "  --jobs <n>          Process files concurrently")
	fmt.Fprintln(w, "  --timeout 30s       Request timeout for run")
	fmt.Fprintln(w, "  --verbose           Print expanded request and response details")
	fmt.Fprintln(w, "  --fail-http         Return non-zero on HTTP status >= 400")
}

type varsFlag struct {
	values map[string]string
}

func newVarsFlag() *varsFlag {
	return &varsFlag{values: make(map[string]string)}
}

func (v *varsFlag) String() string {
	return ""
}

func (v *varsFlag) Set(input string) error {
	if input == "" {
		return fmt.Errorf("variable override cannot be empty")
	}

	parts := splitKeyValue(input)
	if len(parts) != 2 || parts[0] == "" {
		return fmt.Errorf("invalid variable override %q, want key=value", input)
	}

	v.values[parts[0]] = parts[1]
	return nil
}

func (v *varsFlag) Values() map[string]string {
	cloned := make(map[string]string, len(v.values))
	for key, value := range v.values {
		cloned[key] = value
	}
	return cloned
}

func splitKeyValue(input string) []string {
	for i := 0; i < len(input); i++ {
		if input[i] == '=' {
			return []string{input[:i], input[i+1:]}
		}
	}
	return []string{input}
}

type commandResult struct {
	path   string
	output string
	stats  app.RunStats
	err    error
}

func shouldPrintSummary(result commandResult) bool {
	return result.stats.Selected > 0 && (result.stats.Executed > 0 || result.err == nil)
}

func formatRunSummary(result commandResult) string {
	if !shouldPrintSummary(result) {
		return ""
	}

	stats := result.stats
	skipped := stats.Selected - stats.Executed
	switch {
	case skipped == 0 && stats.Failed == 0:
		return fmt.Sprintf("Summary: %d requests, %d passed", stats.Selected, stats.Passed)
	case skipped == 0:
		return fmt.Sprintf("Summary: %d requests, %d passed, %d failed", stats.Selected, stats.Passed, stats.Failed)
	default:
		return fmt.Sprintf("Summary: %d/%d executed, %d passed, %d failed, %d skipped", stats.Executed, stats.Selected, stats.Passed, stats.Failed, skipped)
	}
}

func executeInParallel(paths []string, jobs int, fn func(path string) commandResult) []commandResult {
	results := make([]commandResult, len(paths))
	indexes := make(chan int)

	var wg sync.WaitGroup
	workerCount := jobs
	if workerCount > len(paths) {
		workerCount = len(paths)
	}

	for range workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range indexes {
				results[idx] = fn(paths[idx])
			}
		}()
	}

	for idx := range paths {
		indexes <- idx
	}
	close(indexes)
	wg.Wait()

	return results
}
