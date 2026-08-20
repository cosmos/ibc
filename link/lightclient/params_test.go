// SPDX-License-Identifier: Apache-2.0

package lightclient

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type stubParams struct {
	ProverURL string `yaml:"proverUrl"`
}

// A misspelled params key must fail rather than silently becoming a zero
// value: the top-level decode's DisallowUnknownField cannot see inside a
// captured params block, so RawParams.Decode has to re-apply it.
func TestRawParamsRejectsUnknownField(t *testing.T) {
	params := NewRawParams([]byte("proverURL: https://example.com\n"))

	var p stubParams
	require.ErrorContains(t, params.Decode(&p), "proverURL")
	require.Empty(t, p.ProverURL)
}

func TestRawParamsRoundTrip(t *testing.T) {
	params := NewRawParams([]byte("proverUrl: https://example.com\n"))

	var p stubParams
	require.NoError(t, params.Decode(&p))
	require.Equal(t, "https://example.com", p.ProverURL)
}

func TestRawParamsEmptyDecodeIsNoOp(t *testing.T) {
	var params *RawParams

	var p stubParams
	require.NoError(t, params.Decode(&p))
	require.True(t, params.IsEmpty())
}
