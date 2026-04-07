package assert

import (
	"net/http"
	"strings"
	"testing"

	"github.com/smallfish/httprun/internal/ast"
)

func TestCheckStatusOperators(t *testing.T) {
	response := &http.Response{StatusCode: http.StatusCreated}

	tests := []struct {
		name      string
		assertion ast.Assertion
		wantPass  bool
		wantText  string
	}{
		{name: "equal pass", assertion: ast.Assertion{Subject: ast.AssertSubjectStatus, Operator: ast.AssertOpEqual, Expected: "201", Pos: ast.Position{Line: 1}}, wantPass: true},
		{name: "equal fail", assertion: ast.Assertion{Subject: ast.AssertSubjectStatus, Operator: ast.AssertOpEqual, Expected: "200", Pos: ast.Position{Line: 2}}, wantText: "expected status == 200, got 201"},
		{name: "not equal pass", assertion: ast.Assertion{Subject: ast.AssertSubjectStatus, Operator: ast.AssertOpNotEqual, Expected: "200", Pos: ast.Position{Line: 3}}, wantPass: true},
		{name: "greater pass", assertion: ast.Assertion{Subject: ast.AssertSubjectStatus, Operator: ast.AssertOpGreater, Expected: "200", Pos: ast.Position{Line: 4}}, wantPass: true},
		{name: "greater equal pass", assertion: ast.Assertion{Subject: ast.AssertSubjectStatus, Operator: ast.AssertOpGreaterEqual, Expected: "201", Pos: ast.Position{Line: 5}}, wantPass: true},
		{name: "less pass", assertion: ast.Assertion{Subject: ast.AssertSubjectStatus, Operator: ast.AssertOpLess, Expected: "300", Pos: ast.Position{Line: 6}}, wantPass: true},
		{name: "less equal pass", assertion: ast.Assertion{Subject: ast.AssertSubjectStatus, Operator: ast.AssertOpLessEqual, Expected: "201", Pos: ast.Position{Line: 7}}, wantPass: true},
	}

	runAssertionCases(t, response, []byte(""), tests)
}

func TestCheckBodyOperators(t *testing.T) {
	tests := []struct {
		name      string
		body      []byte
		assertion ast.Assertion
		wantPass  bool
		wantText  string
	}{
		{name: "exists pass", body: []byte("hello"), assertion: ast.Assertion{Subject: ast.AssertSubjectBody, Operator: ast.AssertOpExists, Pos: ast.Position{Line: 1}}, wantPass: true},
		{name: "not exists pass", body: nil, assertion: ast.Assertion{Subject: ast.AssertSubjectBody, Operator: ast.AssertOpNotExists, Pos: ast.Position{Line: 2}}, wantPass: true},
		{name: "equal quoted pass", body: []byte("hello"), assertion: ast.Assertion{Subject: ast.AssertSubjectBody, Operator: ast.AssertOpEqual, Expected: `"hello"`, Pos: ast.Position{Line: 3}}, wantPass: true},
		{name: "equal raw pass", body: []byte("hello"), assertion: ast.Assertion{Subject: ast.AssertSubjectBody, Operator: ast.AssertOpEqual, Expected: "hello", Pos: ast.Position{Line: 4}}, wantPass: true},
		{name: "not equal pass", body: []byte("hello"), assertion: ast.Assertion{Subject: ast.AssertSubjectBody, Operator: ast.AssertOpNotEqual, Expected: "goodbye", Pos: ast.Position{Line: 5}}, wantPass: true},
		{name: "contains pass", body: []byte("hello world"), assertion: ast.Assertion{Subject: ast.AssertSubjectBody, Operator: ast.AssertOpContains, Expected: "world", Pos: ast.Position{Line: 6}}, wantPass: true},
		{name: "not contains pass", body: []byte("hello world"), assertion: ast.Assertion{Subject: ast.AssertSubjectBody, Operator: ast.AssertOpNotContains, Expected: "error", Pos: ast.Position{Line: 7}}, wantPass: true},
		{name: "contains fail", body: []byte("hello world"), assertion: ast.Assertion{Subject: ast.AssertSubjectBody, Operator: ast.AssertOpContains, Expected: "error", Pos: ast.Position{Line: 8}}, wantText: `expected body to contain "error"`},
		{name: "not exists fail", body: []byte("hello"), assertion: ast.Assertion{Subject: ast.AssertSubjectBody, Operator: ast.AssertOpNotExists, Pos: ast.Position{Line: 9}}, wantText: "expected body to not exist"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			failures := Check(&http.Response{StatusCode: http.StatusOK}, tt.body, []ast.Assertion{tt.assertion})
			assertFailures(t, failures, tt.wantPass, tt.wantText)
		})
	}
}

