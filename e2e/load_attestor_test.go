// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"net/http"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	attestorv2 "github.com/cosmos/ibc/cli/api/v2/attestor"
	attestoribc "github.com/cosmos/ibc/cli/attestor/evm/ibc"
	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibccli"
	"github.com/cosmos/ibc/e2e/internal/loadtest"
)

// E2E_LOAD=true, E2E_LOAD=1, or -e2e.load required
// Note these tests are NOT parallel on purpose.

func TestLoad_AttestorBurst(t *testing.T) {
	// ARRANGE
	ts := newLoadTestAttestor(t)
	rpc := ts.attestorRPC()
	attestorName := string(ts.attestor.ID())

	// Given 10 delivered packets whose commitments are attestable at their send heights
	sends := ts.sendPackets(10)
	packets := make([]attestablePacket, 0, len(sends))
	for _, send := range sends {
		packets = append(packets, newAttestablePacket(t, send))
	}
	randomPacket := func() *attestablePacket {
		return &packets[rand.Intn(len(packets))]
	}

	// generates load for grpc.PacketAttestation()
	t.Run("packets", func(t *testing.T) {
		ctx := t.Context()

		// Given load test spec
		spec := loadtest.Spec{
			Duration:      30 * time.Second,
			RatePerSecond: 500,
			Concurrency:   32,
		}

		// Given load test call
		call := func(ctx context.Context) error {
			packet := randomPacket()

			req := connect.NewRequest(&attestorv2.PacketAttestationRequest{
				Attestor:       attestorName,
				Packets:        [][]byte{packet.encoded},
				Height:         packet.height,
				CommitmentType: attestorv2.CommitmentType_COMMITMENT_TYPE_PACKET,
			})

			res, err := rpc.PacketAttestation(ctx, req)
			switch {
			case err != nil:
				return err
			case res == nil || res.Msg.Attestation == nil:
				return fmt.Errorf("packet attestation: got nil response")
			case res.Msg.Attestation.Height != packet.height:
				// should not happen
				return fmt.Errorf("expected height %d, got %d", packet.height, res.Msg.Attestation.Height)
			default:
				return nil
			}
		}

		// ACT
		result := loadtest.SimpleLoad(ctx, spec, call)
		result.LogT(t)

		// ASSERT
		require.Positive(t, result.Succeeded)
	})

	// generates load for grpc.StateAttestation()
	t.Run("state", func(t *testing.T) {
		ctx := t.Context()

		// Given the attestable blocks 1..latest on the source chain
		latest, err := ts.attestor.LatestHeight(ctx)
		require.NoError(t, err)
		require.Positive(t, latest)

		randomHeight := func() uint64 {
			return uint64(rand.Intn(int(latest))) + 1
		}

		// Given load test spec
		spec := loadtest.Spec{
			Duration:      30 * time.Second,
			RatePerSecond: 100,
			Concurrency:   16,
		}

		// Given load test call
		call := func(ctx context.Context) error {
			height := randomHeight()

			req := connect.NewRequest(&attestorv2.StateAttestationRequest{
				Attestor: attestorName,
				Height:   height,
			})

			res, err := rpc.StateAttestation(ctx, req)
			switch {
			case err != nil:
				return err
			case res == nil || res.Msg.Attestation == nil:
				return fmt.Errorf("state attestation: got nil response")
			case res.Msg.Attestation.Height != height:
				// should not happen
				return fmt.Errorf("expected height %d, got %d", height, res.Msg.Attestation.Height)
			default:
				return nil
			}
		}

		// ACT
		result := loadtest.SimpleLoad(ctx, spec, call)
		result.LogT(t)

		// ASSERT
		require.Positive(t, result.Succeeded)
	})
}

type loadTestAttestor struct {
	t           *testing.T
	transferApp *e2etest.Transfer
	relayer     *ibccli.Relayer
	attestor    *environment.Attestor
}

