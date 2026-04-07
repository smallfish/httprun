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
		return validateCommand(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func runCommand(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	vars := newVarsFlag()
	name := fs.String("name", "", "run only the named request")
	envName := fs.String("env", "", "read variables from http-client.env.json")
	jobs := fs.Int("jobs", 1, "number of .http files to run at the same time")
	timeout := fs.Duration("timeout", 30*time.Second, "default request timeout")
	verbose := fs.Bool("verbose", false, "print full request and response details")
	failHTTP := fs.Bool("fail-http", false, "return non-zero when HTTP status is >= 400")
	fs.Var(vars, "var", "override a variable (key=value), may be repeated")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printRunUsage(stdout)
			return 0
		}
		fmt.Fprintln(stderr, err)
		printRunUsage(stderr)
		return 2
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(stderr, "missing .http file path for run")
		printRunUsage(stderr)
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
		var captureErr app.CaptureError
		if errors.As(result.err, &httpErr) || errors.As(result.err, &assertionErr) || errors.As(result.err, &captureErr) {
			exitCode = 1
		} else {
			fmt.Fprintln(stderr, result.err)
			exitCode = 1
		}
	}
	return exitCode
}

func validateCommand(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	vars := newVarsFlag()
	name := fs.String("name", "", "check only the named request")
	envName := fs.String("env", "", "read variables from http-client.env.json")
	jobs := fs.Int("jobs", 1, "number of .http files to check at the same time")
	fs.Var(vars, "var", "override a variable (key=value), may be repeated")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printValidateUsage(stdout)
			return 0
		}
		fmt.Fprintln(stderr, err)
		printValidateUsage(stderr)
		return 2
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(stderr, "missing .http file path for validate")
		printValidateUsage(stderr)
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
	fmt.Fprintln(w, "NAME")
	fmt.Fprintln(w, "  httprun - command-line tool for running .http files")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "SYNOPSIS")
	fmt.Fprintln(w, "  httprun run [flags] <file.http> [more.http ...]")
	fmt.Fprintln(w, "  httprun validate [flags] <file.http> [more.http ...]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "COMMANDS")
	fmt.Fprintln(w, "  run                 Send requests from one or more .http files")
	fmt.Fprintln(w, "  validate            Check .http files without sending requests")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "COMMON FLAGS")
	fmt.Fprintln(w, "  --name <request>    Select a named request")
	fmt.Fprintln(w, "  --env <env>         Read variables from http-client.env.json files")
	fmt.Fprintln(w, "  --var key=value     Override a variable, may be repeated")
	fmt.Fprintln(w, "  --jobs <n>          Number of files to process at the same time")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "RUN-ONLY FLAGS")
	fmt.Fprintln(w, "  --timeout 30s       Default request timeout")
	fmt.Fprintln(w, "  --verbose           Print full request and response details")
	fmt.Fprintln(w, "  --fail-http         Return non-zero when HTTP status is >= 400")
}

func printRunUsage(w io.Writer) {
	fmt.Fprintln(w, "NAME")
	fmt.Fprintln(w, "  httprun run - send requests from .http files")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "SYNOPSIS")
	fmt.Fprintln(w, "  httprun run [flags] <file.http> [more.http ...]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "FLAGS")
	fmt.Fprintln(w, "  --name <request>    Run only the named request")
	fmt.Fprintln(w, "  --env <env>         Read variables from http-client.env.json files")
	fmt.Fprintln(w, "  --var key=value     Override a variable, may be repeated")
	fmt.Fprintln(w, "  --jobs <n>          Number of .http files to run at the same time")
	fmt.Fprintln(w, "  --timeout 30s       Default request timeout")
	fmt.Fprintln(w, "  --verbose           Print full request and response details")
	fmt.Fprintln(w, "  --fail-http         Return non-zero when HTTP status is >= 400")
}

func printValidateUsage(w io.Writer) {
	fmt.Fprintln(w, "NAME")
	fmt.Fprintln(w, "  httprun validate - check .http files without sending requests")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "SYNOPSIS")
	fmt.Fprintln(w, "  httprun validate [flags] <file.http> [more.http ...]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "FLAGS")
	fmt.Fprintln(w, "  --name <request>    Check only the named request")
	fmt.Fprintln(w, "  --env <env>         Read variables from http-client.env.json files")
	fmt.Fprintln(w, "  --var key=value     Override a variable, may be repeated")
	fmt.Fprintln(w, "  --jobs <n>          Number of .http files to check at the same time")
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
