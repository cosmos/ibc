// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"context"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"
	relayerv2 "github.com/cosmos/ibc/link/api/v2/relayer"
)

// packetsClient dials the relayer directly; the harness helpers wrap a
// different call shape.
func packetsClient(t *testing.T, relayer *ibclink.Relayer) relayerv2.RelayerApiServiceClient {
	t.Helper()

	return relayerv2.NewRelayerApiServiceClient(
		&http.Client{Timeout: 10 * time.Second},
		"http://"+relayer.Ready().HTTP,
		connect.WithGRPC(),
	)
}

func listPackets(
	ctx context.Context,
	t *testing.T,
	client relayerv2.RelayerApiServiceClient,
	filter *relayerv2.PacketFilter,
) *relayerv2.PacketsResponse {
	t.Helper()

	res, err := client.Packets(ctx, connect.NewRequest(&relayerv2.PacketsRequest{Filter: filter}))
	require.NoError(t, err, "packets request")

	return res.Msg
}

func sourceTxHashes(res *relayerv2.PacketsResponse) []string {
	hashes := make([]string, 0, len(res.GetPackets()))
	for _, packet := range res.GetPackets() {
		hashes = append(hashes, packet.GetSendTx().GetTxHash())
	}

	return hashes
}

func wireChainID(t *testing.T, env *environment.Environment, id environment.ChainID) string {
	t.Helper()

	chain, err := env.Chain(id)
	require.NoError(t, err, "resolve chain %q", id)

	return strconv.FormatUint(chain.EVMChainID(), 10)
}

func ptr[T any](v T) *T { return &v }

