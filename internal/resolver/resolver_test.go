package resolver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smallfish/httprun/internal/ast"
)

func TestMergeVariablesPriority(t *testing.T) {
	fileVars := []ast.VariableDecl{{Name: "host", Value: "file"}}
	publicVars := map[string]string{"host": "public"}
	secretVars := map[string]string{"host": "secret"}
	cliVars := map[string]string{"host": "cli"}

	merged := MergeVariables(fileVars, publicVars, secretVars, cliVars)
	if got := merged["host"]; got != "cli" {
		t.Fatalf("expected cli override, got %q", got)
	}
}

func TestMergeVariablesPublicOverridesPrivate(t *testing.T) {
	merged := MergeVariables(nil, map[string]string{"token": "public"}, map[string]string{"token": "private", "secret": "yes"}, nil)
	if got := merged["token"]; got != "public" {
		t.Fatalf("expected public env to override private env, got %q", got)
	}
	if got := merged["secret"]; got != "yes" {
		t.Fatalf("expected private-only variable to be preserved, got %q", got)
	}
}

func TestResolveBuiltins(t *testing.T) {
	request := ast.RequestBlock{
		Method: "GET",
		URL:    "https://example.com/{{$uuid}}/{{$timestamp}}",
	}

	resolved, err := ResolveRequest(request, map[string]string{}, ResolveOptions{})
	if err != nil {
		t.Fatalf("ResolveRequest() error = %v", err)
	}

	if !strings.HasPrefix(resolved.URL, "https://example.com/") {
		t.Fatalf("unexpected url %q", resolved.URL)
	}
}

func TestResolveBodyFromFile(t *testing.T) {
	tempDir := t.TempDir()
	bodyPath := filepath.Join(tempDir, "payload.json")
	if err := os.WriteFile(bodyPath, []byte(`{"token":"{{token}}"}`), 0o644); err != nil {
		t.Fatalf("write payload: %v", err)
	}

	request := ast.RequestBlock{
		Method:   "POST",
		URL:      "https://example.com",
		BodyFile: "payload.json",
		BodyPos:  ast.Position{Line: 4, Column: 1},
	}

	resolved, err := ResolveRequest(request, map[string]string{"token": "abc"}, ResolveOptions{BaseDir: tempDir})
	if err != nil {
		t.Fatalf("ResolveRequest() error = %v", err)
	}
	if got := string(resolved.Body); got != `{"token":"abc"}` {
		t.Fatalf("unexpected body %q", got)
	}
}
