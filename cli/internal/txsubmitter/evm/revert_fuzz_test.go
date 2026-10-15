// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
)

// FuzzExplainRevert feeds node-supplied revert data to explainRevert, which
// must not panic or drop the original error.
func FuzzExplainRevert(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		original := revertError{code: 3, data: hexutil.Encode(data)}
		require.ErrorIs(t, explainRevert(original), original)
	})
}
