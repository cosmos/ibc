// SPDX-License-Identifier: Apache-2.0

package v2

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPreparationValidate(t *testing.T) {
	for _, tc := range []struct {
		name        string
		preparation *Preparation
		count       int
		valid       bool
	}{
		{name: "nil"},
		{name: "empty", preparation: &Preparation{}},
		{name: "advance", preparation: &Preparation{Advance: []byte{1}}, count: 2, valid: true},
		{name: "both", preparation: &Preparation{Advance: []byte{1}, Ready: &BatchProofs{}}},
		{name: "ready without packets", preparation: &Preparation{Ready: &BatchProofs{}}, valid: true},
		{name: "ready", preparation: &Preparation{Ready: &BatchProofs{PacketProofs: [][]byte{{1}}}}, count: 1, valid: true},
		{name: "missing proof", preparation: &Preparation{Ready: &BatchProofs{}}, count: 1},
		{name: "extra proof", preparation: &Preparation{Ready: &BatchProofs{PacketProofs: [][]byte{{1}}}}},
		{name: "empty proof", preparation: &Preparation{Ready: &BatchProofs{PacketProofs: [][]byte{nil}}}, count: 1},
		{name: "checkpoint without update", preparation: &Preparation{Ready: &BatchProofs{Checkpoint: true}}},
		{name: "checkpoint", preparation: &Preparation{Ready: &BatchProofs{Update: []byte{1}, Checkpoint: true}}, valid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.preparation.Validate(tc.count)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
