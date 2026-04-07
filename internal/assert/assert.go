package assert

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/textproto"
	"reflect"
	"strconv"
	"strings"

	"github.com/smallfish/httprun/internal/ast"
	"github.com/smallfish/httprun/internal/jsonpath"
)

func Check(response *http.Response, body []byte, assertions []ast.Assertion) []string {
	if len(assertions) == 0 {
		return nil
	}

	var (
		failures []string
		decoded  any
		jsonErr  error
		jsonRead bool
	)

	for _, assertion := range assertions {
		var failure string
		switch assertion.Subject {
		case ast.AssertSubjectStatus:
			failure = checkStatusAssertion(response, assertion)
		case ast.AssertSubjectBody:
			failure = checkBodyAssertion(body, assertion)
		case ast.AssertSubjectHeader:
			failure = checkHeaderAssertion(response, assertion)
		case ast.AssertSubjectJSON:
			if !jsonRead {
				jsonRead = true
				jsonErr = json.Unmarshal(body, &decoded)
			}
			failure = checkJSONAssertion(decoded, jsonErr, assertion)
		default:
			failure = fmt.Sprintf("line %d: unsupported assertion subject %q", assertion.Pos.Line, assertion.Subject)
		}

		if failure != "" {
			failures = append(failures, failure)
		}
	}

	return failures
}

func checkStatusAssertion(response *http.Response, assertion ast.Assertion) string {
	if response == nil {
		return fmt.Sprintf("line %d: expected status response, got no response", assertion.Pos.Line)
	}

	expected, err := strconv.Atoi(assertion.Expected)
	if err != nil {
		return fmt.Sprintf("line %d: invalid status comparison value %q", assertion.Pos.Line, assertion.Expected)
	}
	actual := response.StatusCode
	if compareInts(actual, expected, assertion.Operator) {
		return ""
	}

	return fmt.Sprintf("line %d: expected status %s %d, got %d", assertion.Pos.Line, assertion.Operator, expected, actual)
}

func checkBodyAssertion(body []byte, assertion ast.Assertion) string {
	actual := string(body)
	switch assertion.Operator {
	case ast.AssertOpExists:
		if actual != "" {
			return ""
		}
		return fmt.Sprintf("line %d: expected body to exist", assertion.Pos.Line)
	case ast.AssertOpNotExists:
		if actual == "" {
			return ""
		}
		return fmt.Sprintf("line %d: expected body to not exist", assertion.Pos.Line)
	}

	expected := parseStringOperand(assertion.Expected)
	if compareStrings(actual, expected, assertion.Operator) {
		return ""
	}

	switch assertion.Operator {
	case ast.AssertOpEqual, ast.AssertOpNotEqual:
		return fmt.Sprintf("line %d: expected body %s %q, got %q", assertion.Pos.Line, assertion.Operator, expected, actual)
	case ast.AssertOpContains:
		return fmt.Sprintf("line %d: expected body to contain %q", assertion.Pos.Line, expected)
	case ast.AssertOpNotContains:
		return fmt.Sprintf("line %d: expected body to not contain %q", assertion.Pos.Line, expected)
	default:
		return fmt.Sprintf("line %d: unsupported body operator %q", assertion.Pos.Line, assertion.Operator)
	}
}

func checkHeaderAssertion(response *http.Response, assertion ast.Assertion) string {
	if response == nil {
		return fmt.Sprintf("line %d: expected headers response, got no response", assertion.Pos.Line)
	}

	name := textproto.CanonicalMIMEHeaderKey(assertion.Path)
	values := response.Header.Values(name)
	actual := strings.Join(values, ", ")

	switch assertion.Operator {
	case ast.AssertOpExists:
		if len(values) > 0 {
			return ""
		}
		return fmt.Sprintf("line %d: expected header %q to exist", assertion.Pos.Line, name)
	case ast.AssertOpNotExists:
		if len(values) == 0 {
			return ""
		}
		return fmt.Sprintf("line %d: expected header %q to not exist", assertion.Pos.Line, name)
	}

	expected := parseStringOperand(assertion.Expected)
	if compareStrings(actual, expected, assertion.Operator) {
		return ""
	}

	switch assertion.Operator {
	case ast.AssertOpEqual, ast.AssertOpNotEqual:
		return fmt.Sprintf("line %d: expected header %q %s %q, got %q", assertion.Pos.Line, name, assertion.Operator, expected, actual)
	case ast.AssertOpContains:
		return fmt.Sprintf("line %d: expected header %q to contain %q, got %q", assertion.Pos.Line, name, expected, actual)
	case ast.AssertOpNotContains:
		return fmt.Sprintf("line %d: expected header %q to not contain %q, got %q", assertion.Pos.Line, name, expected, actual)
	default:
		return fmt.Sprintf("line %d: unsupported header operator %q", assertion.Pos.Line, assertion.Operator)
	}
}

