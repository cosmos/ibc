// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPathError(t *testing.T) {
	t.Run("joinsNestedFields", func(t *testing.T) {
		// ARRANGE
		err := errPath("server", errPath("my", errPath("error", errors.New("is invalid"))))

		// ACT
		var pathErr *PathError
		ok := errors.As(err, &pathErr)

		// ASSERT
		require.True(t, ok)
		assert.Equal(t, "server.my.error", pathErr.Path())
		assert.EqualError(t, err, "server.my.error: is invalid")
	})

	t.Run("joinsArrayIndex", func(t *testing.T) {
		// ARRANGE
		err := errPath("server", errPath("stuff", errPath("[0]", errors.New("is invalid"))))

		// ACT
		var pathErr *PathError
		ok := errors.As(err, &pathErr)

		// ASSERT
		require.True(t, ok)
		assert.Equal(t, "server.stuff[0]", pathErr.Path())
		assert.EqualError(t, err, "server.stuff[0]: is invalid")
	})
}