type attestablePacket struct {
	height  uint64
	encoded []byte
}

func newLoadTestAttestor(t *testing.T) *loadTestAttestor {
	// ARRANGE
	// Given attested setup
	spec, runtime := attestedMesh(e2etest.EVMChains(t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB))
	env := e2etest.Start(t, spec, runtime)

	// Given signers
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)

	// Given relayer deployment with clearing enabled and two routes
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)

	driver, deployment := e2etest.Deploy(t, env, sender, relayerSigner, route)

	// Given deployed transfer apps for both routes
	transferApp := e2etest.NewTransfer(t, env, deployment, sender, route)

	// Given a relayer
	relayer := e2etest.StartRelayer(t, driver, env)

	var attestor *environment.Attestor
	for _, id := range env.Attestors() {
		candidate, err := env.Attestor(id)
		require.NoError(t, err)
		if candidate.ObservedIBCInstance().Chain().ID() == e2etest.ChainA {
			attestor = candidate
		}
	}
	require.NotNil(t, attestor, "no attestor observes %s", e2etest.ChainA)

	return &loadTestAttestor{
		t:           t,
		transferApp: transferApp,
		relayer:     relayer,
		attestor:    attestor,
	}
}

func (ts *loadTestAttestor) attestorRPC() attestorv2.AttestationServiceClient {
	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	transport := &http.Transport{Protocols: protocols}
	ts.t.Cleanup(transport.CloseIdleConnections)

	return attestorv2.NewAttestationServiceClient(
		&http.Client{Transport: transport},
		"http://"+ts.attestor.Endpoint(),
		connect.WithGRPC(),
	)
}

func (ts *loadTestAttestor) sendPackets(count int) []*e2etest.TransferSend {
	results := make([]*e2etest.TransferSend, 0, count)

	for i := 0; i < count; i++ {
		send := mustSend(ts.t, ts.transferApp, big.NewInt(100_000))
		results = append(results, send)
	}

	var wg sync.WaitGroup
	for _, send := range results {
		wg.Go(func() { mustDeliver(ts.t, ts.relayer, send) })
	}

	wg.Wait()

	return results
}

func newAttestablePacket(t *testing.T, send *e2etest.TransferSend) attestablePacket {
	t.Helper()

	filterer, err := ics26router.NewContractFilterer(common.Address{}, nil)
	require.NoError(t, err)

	tx := send.PacketTx()
	for _, log := range send.Receipt().Logs {
		event, err := filterer.ParseSendPacket(*log)
		if errors.Is(err, bind.ErrNoEventSignature) || errors.Is(err, bind.ErrEventSignatureMismatch) {
			continue
		}
		require.NoError(t, err)
		if event.Packet.SourceClient != tx.SourceClientID || event.Packet.Sequence != tx.Sequence {
			continue
		}

		encoded, err := attestoribc.EncodePacket(routerPacket(event.Packet))
		require.NoError(t, err)

		return attestablePacket{
			height:  tx.SourceBlockNumber,
			encoded: encoded,
		}
	}

	require.FailNowf(t, "no matching SendPacket event", "packet %s", tx)
	return attestablePacket{}
}

func routerPacket(p ics26router.IICS26RouterMsgsPacket) channeltypesv2.Packet {
	payloads := make([]channeltypesv2.Payload, len(p.Payloads))
	for i, item := range p.Payloads {
		payloads[i] = channeltypesv2.Payload{
			SourcePort:      item.SourcePort,
			DestinationPort: item.DestPort,
			Version:         item.Version,
			Encoding:        item.Encoding,
			Value:           item.Value,
		}
	}
	return channeltypesv2.Packet{
		Sequence:          p.Sequence,
		SourceClient:      p.SourceClient,
		DestinationClient: p.DestClient,
		TimeoutTimestamp:  p.TimeoutTimestamp,
		Payloads:          payloads,
	}
}
