// SPDX-License-Identifier: Apache-2.0

// Package textout writes formatted text and keeps the first write error.
package textout

import (
	"fmt"
	"io"
)

// Writer formats onto an io.Writer, remembering the first error so callers
// check once at the end.
type Writer struct {
	w   io.Writer
	err error
}

// New wraps w.
func New(w io.Writer) *Writer { return &Writer{w: w} }

// Printf formats like fmt.Fprintf unless an earlier write failed.
func (p *Writer) Printf(format string, args ...any) {
	if p.err != nil {
		return
	}
	_, p.err = fmt.Fprintf(p.w, format, args...)
}

// Err returns the first write error.
func (p *Writer) Err() error { return p.err }
