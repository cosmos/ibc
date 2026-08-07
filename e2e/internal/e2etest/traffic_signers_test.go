package e2etest

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

func TestSignersFormattingExposesOnlyAddresses(t *testing.T) {
	signers := NewSigners(t)
	addresses := signers.Addresses()
	privateKeys := []string{
		hex.EncodeToString(crypto.FromECDSA(signers.application.key)),
		hex.EncodeToString(crypto.FromECDSA(signers.relayer.key)),
	}

	for _, formatted := range []string{
		fmt.Sprintf("%v", signers),
		fmt.Sprintf("%+v", signers),
		fmt.Sprintf("%#v", signers),
	} {
		require.Contains(t, formatted, addresses.Application.Hex())
		require.Contains(t, formatted, addresses.Relayer.Hex())
		for _, privateKey := range privateKeys {
			require.NotContains(t, formatted, privateKey)
		}
	}
}
