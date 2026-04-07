package parser

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/smallfish/httprun/internal/ast"
)

var durationDirectivePattern = regexp.MustCompile(`^([0-9]+)\s*(ms|s|m)?(?:\s+(.*))?$`)
var unaryAssertionPattern = regexp.MustCompile(`^(.+?)\s+(exists|not_exists)\s*$`)
var wordBinaryAssertionPattern = regexp.MustCompile(`^(.+?)\s+(contains|not_contains)\s+(.+)$`)
var symbolBinaryAssertionPattern = regexp.MustCompile(`^(.+?)\s*(==|!=|>=|<=|>|<)\s*(.+)$`)

type parseLine struct {
	number int
	text   string
}

func ParseFile(path string) (ast.Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ast.Document{}, err
	}

	doc, err := Parse(path, string(data))
	if err != nil {
		return ast.Document{}, err
	}
	return doc, nil
}

func Parse(path, input string) (ast.Document, error) {
	normalized := strings.ReplaceAll(input, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")

	lines := strings.Split(normalized, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	segments := splitSegments(lines)

	doc := ast.Document{Path: path}
	for _, segment := range segments {
		vars, req, err := parseSegment(segment)
		if err != nil {
			return ast.Document{}, err
		}
		doc.Variables = append(doc.Variables, vars...)
		if req != nil {
			doc.Requests = append(doc.Requests, *req)
		}
	}

	return doc, nil
}

func splitSegments(lines []string) [][]parseLine {
	var segments [][]parseLine
	current := make([]parseLine, 0)

	for idx, line := range lines {
		if isSegmentSeparator(line) {
			if len(current) > 0 {
				segments = append(segments, current)
				current = make([]parseLine, 0)
			}
			continue
		}
		current = append(current, parseLine{number: idx + 1, text: line})
	}

	if len(current) > 0 {
		segments = append(segments, current)
	}

	return segments
}

func isSegmentSeparator(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "###" {
		return true
	}
	if !strings.HasPrefix(trimmed, "###") {
		return false
	}
	if len(trimmed) == 3 {
		return true
	}
	return trimmed[3] == ' ' || trimmed[3] == '\t'
}

func parseSegment(lines []parseLine) ([]ast.VariableDecl, *ast.RequestBlock, error) {
	var (
		variables []ast.VariableDecl
		request   *ast.RequestBlock
		bodyStart = -1
	)

	for idx, line := range lines {
		trimmed := strings.TrimSpace(line.text)

		if request == nil {
			switch {
			case trimmed == "":
				continue
			case isRequestDirective(trimmed):
				nextRequest, err := parseRequestDirective(trimmed, request, line.number)
				if err != nil {
					return nil, nil, err
				}
				request = nextRequest
			case isRequestName(trimmed):
				name := parseRequestName(trimmed)
				if name == "" {
					return nil, nil, fmt.Errorf("%d: empty request name", line.number)
				}
				if request != nil {
					request.Name = name
				} else {
					request = &ast.RequestBlock{
						Name: name,
						Pos:  ast.Position{Line: line.number, Column: 1},
					}
				}
			case isComment(trimmed):
				continue
			case isVariableDecl(trimmed):
				variable, err := parseVariableDecl(line)
				if err != nil {
					return nil, nil, err
				}
				variables = append(variables, variable)
			default:
				method, url, err := parseRequestLine(line)
				if err != nil {
					return nil, nil, err
				}
				if request == nil {
					request = &ast.RequestBlock{}
				}
				request.Method = method
				request.URL = url
				request.Pos = ast.Position{Line: line.number, Column: 1}
			}
			continue
		}

		if request.Method == "" {
			switch {
			case trimmed == "":
				continue
			case isRequestDirective(trimmed):
				nextRequest, err := parseRequestDirective(trimmed, request, line.number)
				if err != nil {
					return nil, nil, err
				}
				request = nextRequest
				continue
			case isRequestName(trimmed):
				name := parseRequestName(trimmed)
				if name == "" {
					return nil, nil, fmt.Errorf("%d: empty request name", line.number)
				}
				request.Name = name
				continue
			case isComment(trimmed):
				continue
			case isVariableDecl(trimmed):
				variable, err := parseVariableDecl(line)
				if err != nil {
					return nil, nil, err
				}
				variables = append(variables, variable)
				continue
			}

			method, url, err := parseRequestLine(line)
			if err != nil {
				return nil, nil, err
			}
			request.Method = method
			request.URL = url
			request.Pos = ast.Position{Line: line.number, Column: 1}
			continue
		}

		if bodyStart >= 0 {
			if request.BodyFile != "" {
				if trimmed != "" {
					return nil, nil, fmt.Errorf("%d: external body file cannot be combined with inline body content", line.number)
				}
				continue
			}

			if request.Body == "" && isBodyFileRef(trimmed) {
				bodyFile := parseBodyFileRef(trimmed)
				if bodyFile == "" {
					return nil, nil, fmt.Errorf("%d: body file path cannot be empty", line.number)
				}
				request.BodyFile = bodyFile
				request.BodyPos = ast.Position{Line: line.number, Column: 1}
				continue
			}

			if request.Body == "" {
				request.Body = line.text
			} else {
				request.Body += "\n" + line.text
			}
			continue
		}

		if trimmed == "" {
			bodyStart = idx + 1
			continue
		}
		if isComment(trimmed) {
			continue
		}

		header, err := parseHeader(line)
		if err != nil {
			return nil, nil, err
		}
		request.Headers = append(request.Headers, header)
	}

	if request != nil && request.Method == "" {
		return nil, nil, fmt.Errorf("%d: missing request line after request metadata", request.Pos.Line)
	}
	if request != nil && request.Body != "" {
		request.Body = trimTrailingBlankLines(request.Body)
	}

	return variables, request, nil
}

func isComment(trimmed string) bool {
	return strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//")
}

func isRequestName(trimmed string) bool {
	return strings.HasPrefix(trimmed, "# @name ") || strings.HasPrefix(trimmed, "// @name ")
}

func isRequestDirective(trimmed string) bool {
	content, ok := stripCommentPrefix(trimmed)
	if !ok {
		return false
	}

	switch {
	case strings.HasPrefix(content, "@timeout "):
		return true
	case strings.HasPrefix(content, "@connection-timeout "):
		return true
	case content == "@no-redirect", strings.HasPrefix(content, "@no-redirect "):
		return true
	case content == "@no-cookie-jar", strings.HasPrefix(content, "@no-cookie-jar "):
		return true
	case strings.HasPrefix(content, "@capture "):
		return true
	case strings.HasPrefix(content, "@assert "):
		return true
	default:
		return false
	}
}

func parseRequestName(trimmed string) string {
	content, ok := stripCommentPrefix(trimmed)
	if !ok {
		return ""
	}
	parts := strings.Fields(content)
	if len(parts) < 2 || parts[0] != "@name" {
		return ""
	}
	return parts[1]
}

func parseRequestDirective(trimmed string, request *ast.RequestBlock, lineNumber int) (*ast.RequestBlock, error) {
	content, ok := stripCommentPrefix(trimmed)
	if !ok {
		return request, nil
	}
	if request == nil {
		request = &ast.RequestBlock{
			Pos: ast.Position{Line: lineNumber, Column: 1},
		}
	}

	switch {
	case strings.HasPrefix(content, "@timeout "):
		duration, remainder, err := parseDirectiveDuration(strings.TrimSpace(strings.TrimPrefix(content, "@timeout")))
		if err != nil {
			return nil, fmt.Errorf("%d: invalid @timeout: %w", lineNumber, err)
		}
		request.Timeout = &duration
		return applyDirectiveRemainder(request, remainder, lineNumber)
	case strings.HasPrefix(content, "@connection-timeout "):
		duration, remainder, err := parseDirectiveDuration(strings.TrimSpace(strings.TrimPrefix(content, "@connection-timeout")))
		if err != nil {
			return nil, fmt.Errorf("%d: invalid @connection-timeout: %w", lineNumber, err)
		}
		request.ConnectionTimeout = &duration
		return applyDirectiveRemainder(request, remainder, lineNumber)
	case content == "@no-redirect", strings.HasPrefix(content, "@no-redirect "):
		request.NoRedirect = true
		return applyDirectiveRemainder(request, strings.TrimSpace(strings.TrimPrefix(content, "@no-redirect")), lineNumber)
	case content == "@no-cookie-jar", strings.HasPrefix(content, "@no-cookie-jar "):
		request.NoCookieJar = true
		return applyDirectiveRemainder(request, strings.TrimSpace(strings.TrimPrefix(content, "@no-cookie-jar")), lineNumber)
	case strings.HasPrefix(content, "@capture "):
		capture, err := parseCapture(strings.TrimSpace(strings.TrimPrefix(content, "@capture")))
		if err != nil {
			return nil, fmt.Errorf("%d: invalid @capture: %w", lineNumber, err)
		}
		capture.Pos = ast.Position{Line: lineNumber, Column: 1}
		request.Captures = append(request.Captures, capture)
		return request, nil
	case strings.HasPrefix(content, "@assert "):
		assertion, err := parseAssertion(strings.TrimSpace(strings.TrimPrefix(content, "@assert")))
		if err != nil {
			return nil, fmt.Errorf("%d: invalid @assert: %w", lineNumber, err)
		}
		assertion.Pos = ast.Position{Line: lineNumber, Column: 1}
		request.Assertions = append(request.Assertions, assertion)
		return request, nil
	default:
		return request, nil
	}
}

func isVariableDecl(trimmed string) bool {
	return strings.HasPrefix(trimmed, "@") && strings.Contains(trimmed, "=")
}

func parseVariableDecl(line parseLine) (ast.VariableDecl, error) {
	trimmed := strings.TrimSpace(line.text)
	idx := strings.Index(trimmed, "=")
	if idx <= 1 {
		return ast.VariableDecl{}, fmt.Errorf("%d: invalid variable declaration", line.number)
	}

	name := strings.TrimSpace(trimmed[1:idx])
	value := strings.TrimSpace(trimmed[idx+1:])
	if name == "" {
		return ast.VariableDecl{}, fmt.Errorf("%d: variable name cannot be empty", line.number)
	}

	return ast.VariableDecl{
		Name:  name,
		Value: value,
		Pos:   ast.Position{Line: line.number, Column: 1},
	}, nil
}

func parseRequestLine(line parseLine) (string, string, error) {
	fields := strings.Fields(line.text)
	if len(fields) < 2 {
		return "", "", fmt.Errorf("%d: invalid request line, expected \"METHOD URL\"", line.number)
	}
	method := fields[0]
	url := strings.TrimSpace(line.text[len(method):])
	if method == "" || url == "" {
		return "", "", fmt.Errorf("%d: invalid request line, expected \"METHOD URL\"", line.number)
	}
	return strings.ToUpper(method), url, nil
}

func parseHeader(line parseLine) (ast.Header, error) {
	idx := strings.Index(line.text, ":")
	if idx <= 0 {
		return ast.Header{}, fmt.Errorf("%d: invalid header, expected \"Name: Value\"", line.number)
	}

	name := strings.TrimSpace(line.text[:idx])
	value := strings.TrimSpace(line.text[idx+1:])
	if name == "" {
		return ast.Header{}, fmt.Errorf("%d: header name cannot be empty", line.number)
	}

	return ast.Header{
		Name:  name,
		Value: value,
		Pos:   ast.Position{Line: line.number, Column: 1},
	}, nil
}

func isBodyFileRef(trimmed string) bool {
	if len(trimmed) < 2 || trimmed[0] != '<' {
		return false
	}
	return trimmed[1] == ' ' || trimmed[1] == '\t'
}

func parseBodyFileRef(trimmed string) string {
	return strings.TrimSpace(trimmed[1:])
}

func trimTrailingBlankLines(input string) string {
	return strings.TrimRight(input, "\n")
}

func stripCommentPrefix(trimmed string) (string, bool) {
	switch {
	case strings.HasPrefix(trimmed, "#"):
		return strings.TrimSpace(strings.TrimPrefix(trimmed, "#")), true
	case strings.HasPrefix(trimmed, "//"):
		return strings.TrimSpace(strings.TrimPrefix(trimmed, "//")), true
	default:
		return "", false
	}
}

func parseDirectiveDuration(spec string) (time.Duration, string, error) {
	matches := durationDirectivePattern.FindStringSubmatch(spec)
	if len(matches) != 4 {
		return 0, "", fmt.Errorf("expected integer duration with optional unit")
	}

	value, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, "", err
	}
	if value <= 0 {
		return 0, "", fmt.Errorf("duration must be positive")
	}

	switch matches[2] {
	case "", "s":
		return time.Duration(value) * time.Second, matches[3], nil
	case "ms":
		return time.Duration(value) * time.Millisecond, matches[3], nil
	case "m":
		return time.Duration(value) * time.Minute, matches[3], nil
	default:
		return 0, "", fmt.Errorf("unsupported duration unit %q", matches[2])
	}
}

func applyDirectiveRemainder(request *ast.RequestBlock, remainder string, lineNumber int) (*ast.RequestBlock, error) {
	remainder = strings.TrimSpace(remainder)
	if remainder == "" {
		return request, nil
	}
	if request.Method != "" {
		return nil, fmt.Errorf("%d: request line already defined", lineNumber)
	}

	method, url, err := parseRequestLine(parseLine{number: lineNumber, text: remainder})
	if err != nil {
		return nil, err
	}
	request.Method = method
	request.URL = url
	if request.Pos.Line == 0 {
		request.Pos = ast.Position{Line: lineNumber, Column: 1}
	}
	return request, nil
}

func parseAssertion(spec string) (ast.Assertion, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return ast.Assertion{}, fmt.Errorf("expression cannot be empty")
	}

	subjectText, operator, expected, hasExpected, err := splitAssertionExpression(spec)
	if err != nil {
		return ast.Assertion{}, err
	}

	subject, path, err := parseAssertionSubject(subjectText)
	if err != nil {
		return ast.Assertion{}, err
	}

	assertion := ast.Assertion{
		Subject:  subject,
		Path:     path,
		Operator: operator,
		Expected: expected,
	}
	if err := validateAssertion(assertion, hasExpected); err != nil {
		return ast.Assertion{}, err
	}
	return assertion, nil
}

