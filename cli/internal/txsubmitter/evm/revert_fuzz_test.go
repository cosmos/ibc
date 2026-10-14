// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

// FuzzExplainRevert feeds node-supplied revert data to explainRevert, which
// must never lose the original error.
func FuzzExplainRevert(f *testing.F) {
	f.Add([]byte{})
	for _, metadata := range revertContracts {
		contractABI, err := metadata.GetAbi()
		if err != nil {
			f.Fatal(err)
		}
		for _, customErr := range contractABI.Errors {
			f.Add(customErr.ID[:4])
		}
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		original := revertError{code: 3, data: hexutil.Encode(data)}
		if err := explainRevert(original); !errors.Is(err, original) {
			t.Fatalf("explained error %v does not wrap the original", err)
		}
	})
}
