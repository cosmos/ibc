// SPDX-License-Identifier: Apache-2.0

package e2etest

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

func TestSignerFormattingExposesOnlyAddress(t *testing.T) {
	signer := NewSigner(t)
	privateKey := hex.EncodeToString(crypto.FromECDSA(signer.key))

	for _, formatted := range []string{
		fmt.Sprintf("%v", signer),
		fmt.Sprintf("%+v", signer),
		fmt.Sprintf("%#v", signer),
	} {
		require.Contains(t, formatted, signer.Address().Hex())
		require.NotContains(t, formatted, privateKey)
	}
}