func parseCapture(spec string) (ast.Capture, error) {
	idx := strings.Index(spec, "=")
	if idx < 0 {
		return ast.Capture{}, fmt.Errorf("expected target = source")
	}

	name := strings.TrimSpace(spec[:idx])
	source := strings.TrimSpace(spec[idx+1:])
	if name == "" {
		return ast.Capture{}, fmt.Errorf("target variable cannot be empty")
	}
	if strings.ContainsAny(name, " \t") {
		return ast.Capture{}, fmt.Errorf("target variable cannot contain whitespace")
	}
	if source == "" {
		return ast.Capture{}, fmt.Errorf("capture source cannot be empty")
	}

	switch {
	case source == "status":
		return ast.Capture{Name: name, Subject: ast.CaptureSubjectStatus}, nil
	case source == "body":
		return ast.Capture{Name: name, Subject: ast.CaptureSubjectBody}, nil
	case strings.HasPrefix(source, "json."):
		path := strings.TrimSpace(strings.TrimPrefix(source, "json."))
		if err := validateJSONPath(path); err != nil {
			return ast.Capture{}, err
		}
		return ast.Capture{Name: name, Subject: ast.CaptureSubjectJSON, Path: path}, nil
	case strings.HasPrefix(source, "header."):
		headerName := strings.TrimSpace(strings.TrimPrefix(source, "header."))
		if headerName == "" {
			return ast.Capture{}, fmt.Errorf("header name cannot be empty")
		}
		return ast.Capture{Name: name, Subject: ast.CaptureSubjectHeader, Path: headerName}, nil
	default:
		return ast.Capture{}, fmt.Errorf("unsupported capture source %q", source)
	}
}

