package harness

import (
	"context"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/harness/fixturekeys"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/harness/internal/onchain"
	"github.com/cosmos/ibc/link/harness/topology"
)

type preparedInputReader struct {
	onchain.Reader
	defaultPayload []byte
}

func (preparedInputReader) IFTBalance(context.Context, string) (*big.Int, error) {
	return new(big.Int), nil
}

func (preparedInputReader) GMPCount(context.Context, string) (*big.Int, error) {
	return new(big.Int), nil
}

func (r preparedInputReader) GMPDefaultPayload() []byte { return r.defaultPayload }

func (preparedInputReader) CanonicalAddr(addr string) (string, error) { return addr, nil }

func (preparedInputReader) Budget() onchain.Budget { return onchain.Budget{} }

func newPreparedInputSession(defaultPayload []byte) *Session {
	route := wire.Route{ID: "route", Source: "source", Destination: "destination"}
	rdr := preparedInputReader{defaultPayload: defaultPayload}
	readers := map[string]onchain.Reader{
		"source":      rdr,
		"destination": rdr,
	}
	return &Session{
		h: &Harness{topo: topology.Topology{Config: wire.ConfigYAML{
			Relayer: wire.Relayer{Routes: []wire.Route{route}},
		}}},
		deployment: &wire.Deployment{Chains: map[string]wire.ChainDeployment{
			"source": {Fixtures: map[string]string{fixturekeys.IFTFaucet: "holder"}},
		}},
		readers: readers,
		ift:     onchain.NewIFT(readers),
		gmp:     onchain.NewGMP(readers),
	}
}

func TestPrepareIFTOwnsAmount(t *testing.T) {
	session := newPreparedInputSession(nil)
	amount := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	want := new(big.Int).Set(amount)

	plan, err := session.PrepareIFT(t.Context(), IFT{
		Route:    "route",
		Amount:   amount,
		Receiver: "receiver",
	})
	require.NoError(t, err)

	amount.SetInt64(99)
	require.Equal(t, want, plan.in.Amount)
}

func TestPrepareIFTRejectsInvalidAmounts(t *testing.T) {
	session := newPreparedInputSession(nil)
	tooLarge := new(big.Int).Lsh(big.NewInt(1), 256)

	for _, amount := range []*big.Int{nil, new(big.Int), big.NewInt(-1), tooLarge} {
		_, err := session.PrepareIFT(t.Context(), IFT{Route: "route", Amount: amount, Receiver: "receiver"})
		require.Error(t, err)
	}
}

func TestPrepareGMPOwnsPayload(t *testing.T) {
	session := newPreparedInputSession(nil)
	payload := []byte{1, 2, 3}

	plan, err := session.PrepareGMP(t.Context(), GMP{
		Route:   "route",
		Target:  "target",
		Payload: payload,
	})
	require.NoError(t, err)

	payload[0] = 9
	require.Equal(t, []byte{1, 2, 3}, plan.in.Payload)
}

func TestPrepareGMPOwnsDefaultPayload(t *testing.T) {
	payload := []byte{1, 2, 3}
	session := newPreparedInputSession(payload)

	plan, err := session.PrepareGMP(t.Context(), GMP{Route: "route", Target: "target"})
	require.NoError(t, err)

	payload[0] = 9
	require.Equal(t, []byte{1, 2, 3}, plan.in.Payload)
}
