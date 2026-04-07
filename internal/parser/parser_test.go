package parser

import (
	"strings"
	"testing"
	"time"

	"github.com/smallfish/httprun/internal/ast"
)

func TestParseDocument(t *testing.T) {
	input := `
@host = https://example.com

### 
# @name listUsers
GET {{host}}/users
Accept: application/json

###
POST {{host}}/login
Content-Type: application/json

{"name":"demo"}
`

	doc, err := Parse("demo.http", input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(doc.Variables) != 1 {
		t.Fatalf("expected 1 variable, got %d", len(doc.Variables))
	}
	if len(doc.Requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(doc.Requests))
	}
	if got := doc.Requests[0].Name; got != "listUsers" {
		t.Fatalf("expected name listUsers, got %q", got)
	}
	if got := doc.Requests[1].Method; got != "POST" {
		t.Fatalf("expected POST, got %q", got)
	}
	if got := doc.Requests[1].Body; got != `{"name":"demo"}` {
		t.Fatalf("unexpected body %q", got)
	}
}

func TestParseExternalBodyFile(t *testing.T) {
	input := `
###
# @name fromFile
POST https://example.com/items
Content-Type: application/json

< ./payload.json
`

	doc, err := Parse("demo.http", input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	if got := doc.Requests[0].BodyFile; got != "./payload.json" {
		t.Fatalf("expected external body file, got %q", got)
	}
	if got := doc.Requests[0].Body; got != "" {
		t.Fatalf("expected empty inline body, got %q", got)
	}
}

func TestParseTrimsTrailingBodySeparatorWhitespace(t *testing.T) {
	input := `
###
POST https://example.com/items
Content-Type: application/json

{"name":"demo"}

###
GET https://example.com/items
`

	doc, err := Parse("demo.http", input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if got := doc.Requests[0].Body; got != `{"name":"demo"}` {
		t.Fatalf("unexpected body %q", got)
	}
}

func TestParseRequestDirectives(t *testing.T) {
	input := `
	###
	# @name tuned
	# @timeout 150ms
	# @connection-timeout 2s
	# @no-redirect
	# @no-cookie-jar
	# @capture itemId = json.data.id
	# @capture traceId = header.X-Trace-Id
	# @assert status == 200
	# @assert body contains "ok"
	# @assert json.data.user.name == "demo"
	# @assert header.Content-Type exists
	GET https://example.com/items
`

	doc, err := Parse("demo.http", input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	request := doc.Requests[0]
	if request.Timeout == nil || *request.Timeout != 150*time.Millisecond {
		t.Fatalf("unexpected timeout %+v", request.Timeout)
	}
	if request.ConnectionTimeout == nil || *request.ConnectionTimeout != 2*time.Second {
		t.Fatalf("unexpected connection timeout %+v", request.ConnectionTimeout)
	}
	if !request.NoRedirect {
		t.Fatalf("expected no redirect directive")
	}
	if !request.NoCookieJar {
		t.Fatalf("expected no cookie jar directive")
	}
	if len(request.Captures) != 2 {
		t.Fatalf("expected 2 captures, got %d", len(request.Captures))
	}
	if request.Captures[0].Name != "itemId" || request.Captures[0].Subject != ast.CaptureSubjectJSON || request.Captures[0].Path != "data.id" {
		t.Fatalf("unexpected first capture %+v", request.Captures[0])
	}
	if request.Captures[1].Name != "traceId" || request.Captures[1].Subject != ast.CaptureSubjectHeader || request.Captures[1].Path != "X-Trace-Id" {
		t.Fatalf("unexpected second capture %+v", request.Captures[1])
	}
	if len(request.Assertions) != 4 {
		t.Fatalf("expected 4 assertions, got %d", len(request.Assertions))
	}
	if request.Assertions[0].Subject != ast.AssertSubjectStatus || request.Assertions[0].Operator != ast.AssertOpEqual || request.Assertions[0].Expected != "200" {
		t.Fatalf("unexpected status assertion %+v", request.Assertions[0])
	}
	if request.Assertions[1].Subject != ast.AssertSubjectBody || request.Assertions[1].Operator != ast.AssertOpContains || request.Assertions[1].Expected != `"ok"` {
		t.Fatalf("unexpected body contains assertion %+v", request.Assertions[1])
	}
	if request.Assertions[2].Subject != ast.AssertSubjectJSON || request.Assertions[2].Path != "data.user.name" || request.Assertions[2].Expected != `"demo"` {
		t.Fatalf("unexpected json assertion %+v", request.Assertions[2])
	}
	if request.Assertions[3].Subject != ast.AssertSubjectHeader || request.Assertions[3].Path != "Content-Type" || request.Assertions[3].Operator != ast.AssertOpExists {
		t.Fatalf("unexpected header assertion %+v", request.Assertions[3])
	}
}

func TestParseInlineDirectiveRequestLine(t *testing.T) {
	input := `
###
# @no-redirect GET https://example.com/redirect
`

	doc, err := Parse("demo.http", input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	request := doc.Requests[0]
	if request.Method != "GET" {
		t.Fatalf("unexpected method %q", request.Method)
	}
	if request.URL != "https://example.com/redirect" {
		t.Fatalf("unexpected url %q", request.URL)
	}
	if !request.NoRedirect {
		t.Fatalf("expected no redirect directive")
	}
}

func TestParseSegmentSeparatorWithComment(t *testing.T) {
	input := `
### first request
GET https://example.com/first

### second request
POST https://example.com/second
Content-Type: application/json

{"name":"demo"}
`

	doc, err := Parse("demo.http", input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(doc.Requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(doc.Requests))
	}
	if got := doc.Requests[0].Method; got != "GET" {
		t.Fatalf("expected GET, got %q", got)
	}
	if got := doc.Requests[1].Method; got != "POST" {
		t.Fatalf("expected POST, got %q", got)
	}
}

func TestParseSegmentSeparatorWithoutWhitespaceIsNotSplit(t *testing.T) {
	input := `
###first request
GET https://example.com/first
`

	doc, err := Parse("demo.http", input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	if got := doc.Requests[0].Method; got != "GET" {
		t.Fatalf("expected GET, got %q", got)
	}
}

func TestParseRejectsInvalidAssertJSON(t *testing.T) {
	input := `
###
# @assert json.data.user.name == demo
GET https://example.com/items
`

	_, err := Parse("demo.http", input)
	if err == nil {
		t.Fatalf("expected parse error")
	}
	if got := err.Error(); got != "3: invalid @assert: json comparison value must be valid JSON" {
		t.Fatalf("unexpected error %q", got)
	}
}

func TestParseRejectsInvalidAssertOperatorForStatus(t *testing.T) {
	input := `
###
# @assert status contains "200"
GET https://example.com/items
`

	_, err := Parse("demo.http", input)
	if err == nil {
		t.Fatalf("expected parse error")
	}
	if got := err.Error(); got != "3: invalid @assert: status supports only ==, !=, >, >=, <, <=" {
		t.Fatalf("unexpected error %q", got)
	}
}

func TestParseSupportsRawBodyContainsAssertion(t *testing.T) {
	input := `
###
# @assert body contains hello
# @assert status!=200
GET https://example.com/items
`

	doc, err := Parse("demo.http", input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got := doc.Requests[0].Assertions[0].Expected; got != "hello" {
		t.Fatalf("unexpected error %q", got)
	}
	if got := doc.Requests[0].Assertions[1].Operator; got != ast.AssertOpNotEqual {
		t.Fatalf("unexpected operator %q", got)
	}
}

func TestParseSupportsAssertionSubjectsAndOperators(t *testing.T) {
	input := `
###
# @assert status >= 200
# @assert status<=299
# @assert body exists
# @assert body not_contains error
# @assert header.X-Trace-Id == trace-123
# @assert header.X-Trace-Id not_exists
# @assert json.data.count > 1
# @assert json.data.count <= 2
# @assert json.data.items exists
# @assert json.data.missing not_exists
GET https://example.com/items
`

	doc, err := Parse("demo.http", input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	assertions := doc.Requests[0].Assertions
	if len(assertions) != 10 {
		t.Fatalf("expected 10 assertions, got %d", len(assertions))
	}

	want := []ast.Assertion{
		{Subject: ast.AssertSubjectStatus, Operator: ast.AssertOpGreaterEqual, Expected: "200"},
		{Subject: ast.AssertSubjectStatus, Operator: ast.AssertOpLessEqual, Expected: "299"},
		{Subject: ast.AssertSubjectBody, Operator: ast.AssertOpExists},
		{Subject: ast.AssertSubjectBody, Operator: ast.AssertOpNotContains, Expected: "error"},
		{Subject: ast.AssertSubjectHeader, Path: "X-Trace-Id", Operator: ast.AssertOpEqual, Expected: "trace-123"},
		{Subject: ast.AssertSubjectHeader, Path: "X-Trace-Id", Operator: ast.AssertOpNotExists},
		{Subject: ast.AssertSubjectJSON, Path: "data.count", Operator: ast.AssertOpGreater, Expected: "1"},
		{Subject: ast.AssertSubjectJSON, Path: "data.count", Operator: ast.AssertOpLessEqual, Expected: "2"},
		{Subject: ast.AssertSubjectJSON, Path: "data.items", Operator: ast.AssertOpExists},
		{Subject: ast.AssertSubjectJSON, Path: "data.missing", Operator: ast.AssertOpNotExists},
	}

	for idx, assertion := range assertions {
		if assertion.Subject != want[idx].Subject || assertion.Path != want[idx].Path || assertion.Operator != want[idx].Operator || assertion.Expected != want[idx].Expected {
			t.Fatalf("unexpected assertion %d: %+v", idx, assertion)
		}
	}
}

func TestParseRejectsInvalidAssertionForms(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		substr string
	}{
		{
			name:   "unknown subject",
			line:   "# @assert cookie.session exists",
			substr: `unsupported assertion subject "cookie.session"`,
		},
		{
			name:   "empty json path",
			line:   "# @assert json. == 1",
			substr: "json path cannot be empty",
		},
		{
			name:   "empty header name",
			line:   "# @assert header. exists",
			substr: "header name cannot be empty",
		},
		{
			name:   "json numeric needs number",
			line:   `# @assert json.data.count > "many"`,
			substr: "numeric json comparison value must be a JSON number",
		},
		{
			name:   "body does not support greater",
			line:   "# @assert body > 1",
			substr: "body supports only ==",
		},
		{
			name:   "header does not support less",
			line:   "# @assert header.X-Test < 1",
			substr: "header supports only ==",
		},
		{
			name:   "status requires value",
			line:   "# @assert status ==",
			substr: "expected an operator such as ==",
		},
		{
			name:   "exists with extra value",
			line:   "# @assert body exists hello",
			substr: "expected an operator such as ==",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := "###\n" + tt.line + "\nGET https://example.com/items\n"
			_, err := Parse("demo.http", input)
			if err == nil {
				t.Fatalf("expected parse error")
			}
			if !strings.Contains(err.Error(), tt.substr) {
				t.Fatalf("expected error containing %q, got %q", tt.substr, err.Error())
			}
		})
	}
}

func TestParseSupportsCaptureSources(t *testing.T) {
	input := `
###
# @capture statusCode = status
# @capture rawBody = body
# @capture itemId = json.data.id
# @capture traceId = header.X-Trace-Id
GET https://example.com/items
`

	doc, err := Parse("demo.http", input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	captures := doc.Requests[0].Captures
	if len(captures) != 4 {
		t.Fatalf("expected 4 captures, got %d", len(captures))
	}

	want := []ast.Capture{
		{Name: "statusCode", Subject: ast.CaptureSubjectStatus},
		{Name: "rawBody", Subject: ast.CaptureSubjectBody},
		{Name: "itemId", Subject: ast.CaptureSubjectJSON, Path: "data.id"},
		{Name: "traceId", Subject: ast.CaptureSubjectHeader, Path: "X-Trace-Id"},
	}

	for idx, capture := range captures {
		if capture.Name != want[idx].Name || capture.Subject != want[idx].Subject || capture.Path != want[idx].Path {
			t.Fatalf("unexpected capture %d: %+v", idx, capture)
		}
	}
}

func TestParseRejectsInvalidCaptureForms(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		substr string
	}{
		{
			name:   "missing equals",
			line:   "# @capture test_id json.data.id",
			substr: "expected target = source",
		},
		{
			name:   "empty target",
			line:   "# @capture = json.data.id",
			substr: "target variable cannot be empty",
		},
		{
			name:   "whitespace target",
			line:   "# @capture test id = json.data.id",
			substr: "target variable cannot contain whitespace",
		},
		{
			name:   "empty source",
			line:   "# @capture test_id = ",
			substr: "capture source cannot be empty",
		},
		{
			name:   "bad json path",
			line:   "# @capture test_id = json.",
			substr: "json path cannot be empty",
		},
		{
			name:   "bad header name",
			line:   "# @capture trace = header.",
			substr: "header name cannot be empty",
		},
		{
			name:   "unsupported source",
			line:   "# @capture trace = cookie.session",
			substr: `unsupported capture source "cookie.session"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := "###\n" + tt.line + "\nGET https://example.com/items\n"
			_, err := Parse("demo.http", input)
			if err == nil {
				t.Fatalf("expected parse error")
			}
			if !strings.Contains(err.Error(), tt.substr) {
				t.Fatalf("expected error containing %q, got %q", tt.substr, err.Error())
			}
		})
	}
}
