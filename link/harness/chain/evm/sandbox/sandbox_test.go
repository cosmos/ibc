package sandbox

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/cosmos/ibc/link/harness/chain/evm"
)

// TestSandboxBech32 pins the funding path against sandboxd debug addr output: genesis funding targets these
// encodings of the two dev EOAs; if the prefix or byte source drifts, funding silently misses and the faucet
// reads empty in the sandbox lane.
func TestSandboxBech32(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"faucet", sandboxBech32Must(t, evm.FaucetAccount().Addr), "cosmos17w0adeg64ky0daxwd2ugyuneellmjgnxramjtq"},
		{"relayer", sandboxBech32Must(t, evm.RelayerAccount().Addr), "cosmos1wzvhjux9rqfdcwspp37srdgwp5tac7wgtkldlx"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Fatalf("%s sandboxBech32 = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}

func sandboxBech32Must(t *testing.T, addr common.Address) string {
	t.Helper()
	got, err := sandboxBech32(addr)
	if err != nil {
		t.Fatalf("sandboxBech32: %v", err)
	}
	return got
}
