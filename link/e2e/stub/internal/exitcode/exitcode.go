// Package exitcode lets a cobra RunE choose the process exit code main exits with. The codes are the
// sysexits values that are part of the executable CLI contract the harness asserts on
// (e.g. invalid config MUST exit 64) — so a command signals its failure class by wrapping its error
// here rather than by string-matching on stderr.
package exitcode

import (
	"errors"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

// Error carries a CLI exit code alongside the underlying error.
type Error struct {
	Code int
	Err  error
}

func (e *Error) Error() string { return e.Err.Error() }

func (e *Error) Unwrap() error { return e.Err }

// New wraps err with the given CLI exit code.
func New(code int, err error) *Error { return &Error{Code: code, Err: err} }

// Of resolves the process exit code for err: a wrapped *Error's Code, ExitOK for nil, otherwise
// ExitInternal (an unexpected, uncoded error is a software fault by definition).
func Of(err error) int {
	if err == nil {
		return wire.ExitOK
	}
	var ce *Error
	if errors.As(err, &ce) {
		return ce.Code
	}
	return wire.ExitInternal
}
