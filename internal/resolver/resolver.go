package resolver

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/smallfish/httprun/internal/ast"
)

var templatePattern = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)

type ResolvedRequest struct {
	Name              string
	Method            string
	URL               string
	Headers           http.Header
	Body              []byte
	Timeout           *time.Duration
	ConnectionTimeout *time.Duration
	NoRedirect        bool
	NoCookieJar       bool
	Pos               ast.Position
}

type ResolveOptions struct {
	BaseDir string
}

func MergeVariables(fileVars []ast.VariableDecl, publicVars, secretVars, cliVars map[string]string) map[string]string {
	merged := make(map[string]string, len(fileVars)+len(publicVars)+len(secretVars)+len(cliVars))

	for _, variable := range fileVars {
		merged[variable.Name] = variable.Value
	}
	for key, value := range secretVars {
		merged[key] = value
	}
	for key, value := range publicVars {
		merged[key] = value
	}
	for key, value := range cliVars {
		merged[key] = value
	}

	return merged
}

func ResolveRequest(request ast.RequestBlock, variables map[string]string, options ResolveOptions) (ResolvedRequest, error) {
	url, err := resolveString(request.URL, variables)
	if err != nil {
		return ResolvedRequest{}, fmt.Errorf("line %d: url: %w", request.Pos.Line, err)
	}

	headers := make(http.Header, len(request.Headers))
	for _, header := range request.Headers {
		value, err := resolveString(header.Value, variables)
		if err != nil {
			return ResolvedRequest{}, fmt.Errorf("line %d: header %q: %w", header.Pos.Line, header.Name, err)
		}
		headers.Add(header.Name, value)
	}

	body, err := resolveBody(request, variables, options)
	if err != nil {
		line := request.Pos.Line
		if request.BodyFile != "" {
			line = request.BodyPos.Line
		}
		return ResolvedRequest{}, fmt.Errorf("line %d: body: %w", line, err)
	}

	return ResolvedRequest{
		Name:              request.Name,
		Method:            request.Method,
		URL:               url,
		Headers:           headers,
		Body:              []byte(body),
		Timeout:           request.Timeout,
		ConnectionTimeout: request.ConnectionTimeout,
		NoRedirect:        request.NoRedirect,
		NoCookieJar:       request.NoCookieJar,
		Pos:               request.Pos,
	}, nil
}

func resolveBody(request ast.RequestBlock, variables map[string]string, options ResolveOptions) (string, error) {
	if request.BodyFile == "" {
		return resolveString(request.Body, variables)
	}

	bodyPath, err := resolveString(request.BodyFile, variables)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(bodyPath) {
		bodyPath = filepath.Join(options.BaseDir, bodyPath)
	}

	data, err := os.ReadFile(bodyPath)
	if err != nil {
		return "", err
	}

	return resolveString(string(data), variables)
}

func resolveString(input string, variables map[string]string) (string, error) {
	if input == "" {
		return "", nil
	}

	var unresolved []string
	resolved := templatePattern.ReplaceAllStringFunc(input, func(match string) string {
		groups := templatePattern.FindStringSubmatch(match)
		if len(groups) != 2 {
			return match
		}

		name := strings.TrimSpace(groups[1])
		if builtin, ok := builtinVariable(name); ok {
			return builtin
		}
		if value, ok := variables[name]; ok {
			return value
		}

		unresolved = append(unresolved, name)
		return match
	})

	if len(unresolved) > 0 {
		return "", fmt.Errorf("undefined variable %q", unresolved[0])
	}

	return resolved, nil
}

func builtinVariable(name string) (string, bool) {
	switch name {
	case "$timestamp":
		return fmt.Sprintf("%d", time.Now().Unix()), true
	case "$uuid":
		return randomUUID(), true
	default:
		return "", false
	}
}

func randomUUID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		now := time.Now().UnixNano()
		return fmt.Sprintf("fallback-%d", now)
	}

	buffer[6] = (buffer[6] & 0x0f) | 0x40
	buffer[8] = (buffer[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		buffer[0:4],
		buffer[4:6],
		buffer[6:8],
		buffer[8:10],
		buffer[10:16],
	)
}
