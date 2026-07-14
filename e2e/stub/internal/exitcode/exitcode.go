// Package exitcode maps errors to sysexits codes the harness asserts on.
package exitcode

import (
	"errors"

	"github.com/cosmos/ibc/e2e/internal/harness/ibclink/wire"
)

type Error struct {
	Code int
	Err  error
}

func (e *Error) Error() string { return e.Err.Error() }

func (e *Error) Unwrap() error { return e.Err }

func New(code int, err error) *Error { return &Error{Code: code, Err: err} }

// Uncoded errors exit ExitInternal.
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
