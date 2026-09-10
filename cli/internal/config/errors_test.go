// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"testing"

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
		require.Equal(t, "server.my.error", pe.Path())
		require.EqualError(t, err, "server.my.error: is invalid")
		require.ErrorIs(t, err, leaf)
	})

	t.Run("pathErrPathErrJustAnErr", func(t *testing.T) {
		// ARRANGE
		mid := errPath("my", leaf)

		// ACT
		err := errPath("server", mid)

		// ASSERT
		pe := requirePathError(t, err)
		require.Equal(t, "server.my", pe.Path())
		require.EqualError(t, err, "server.my: is invalid")
		require.ErrorIs(t, err, leaf)
	})

	t.Run("joinsArrayIndex", func(t *testing.T) {
		// ARRANGE
		inner := errPath("[0]", leaf)
		mid := errPath("stuff", inner)

		// ACT
		err := errPath("server", mid)

		// ASSERT
		pe := requirePathError(t, err)
		require.Equal(t, "server.stuff[0]", pe.Path())
		require.EqualError(t, err, "server.stuff[0]: is invalid")
	})

	t.Run("returnsNilWhenErrIsNil", func(t *testing.T) {
		// ACT
		err := errPath("server", nil)

		// ASSERT
		require.NoError(t, err)
	})

	t.Run("returnsErrUnchangedWhenSegmentEmpty", func(t *testing.T) {
		// ARRANGE
		inner := errPath("child", leaf)

		// ACT
		err := errPath("", inner)

		// ASSERT
		require.Same(t, inner, err)
	})
}

func requirePathError(t *testing.T, err error) *PathError {
	t.Helper()

	var pe *PathError
	require.ErrorAs(t, err, &pe)
	return pe
}
