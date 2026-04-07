package ast

import "time"

type Position struct {
	Line   int
	Column int
}

type Document struct {
	Path      string
	Variables []VariableDecl
	Requests  []RequestBlock
}

type VariableDecl struct {
	Name  string
	Value string
	Pos   Position
}

type Header struct {
	Name  string
	Value string
	Pos   Position
}

type AssertionSubject string

const (
	AssertSubjectStatus AssertionSubject = "status"
	AssertSubjectBody   AssertionSubject = "body"
	AssertSubjectJSON   AssertionSubject = "json"
	AssertSubjectHeader AssertionSubject = "header"
)

type AssertionOperator string

const (
	AssertOpEqual        AssertionOperator = "=="
	AssertOpNotEqual     AssertionOperator = "!="
	AssertOpContains     AssertionOperator = "contains"
	AssertOpNotContains  AssertionOperator = "not_contains"
	AssertOpExists       AssertionOperator = "exists"
	AssertOpNotExists    AssertionOperator = "not_exists"
	AssertOpGreater      AssertionOperator = ">"
	AssertOpGreaterEqual AssertionOperator = ">="
	AssertOpLess         AssertionOperator = "<"
	AssertOpLessEqual    AssertionOperator = "<="
)

type Assertion struct {
	Subject  AssertionSubject
	Path     string
	Operator AssertionOperator
	Expected string
	Pos      Position
}

type RequestBlock struct {
	Name              string
	Method            string
	URL               string
	Headers           []Header
	Assertions        []Assertion
	Body              string
	BodyFile          string
	BodyPos           Position
	Timeout           *time.Duration
	ConnectionTimeout *time.Duration
	NoRedirect        bool
	NoCookieJar       bool
	Pos               Position
}