func TestCheckHeaderOperators(t *testing.T) {
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json; charset=utf-8"},
			"X-Trace-Id":   []string{"trace-123"},
		},
	}

	tests := []struct {
		name      string
		assertion ast.Assertion
		wantPass  bool
		wantText  string
	}{
		{name: "exists pass", assertion: ast.Assertion{Subject: ast.AssertSubjectHeader, Path: "X-Trace-Id", Operator: ast.AssertOpExists, Pos: ast.Position{Line: 1}}, wantPass: true},
		{name: "not exists pass", assertion: ast.Assertion{Subject: ast.AssertSubjectHeader, Path: "X-Missing", Operator: ast.AssertOpNotExists, Pos: ast.Position{Line: 2}}, wantPass: true},
		{name: "equal pass", assertion: ast.Assertion{Subject: ast.AssertSubjectHeader, Path: "X-Trace-Id", Operator: ast.AssertOpEqual, Expected: "trace-123", Pos: ast.Position{Line: 3}}, wantPass: true},
		{name: "not equal pass", assertion: ast.Assertion{Subject: ast.AssertSubjectHeader, Path: "X-Trace-Id", Operator: ast.AssertOpNotEqual, Expected: "other", Pos: ast.Position{Line: 4}}, wantPass: true},
		{name: "contains pass", assertion: ast.Assertion{Subject: ast.AssertSubjectHeader, Path: "Content-Type", Operator: ast.AssertOpContains, Expected: `"application/json"`, Pos: ast.Position{Line: 5}}, wantPass: true},
		{name: "not contains pass", assertion: ast.Assertion{Subject: ast.AssertSubjectHeader, Path: "Content-Type", Operator: ast.AssertOpNotContains, Expected: "text/plain", Pos: ast.Position{Line: 6}}, wantPass: true},
		{name: "equal fail", assertion: ast.Assertion{Subject: ast.AssertSubjectHeader, Path: "X-Trace-Id", Operator: ast.AssertOpEqual, Expected: "other", Pos: ast.Position{Line: 7}}, wantText: `expected header "X-Trace-Id" == "other"`},
		{name: "exists fail", assertion: ast.Assertion{Subject: ast.AssertSubjectHeader, Path: "X-Missing", Operator: ast.AssertOpExists, Pos: ast.Position{Line: 8}}, wantText: `expected header "X-Missing" to exist`},
	}

	runAssertionCases(t, response, nil, tests)
}