// Two routes sharing a destination but differing in source chain and client: a
// silently ignored filter still looks correct against a single-route fixture.
func TestPacketsFiltersDiscriminate(t *testing.T) {
	t.Parallel()

	const (
		chainA environment.ChainID = "chain-a"
		chainB environment.ChainID = "chain-b"
		chainC environment.ChainID = "chain-c"
	)

	spec, runtime := attestedMesh(e2etest.EVMChains(t, e2etest.EVMRequirements{}, chainA, chainB, chainC))
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	routes := []e2etest.Route{
		{ID: "b-to-a", Source: chainB, Destination: chainA},
		{ID: "c-to-a", Source: chainC, Destination: chainA},
	}
	driver, deployment := e2etest.Deploy(t, env, sender, relayerSigner, routes...)
	bToAApp := e2etest.NewTransfer(t, env, deployment, sender, routes[0])
	cToAApp := e2etest.NewTransfer(t, env, deployment, sender, routes[1])
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	bToA, err := bToAApp.Send(ctx, e2etest.TransferRequest{Amount: big.NewInt(333)})
	require.NoError(t, err)
	cToA, err := cToAApp.Send(ctx, e2etest.TransferRequest{Amount: big.NewInt(444)})
	require.NoError(t, err)

	for _, transfer := range []*e2etest.TransferSend{bToA, cToA} {
		_, awaitErr := e2etest.AwaitState(ctx, relayer, transfer.PacketTx(),
			relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
		require.NoError(t, awaitErr)
	}

	client := packetsClient(t, relayer)
	bHash, cHash := bToA.PacketTx().SourceTxHash, cToA.PacketTx().SourceTxHash

	t.Run("unfilteredReturnsBoth", func(t *testing.T) {
		hashes := sourceTxHashes(listPackets(ctx, t, client, nil))
		require.Subset(t, hashes, []string{bHash, cHash})
	})

	t.Run("bySourceChain", func(t *testing.T) {
		res := listPackets(ctx, t, client, &relayerv2.PacketFilter{
			SourceChainId: ptr(wireChainID(t, env, chainB)),
		})
		require.Equal(t, []string{bHash}, sourceTxHashes(res))
		require.False(t, res.GetHasMore())
	})

	t.Run("bySourceClient", func(t *testing.T) {
		res := listPackets(ctx, t, client, &relayerv2.PacketFilter{
			SourceClientId: ptr(cToA.PacketTx().SourceClientID),
		})
		require.Equal(t, []string{cHash}, sourceTxHashes(res))
	})

	t.Run("bySourceTxHash", func(t *testing.T) {
		res := listPackets(ctx, t, client, &relayerv2.PacketFilter{SourceTxHash: ptr(bHash)})
		require.Equal(t, []string{bHash}, sourceTxHashes(res))
	})

	t.Run("txHashIsCaseInsensitive", func(t *testing.T) {
		// An uppercase hash must normalize, not silently match nothing.
		upper := "0x" + strings.ToUpper(strings.TrimPrefix(bHash, "0x"))
		res := listPackets(ctx, t, client, &relayerv2.PacketFilter{SourceTxHash: ptr(upper)})
		require.Equal(t, []string{bHash}, sourceTxHashes(res))
	})

	t.Run("filtersCombineAsAnd", func(t *testing.T) {
		// chain B holds no packet from route c-to-a's client.
		res := listPackets(ctx, t, client, &relayerv2.PacketFilter{
			SourceChainId:  ptr(wireChainID(t, env, chainB)),
			SourceClientId: ptr(cToA.PacketTx().SourceClientID),
		})
		require.Empty(t, res.GetPackets())
		require.False(t, res.GetHasMore())
	})

	t.Run("unknownTxHashIsEmptyNotError", func(t *testing.T) {
		res := listPackets(ctx, t, client, &relayerv2.PacketFilter{
			SourceTxHash: ptr("0x" + strings.Repeat("ab", 32)),
		})
		require.Empty(t, res.GetPackets())
		require.False(t, res.GetHasMore())
	})

	// An unconfigured chain names configuration the relayer does not have, so
	// it is a caller error rather than a filter matching nothing.
	t.Run("unconfiguredChainIsRejected", func(t *testing.T) {
		_, err := client.Packets(ctx, connect.NewRequest(&relayerv2.PacketsRequest{
			Filter: &relayerv2.PacketFilter{SourceChainId: ptr("99999")},
		}))
		require.Error(t, err)
		require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})

	t.Run("terminalStateFilter", func(t *testing.T) {
		res := listPackets(ctx, t, client, &relayerv2.PacketFilter{
			State: relayerv2.PacketState_PACKET_STATE_SUCCEEDED.Enum(),
		})
		require.Subset(t, sourceTxHashes(res), []string{bHash, cHash})

		none := listPackets(ctx, t, client, &relayerv2.PacketFilter{
			State: relayerv2.PacketState_PACKET_STATE_RELAY_FAILED.Enum(),
		})
		require.Empty(t, none.GetPackets())
	})

	t.Run("pagingCoversEveryPacketOnce", func(t *testing.T) {
		full := listPackets(ctx, t, client, nil)
		total := len(full.GetPackets())
		require.GreaterOrEqual(t, total, 2)
		require.False(t, full.GetHasMore(), "an unpaged listing has nothing beyond it")

		seen := map[string]int{}
		cursor := ""

		for page := 0; page < total; page++ {
			res, err := client.Packets(ctx, connect.NewRequest(&relayerv2.PacketsRequest{
				Limit: 1, Cursor: cursor,
			}))
			require.NoError(t, err)
			require.Len(t, res.Msg.GetPackets(), 1)

			// has_more is true for every page but the last.
			require.Equalf(t, page < total-1, res.Msg.GetHasMore(),
				"has_more at page %d of %d", page, total)

			if res.Msg.GetHasMore() {
				require.NotEmpty(t, res.Msg.GetNextCursor(), "has_more must come with a cursor")
			} else {
				require.Empty(t, res.Msg.GetNextCursor(), "a final page must not offer a cursor")
			}

			seen[res.Msg.GetPackets()[0].GetSendTx().GetTxHash()]++
			cursor = res.Msg.GetNextCursor()
		}

		for hash, count := range seen {
			require.Equalf(t, 1, count, "packet %s appeared %d times across pages", hash, count)
		}
	})

	t.Run("rejectsMalformedCursor", func(t *testing.T) {
		_, err := client.Packets(ctx, connect.NewRequest(&relayerv2.PacketsRequest{
			Cursor: "not-a-cursor!",
		}))
		require.Error(t, err)
		require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})
}

// Wire-level guard for the state expansion: a packet held mid-pipeline sits in
// an intermediate status, not the literal PENDING one, and must still be
// returned by a PENDING filter.
func TestPacketsStateFilterCoversInFlightStatuses(t *testing.T) {
	t.Parallel()

	spec, runtime := attestedMesh(e2etest.EVMChains(t,
		e2etest.EVMRequirements{ControlledMining: true}, e2etest.ChainA, e2etest.ChainB))
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, sender, relayerSigner, route)
	transferApp := e2etest.NewTransfer(t, env, deployment, sender, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	chainB, err := env.Chain(e2etest.ChainB)
	require.NoError(t, err)
	mining, err := chainB.Mining()
	require.NoError(t, err)

	client := packetsClient(t, relayer)

	var transfer *e2etest.TransferSend

	require.NoError(t, mining.WithPaused(ctx, func() error {
		transfer, err = transferApp.Send(ctx, e2etest.TransferRequest{Amount: big.NewInt(424_242)})
		require.NoError(t, err)

		require.NoError(t, e2etest.AwaitStable(ctx, relayer, transfer.PacketTx(),
			relayerv2.PacketState_PACKET_STATE_PENDING))

		hash := transfer.PacketTx().SourceTxHash

		pending := listPackets(ctx, t, client, &relayerv2.PacketFilter{
			State: relayerv2.PacketState_PACKET_STATE_PENDING.Enum(),
		})
		require.Contains(t, sourceTxHashes(pending), hash,
			"an in-flight packet must be returned by a PENDING filter")

		succeeded := listPackets(ctx, t, client, &relayerv2.PacketFilter{
			State: relayerv2.PacketState_PACKET_STATE_SUCCEEDED.Enum(),
		})
		require.NotContains(t, sourceTxHashes(succeeded), hash,
			"an in-flight packet must not be reported as succeeded")

		return nil
	}))

	_, err = e2etest.AwaitState(ctx, relayer, transfer.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)

	hash := transfer.PacketTx().SourceTxHash

	// The same packet moves between filters as it completes.
	require.Contains(t, sourceTxHashes(listPackets(ctx, t, client, &relayerv2.PacketFilter{
		State: relayerv2.PacketState_PACKET_STATE_SUCCEEDED.Enum(),
	})), hash)
	require.NotContains(t, sourceTxHashes(listPackets(ctx, t, client, &relayerv2.PacketFilter{
		State: relayerv2.PacketState_PACKET_STATE_PENDING.Enum(),
	})), hash)
}
