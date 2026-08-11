// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResourcePathTokenContainsNoAuthoredPathSegments(t *testing.T) {
	token := resourcePathToken("chain/../../outside")
	require.Len(t, token, 16)
	require.Equal(t, token, filepath.Base(token))
	require.NotContains(t, token, ".")
	require.NotContains(t, token, "/")
}