func checkJSONAssertion(decoded any, jsonErr error, assertion ast.Assertion) string {
	if jsonErr != nil {
		return fmt.Sprintf("line %d: expected JSON response body for %q", assertion.Pos.Line, formatSubject(assertion))
	}

	actual, found, err := jsonpath.Lookup(decoded, assertion.Path)
	switch assertion.Operator {
	case ast.AssertOpExists:
		if err != nil {
			return fmt.Sprintf("line %d: %v", assertion.Pos.Line, err)
		}
		if found {
			return ""
		}
		return fmt.Sprintf("line %d: expected %q to exist", assertion.Pos.Line, formatSubject(assertion))
	case ast.AssertOpNotExists:
		if err != nil {
			return fmt.Sprintf("line %d: %v", assertion.Pos.Line, err)
		}
		if !found {
			return ""
		}
		return fmt.Sprintf("line %d: expected %q to not exist", assertion.Pos.Line, formatSubject(assertion))
	}

	if err != nil {
		return fmt.Sprintf("line %d: %v", assertion.Pos.Line, err)
	}
	if !found {
		return fmt.Sprintf("line %d: %q not found", assertion.Pos.Line, formatSubject(assertion))
	}

	switch assertion.Operator {
	case ast.AssertOpEqual, ast.AssertOpNotEqual:
		var expected any
		if err := json.Unmarshal([]byte(assertion.Expected), &expected); err != nil {
			return fmt.Sprintf("line %d: invalid JSON comparison value %q", assertion.Pos.Line, assertion.Expected)
		}
		matched := reflect.DeepEqual(actual, expected)
		if (assertion.Operator == ast.AssertOpEqual && matched) || (assertion.Operator == ast.AssertOpNotEqual && !matched) {
			return ""
		}
		return fmt.Sprintf("line %d: expected %s %s %s, got %s", assertion.Pos.Line, formatSubject(assertion), assertion.Operator, formatJSONValue(expected), formatJSONValue(actual))
	case ast.AssertOpGreater, ast.AssertOpGreaterEqual, ast.AssertOpLess, ast.AssertOpLessEqual:
		actualNumber, ok := actual.(float64)
		if !ok {
			return fmt.Sprintf("line %d: expected numeric JSON value at %q, got %s", assertion.Pos.Line, formatSubject(assertion), formatJSONValue(actual))
		}
		expectedNumber, err := parseJSONNumber(assertion.Expected)
		if err != nil {
			return fmt.Sprintf("line %d: invalid numeric JSON comparison value %q", assertion.Pos.Line, assertion.Expected)
		}
		if compareNumbers(actualNumber, expectedNumber, assertion.Operator) {
			return ""
		}
		return fmt.Sprintf("line %d: expected %s %s %s, got %s", assertion.Pos.Line, formatSubject(assertion), assertion.Operator, formatJSONValue(expectedNumber), formatJSONValue(actualNumber))
	default:
		return fmt.Sprintf("line %d: unsupported json operator %q", assertion.Pos.Line, assertion.Operator)
	}
}

func compareInts(actual, expected int, operator ast.AssertionOperator) bool {
	switch operator {
	case ast.AssertOpEqual:
		return actual == expected
	case ast.AssertOpNotEqual:
		return actual != expected
	case ast.AssertOpGreater:
		return actual > expected
	case ast.AssertOpGreaterEqual:
		return actual >= expected
	case ast.AssertOpLess:
		return actual < expected
	case ast.AssertOpLessEqual:
		return actual <= expected
	default:
		return false
	}
}

func compareNumbers(actual, expected float64, operator ast.AssertionOperator) bool {
	switch operator {
	case ast.AssertOpGreater:
		return actual > expected
	case ast.AssertOpGreaterEqual:
		return actual >= expected
	case ast.AssertOpLess:
		return actual < expected
	case ast.AssertOpLessEqual:
		return actual <= expected
	default:
		return false
	}
}

func compareStrings(actual, expected string, operator ast.AssertionOperator) bool {
	switch operator {
	case ast.AssertOpEqual:
		return actual == expected
	case ast.AssertOpNotEqual:
		return actual != expected
	case ast.AssertOpContains:
		return strings.Contains(actual, expected)
	case ast.AssertOpNotContains:
		return !strings.Contains(actual, expected)
	default:
		return false
	}
}

func parseStringOperand(input string) string {
	var decoded string
	if err := json.Unmarshal([]byte(input), &decoded); err == nil {
		return decoded
	}
	return input
}

func parseJSONNumber(input string) (float64, error) {
	var number float64
	if err := json.Unmarshal([]byte(input), &number); err != nil {
		return 0, err
	}
	return number, nil
}

func formatJSONValue(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(data)
}

func formatSubject(assertion ast.Assertion) string {
	if assertion.Path == "" {
		return string(assertion.Subject)
	}
	return fmt.Sprintf("%s.%s", assertion.Subject, assertion.Path)
}
