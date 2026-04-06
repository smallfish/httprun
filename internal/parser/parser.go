package parser

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/smallfish/httprun/internal/ast"
)

var durationDirectivePattern = regexp.MustCompile(`^([0-9]+)\s*(ms|s|m)?(?:\s+(.*))?$`)

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
