// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
)

// FuzzExplainRevert feeds node-supplied revert data to ExplainRevert, which
// must not panic or drop the original error.
func FuzzExplainRevert(f *testing.F) {
	for _, tc := range namedReverts {
		f.Add(hexutil.MustDecode(revertData(f, tc.metadata, tc.name, tc.args...)))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		original := revertError{code: 3, data: hexutil.Encode(data)}
		require.ErrorIs(t, ExplainRevert(original), original)
	})
}