func splitAssertionExpression(spec string) (string, ast.AssertionOperator, string, bool, error) {
	if matches := unaryAssertionPattern.FindStringSubmatch(spec); len(matches) == 3 {
		subject := strings.TrimSpace(matches[1])
		if subject == "" {
			return "", "", "", false, fmt.Errorf("assertion subject cannot be empty")
		}
		return subject, ast.AssertionOperator(matches[2]), "", false, nil
	}

	if matches := wordBinaryAssertionPattern.FindStringSubmatch(spec); len(matches) == 4 {
		subject := strings.TrimSpace(matches[1])
		expected := strings.TrimSpace(matches[3])
		if subject == "" {
			return "", "", "", false, fmt.Errorf("assertion subject cannot be empty")
		}
		if expected == "" {
			return "", "", "", false, fmt.Errorf("assertion value cannot be empty")
		}
		return subject, ast.AssertionOperator(matches[2]), expected, true, nil
	}

	if matches := symbolBinaryAssertionPattern.FindStringSubmatch(spec); len(matches) == 4 {
		subject := strings.TrimSpace(matches[1])
		expected := strings.TrimSpace(matches[3])
		if subject == "" {
			return "", "", "", false, fmt.Errorf("assertion subject cannot be empty")
		}
		if expected == "" {
			return "", "", "", false, fmt.Errorf("assertion value cannot be empty")
		}
		return subject, ast.AssertionOperator(matches[2]), expected, true, nil
	}

	return "", "", "", false, fmt.Errorf("expected an operator such as ==, !=, contains, or exists")
}

