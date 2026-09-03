// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrPath(t *testing.T) {
	leaf := errors.New("is invalid")

	t.Run("pathErrPathErrPathErr", func(t *testing.T) {
		// ARRANGE
		inner := errPath("error", leaf)
		mid := errPath("my", inner)

		// ACT
		err := errPath("server", mid)

		// ASSERT
		pe := requirePathError(t, err)
		assert.Equal(t, "server.my.error", pe.Path())
		assert.EqualError(t, err, "server.my.error: is invalid")
		assert.ErrorIs(t, err, leaf)
	})

	t.Run("pathErrPathErrJustAnErr", func(t *testing.T) {
		// ARRANGE
		mid := errPath("my", leaf)

		// ACT
		err := errPath("server", mid)

		// ASSERT
		pe := requirePathError(t, err)
		assert.Equal(t, "server.my", pe.Path())
		assert.EqualError(t, err, "server.my: is invalid")
		assert.ErrorIs(t, err, leaf)
	})

	t.Run("keepsWrapperAroundPathErr", func(t *testing.T) {
		// ARRANGE
		inner := errPath("listenAddr", leaf)
		wrapped := fmt.Errorf("loading: %w", inner)

		// ACT
		err := errPath("server", wrapped)

		// ASSERT
		pe := requirePathError(t, err)
		assert.Equal(t, "server", pe.Path())
		assert.EqualError(t, err, "server: loading: listenAddr: is invalid")
		assert.ErrorIs(t, err, leaf)
	})

	t.Run("joinsArrayIndex", func(t *testing.T) {
		// ARRANGE
		inner := errPath("[0]", leaf)
		mid := errPath("stuff", inner)

		// ACT
		err := errPath("server", mid)

		// ASSERT
		pe := requirePathError(t, err)
		assert.Equal(t, "server.stuff[0]", pe.Path())
		assert.EqualError(t, err, "server.stuff[0]: is invalid")
	})

	t.Run("returnsNilWhenErrIsNil", func(t *testing.T) {
		// ACT
		err := errPath("server", nil)

		// ASSERT
		assert.NoError(t, err)
	})

	t.Run("returnsErrUnchangedWhenSegmentEmpty", func(t *testing.T) {
		// ARRANGE
		inner := errPath("child", leaf)

		// ACT
		err := errPath("", inner)

		// ASSERT
		assert.Same(t, inner, err)
	})
}

func requirePathError(t *testing.T, err error) *PathError {
	t.Helper()

	var pe *PathError
	require.ErrorAs(t, err, &pe)
	return pe
}
