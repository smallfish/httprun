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

type RequestBlock struct {
	Name              string
	Method            string
	URL               string
	Headers           []Header
	Body              string
	BodyFile          string
	BodyPos           Position
	Timeout           *time.Duration
	ConnectionTimeout *time.Duration
	NoRedirect        bool
	NoCookieJar       bool
	Pos               Position
}