func TestCheckJSONOperators(t *testing.T) {
	response := &http.Response{StatusCode: http.StatusOK}
	body := []byte(`{"data":{"count":2,"name":"demo","enabled":true,"tags":["one","two"],"nullable":null}}`)

	tests := []struct {
		name      string
		assertion ast.Assertion
		wantPass  bool
		wantText  string
	}{
		{name: "exists pass", assertion: ast.Assertion{Subject: ast.AssertSubjectJSON, Path: "data.tags", Operator: ast.AssertOpExists, Pos: ast.Position{Line: 1}}, wantPass: true},
		{name: "not exists pass", assertion: ast.Assertion{Subject: ast.AssertSubjectJSON, Path: "data.missing", Operator: ast.AssertOpNotExists, Pos: ast.Position{Line: 2}}, wantPass: true},
		{name: "equal string pass", assertion: ast.Assertion{Subject: ast.AssertSubjectJSON, Path: "data.name", Operator: ast.AssertOpEqual, Expected: `"demo"`, Pos: ast.Position{Line: 3}}, wantPass: true},
		{name: "not equal pass", assertion: ast.Assertion{Subject: ast.AssertSubjectJSON, Path: "data.name", Operator: ast.AssertOpNotEqual, Expected: `"other"`, Pos: ast.Position{Line: 4}}, wantPass: true},
		{name: "greater pass", assertion: ast.Assertion{Subject: ast.AssertSubjectJSON, Path: "data.count", Operator: ast.AssertOpGreater, Expected: "1", Pos: ast.Position{Line: 5}}, wantPass: true},
		{name: "greater equal pass", assertion: ast.Assertion{Subject: ast.AssertSubjectJSON, Path: "data.count", Operator: ast.AssertOpGreaterEqual, Expected: "2", Pos: ast.Position{Line: 6}}, wantPass: true},
		{name: "less pass", assertion: ast.Assertion{Subject: ast.AssertSubjectJSON, Path: "data.count", Operator: ast.AssertOpLess, Expected: "3", Pos: ast.Position{Line: 7}}, wantPass: true},
		{name: "less equal pass", assertion: ast.Assertion{Subject: ast.AssertSubjectJSON, Path: "data.count", Operator: ast.AssertOpLessEqual, Expected: "2", Pos: ast.Position{Line: 8}}, wantPass: true},
		{name: "null equal pass", assertion: ast.Assertion{Subject: ast.AssertSubjectJSON, Path: "data.nullable", Operator: ast.AssertOpEqual, Expected: "null", Pos: ast.Position{Line: 9}}, wantPass: true},
		{name: "equal fail", assertion: ast.Assertion{Subject: ast.AssertSubjectJSON, Path: "data.name", Operator: ast.AssertOpEqual, Expected: `"other"`, Pos: ast.Position{Line: 10}}, wantText: `expected json.data.name == "other", got "demo"`},
		{name: "missing fail", assertion: ast.Assertion{Subject: ast.AssertSubjectJSON, Path: "data.missing", Operator: ast.AssertOpEqual, Expected: `"other"`, Pos: ast.Position{Line: 11}}, wantText: `"json.data.missing" not found`},
		{name: "type fail", assertion: ast.Assertion{Subject: ast.AssertSubjectJSON, Path: "data.name", Operator: ast.AssertOpGreater, Expected: "1", Pos: ast.Position{Line: 12}}, wantText: `expected numeric JSON value at "json.data.name", got "demo"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			failures := Check(response, body, []ast.Assertion{tt.assertion})
			assertFailures(t, failures, tt.wantPass, tt.wantText)
		})
	}
}

func TestCheckJSONEdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		body      []byte
		assertion ast.Assertion
		wantText  string
	}{
		{
			name:      "non json body",
			body:      []byte("plain text"),
			assertion: ast.Assertion{Subject: ast.AssertSubjectJSON, Path: "data.name", Operator: ast.AssertOpEqual, Expected: `"demo"`, Pos: ast.Position{Line: 1}},
			wantText:  `expected JSON response body for "json.data.name"`,
		},
		{
			name:      "invalid array index",
			body:      []byte(`{"data":{"items":["one","two"]}}`),
			assertion: ast.Assertion{Subject: ast.AssertSubjectJSON, Path: "data.items.first", Operator: ast.AssertOpEqual, Expected: `"one"`, Pos: ast.Position{Line: 2}},
			wantText:  `json path "data.items.first" requires numeric index at "first"`,
		},
		{
			name:      "out of range exists treated missing",
			body:      []byte(`{"data":{"items":["one","two"]}}`),
			assertion: ast.Assertion{Subject: ast.AssertSubjectJSON, Path: "data.items.4", Operator: ast.AssertOpExists, Pos: ast.Position{Line: 3}},
			wantText:  `expected "json.data.items.4" to exist`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			failures := Check(&http.Response{StatusCode: http.StatusOK}, tt.body, []ast.Assertion{tt.assertion})
			assertFailures(t, failures, false, tt.wantText)
		})
	}
}

func runAssertionCases(t *testing.T, response *http.Response, body []byte, tests []struct {
	name      string
	assertion ast.Assertion
	wantPass  bool
	wantText  string
}) {
	t.Helper()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			failures := Check(response, body, []ast.Assertion{tt.assertion})
			assertFailures(t, failures, tt.wantPass, tt.wantText)
		})
	}
}

func assertFailures(t *testing.T, failures []string, wantPass bool, wantText string) {
	t.Helper()

	if wantPass {
		if len(failures) != 0 {
			t.Fatalf("expected no failures, got %v", failures)
		}
		return
	}

	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %v", failures)
	}
	if !strings.Contains(failures[0], wantText) {
		t.Fatalf("expected failure containing %q, got %q", wantText, failures[0])
	}
}
