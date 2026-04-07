package validate

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/smallfish/httprun/internal/ast"
	"github.com/smallfish/httprun/internal/resolver"
)

func Document(doc ast.Document, requestName string, variables map[string]string) error {
	var issues []string

	if len(doc.Requests) == 0 {
		issues = append(issues, "no requests found")
	}

	seenNames := make(map[string]ast.Position)
	for _, request := range doc.Requests {
		if request.Name == "" {
			continue
		}
		if previous, exists := seenNames[request.Name]; exists {
			issues = append(issues, fmt.Sprintf("line %d: duplicate request name %q, first declared at line %d", request.Pos.Line, request.Name, previous.Line))
			continue
		}
		seenNames[request.Name] = request.Pos
	}

	selected, err := selectRequests(doc.Requests, requestName)
	if err != nil {
		issues = append(issues, err.Error())
	}

	baseDir := ""
	if doc.Path != "" {
		baseDir = filepath.Dir(doc.Path)
	}

	runtimeVars := cloneVariables(variables)
	for _, request := range selected {
		if _, err := resolver.ResolveRequest(request, runtimeVars, resolver.ResolveOptions{BaseDir: baseDir}); err != nil {
			issues = append(issues, err.Error())
		}
		for _, capture := range request.Captures {
			runtimeVars[capture.Name] = "__captured__"
		}
	}

	if len(issues) > 0 {
		return errors.New(strings.Join(issues, "\n"))
	}
	return nil
}

func cloneVariables(input map[string]string) map[string]string {
	cloned := make(map[string]string, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func SelectRequests(requests []ast.RequestBlock, requestName string) ([]ast.RequestBlock, error) {
	return selectRequests(requests, requestName)
}

func selectRequests(requests []ast.RequestBlock, requestName string) ([]ast.RequestBlock, error) {
	if requestName == "" {
		return requests, nil
	}

	for _, request := range requests {
		if request.Name == requestName {
			return []ast.RequestBlock{request}, nil
		}
	}

	return nil, fmt.Errorf("request %q not found", requestName)
}
