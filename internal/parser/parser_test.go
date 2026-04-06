package parser

import (
	"testing"
	"time"
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
# @timeout 150 ms
# @connection-timeout 2 s
# @no-redirect
# @no-cookie-jar
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
