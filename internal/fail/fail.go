// Package fail defines dlktk's structured error envelope and exit-code mapping
// (design §8.4). Every command error becomes a JSON envelope on stderr with a
// stable code, so an agent harness can branch on it.
package fail

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/chazu/dlktk/internal/af"
	"github.com/chazu/dlktk/internal/ibis"
)

// Exit codes.
const (
	CodeOK       = 0
	CodeGeneric  = 1
	CodeIllegal  = 2 // ill-formed / illegal move; nothing written
	CodeNotFound = 3
	CodeStore    = 4 // store / engine error
)

// Error is a structured dlktk error.
type Error struct {
	ErrKind string `json:"error"`          // stable machine code, e.g. "illegal_move"
	Detail  string `json:"detail"`         // human-readable
	Node    string `json:"node,omitempty"` // offending node, if any
	code    int    // exit code (not serialized)
}

func (e *Error) Error() string { return e.Detail }

// New builds an Error.
func New(code int, kind, format string, a ...any) *Error {
	return &Error{ErrKind: kind, Detail: fmt.Sprintf(format, a...), code: code}
}

// NotFound is a convenience for code 3.
func NotFound(node, format string, a ...any) *Error {
	e := New(CodeNotFound, "not_found", format, a...)
	e.Node = node
	return e
}

// Store wraps a storage/engine error as code 4.
func Store(format string, a ...any) *Error {
	return New(CodeStore, "store_error", format, a...)
}

// Classify maps an arbitrary error to an Error envelope + exit code. ibis
// legality errors become illegal_move (2); *Error passes through; everything
// else is generic (1).
func Classify(err error) *Error {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	var im *ibis.IllegalMove
	if errors.As(err, &im) {
		return &Error{ErrKind: "illegal_move", Detail: im.Detail, Node: im.Node, code: CodeIllegal}
	}
	var nf *ibis.NotFound
	if errors.As(err, &nf) {
		return &Error{ErrKind: "not_found", Detail: nf.Detail, Node: nf.Node, code: CodeNotFound}
	}
	var cyc *af.PreferenceCycleError
	if errors.As(err, &cyc) {
		return &Error{ErrKind: "store_error", Detail: cyc.Error(), Node: cyc.Node, code: CodeStore}
	}
	return &Error{ErrKind: "error", Detail: err.Error(), code: CodeGeneric}
}

// Code returns the exit code for this error.
func (e *Error) Code() int { return e.code }

// JSON renders the envelope as a single-line JSON object.
func (e *Error) JSON() string {
	b, _ := json.Marshal(e)
	return string(b)
}
