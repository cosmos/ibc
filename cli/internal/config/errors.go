// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"fmt"
	"strings"
)

// PathError is an error that wraps an error with a config path. supports nested paths.
type PathError struct {
	path string
	err  error
}

func (e *PathError) Unwrap() error {
	return e.err
}

func (e *PathError) Path() string {
	return e.path
}

func (e *PathError) Error() string {
	return fmt.Sprintf("%s: %s", e.path, e.err.Error())
}

func errPath(segment string, err error) error {
	if err == nil {
		return nil
	}

	if segment == "" {
		return err
	}

	var pe *PathError
	if errors.As(err, &pe) {
		// parent.child vs parent[123]
		var path string
		if strings.HasPrefix(pe.path, "[") {
			path = segment + pe.path
		} else {
			path = segment + "." + pe.path
		}

		return &PathError{
			path: path,
			err:  pe.err,
		}
	}

	return &PathError{
		path: segment,
		err:  err,
	}
}

// like errPath but for slice indexes
func errPathIndex(idx int, err error) error {
	return errPath(fmt.Sprintf("[%d]", idx), err)
}

// errPath + errorf
func errPathf(segment string, format string, args ...any) error {
	return errPath(segment, fmt.Errorf(format, args...))
}

// errPathIndex + errorf
func errPathIndexf(idx int, format string, args ...any) error {
	return errPathIndex(idx, fmt.Errorf(format, args...))
}