func parseAssertionSubject(input string) (ast.AssertionSubject, string, error) {
	switch {
	case input == "status":
		return ast.AssertSubjectStatus, "", nil
	case input == "body":
		return ast.AssertSubjectBody, "", nil
	case strings.HasPrefix(input, "json."):
		path := strings.TrimSpace(strings.TrimPrefix(input, "json."))
		if err := validateJSONPath(path); err != nil {
			return "", "", err
		}
		return ast.AssertSubjectJSON, path, nil
	case strings.HasPrefix(input, "header."):
		name := strings.TrimSpace(strings.TrimPrefix(input, "header."))
		if name == "" {
			return "", "", fmt.Errorf("header name cannot be empty")
		}
		return ast.AssertSubjectHeader, name, nil
	default:
		return "", "", fmt.Errorf("unsupported assertion subject %q", input)
	}
}

func validateAssertion(assertion ast.Assertion, hasExpected bool) error {
	switch assertion.Operator {
	case ast.AssertOpExists, ast.AssertOpNotExists:
		if hasExpected || assertion.Expected != "" {
			return fmt.Errorf("%s does not accept a comparison value", assertion.Operator)
		}
	default:
		if !hasExpected || assertion.Expected == "" {
			return fmt.Errorf("%s requires a comparison value", assertion.Operator)
		}
	}

	switch assertion.Subject {
	case ast.AssertSubjectStatus:
		switch assertion.Operator {
		case ast.AssertOpEqual, ast.AssertOpNotEqual, ast.AssertOpGreater, ast.AssertOpGreaterEqual, ast.AssertOpLess, ast.AssertOpLessEqual:
			_, err := parseStatusCode(assertion.Expected)
			return err
		default:
			return fmt.Errorf("status supports only ==, !=, >, >=, <, <=")
		}
	case ast.AssertSubjectBody:
		switch assertion.Operator {
		case ast.AssertOpEqual, ast.AssertOpNotEqual, ast.AssertOpContains, ast.AssertOpNotContains, ast.AssertOpExists, ast.AssertOpNotExists:
			return nil
		default:
			return fmt.Errorf("body supports only ==, !=, contains, not_contains, exists, not_exists")
		}
	case ast.AssertSubjectHeader:
		switch assertion.Operator {
		case ast.AssertOpEqual, ast.AssertOpNotEqual, ast.AssertOpContains, ast.AssertOpNotContains, ast.AssertOpExists, ast.AssertOpNotExists:
			return nil
		default:
			return fmt.Errorf("header supports only ==, !=, contains, not_contains, exists, not_exists")
		}
	case ast.AssertSubjectJSON:
		switch assertion.Operator {
		case ast.AssertOpExists, ast.AssertOpNotExists:
			return nil
		case ast.AssertOpEqual, ast.AssertOpNotEqual:
			if !json.Valid([]byte(assertion.Expected)) {
				return fmt.Errorf("json comparison value must be valid JSON")
			}
			return nil
		case ast.AssertOpGreater, ast.AssertOpGreaterEqual, ast.AssertOpLess, ast.AssertOpLessEqual:
			if _, err := parseJSONNumber(assertion.Expected); err != nil {
				return err
			}
			return nil
		default:
			return fmt.Errorf("json supports only ==, !=, >, >=, <, <=, exists, not_exists")
		}
	default:
		return fmt.Errorf("unsupported assertion subject %q", assertion.Subject)
	}
}

func validateJSONPath(path string) error {
	if path == "" {
		return fmt.Errorf("json path cannot be empty")
	}
	for _, part := range strings.Split(path, ".") {
		if strings.TrimSpace(part) == "" {
			return fmt.Errorf("json path contains an empty segment")
		}
	}
	return nil
}

func parseStatusCode(input string) (int, error) {
	statusCode, err := strconv.Atoi(input)
	if err != nil {
		return 0, fmt.Errorf("status comparison value must be an integer")
	}
	if statusCode < 100 || statusCode > 599 {
		return 0, fmt.Errorf("status code must be between 100 and 599")
	}
	return statusCode, nil
}

func parseJSONNumber(input string) (float64, error) {
	var number float64
	if err := json.Unmarshal([]byte(input), &number); err != nil {
		return 0, fmt.Errorf("numeric json comparison value must be a JSON number")
	}
	return number, nil
}
