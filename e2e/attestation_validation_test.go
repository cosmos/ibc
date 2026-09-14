// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"context"
	"crypto/ecdsa"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	attestorv2 "github.com/cosmos/ibc/cli/api/v2/attestor"
	relayerv2 "github.com/cosmos/ibc/cli/api/v2/relayer"
	attestorevm "github.com/cosmos/ibc/cli/attestor/evm"
	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibccli"
)

// commitmentCorruptingAttestor forwards real attestations, but can replace a
// commitment and sign the altered claim with the configured attestor's key.
type commitmentCorruptingAttestor struct {
	attestorv2.AttestationServiceClient
	key              *ecdsa.PrivateKey
	corrupt          atomic.Bool
	responses        atomic.Int64
	receiptResponses atomic.Int64
}

func (a *commitmentCorruptingAttestor) PacketAttestation(
	ctx context.Context,
	req *connect.Request[attestorv2.PacketAttestationRequest],
) (*connect.Response[attestorv2.PacketAttestationResponse], error) {
	res, err := a.AttestationServiceClient.PacketAttestation(ctx, req)
	if err != nil || !a.corrupt.Load() {
		return res, err
	}

	claim := res.Msg.GetAttestation()
	height, packets, err := attestorevm.DecodePacketAttestation(claim.GetAttestedData())
	if err != nil {
		return nil, err
	}
	if len(packets) == 0 {
		return nil, connect.NewError(connect.CodeInternal, os.ErrInvalid)
	}
	if req.Msg.GetCommitmentType() == attestorv2.CommitmentType_COMMITMENT_TYPE_RECEIPT {
		if packets[0].Commitment != ([32]byte{}) {
			return nil, connect.NewError(connect.CodeInternal, os.ErrInvalid)
		}
		a.receiptResponses.Add(1)
	}
	packets[0].Commitment[0] ^= 1
	claim.AttestedData, err = attestorevm.EncodePacketAttestation(height, packets)
	if err != nil {
		return nil, err
	}
	digest := attestorevm.Digest(attestorevm.TagPacketAttestation, claim.AttestedData)
	claim.Signature, err = crypto.Sign(digest[:], a.key)
	if err != nil {
		return nil, err
	}
	a.responses.Add(1)
	return res, nil
}

func TestAttestation_RejectsWrongCommitment(t *testing.T) {
	for _, phase := range []string{"packet", "acknowledgement", "timeout"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			spec, runtime := attestedMesh(
				e2etest.EVMChains(
					t,
					e2etest.EVMRequirements{ControlledMining: phase == "timeout"},
					e2etest.ChainA,
					e2etest.ChainB,
				),
			)
			route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
			id := meshAttestorFor(route.Destination, route.Source)
			if phase != "packet" {
				id = meshAttestorFor(route.Source, route.Destination)
			}
			env := e2etest.Start(t, spec, runtime)
			backing, err := env.Attestor(id)
			require.NoError(t, err)
			key, err := crypto.HexToECDSA(runtime.Authorities[environment.AuthorityID(id)].PrivateKeyHex)
			require.NoError(t, err)
			require.Equal(t, string(backing.SignerAddress()), crypto.PubkeyToAddress(key.PublicKey).Hex())
			require.Equal(t, uint8(1), backing.IBCClient().MinRequiredSignatures())

			protocols := new(http.Protocols)
			protocols.SetHTTP1(true)
			protocols.SetUnencryptedHTTP2(true)
			transport := &http.Transport{Protocols: protocols}
			t.Cleanup(transport.CloseIdleConnections)
			proxy := &commitmentCorruptingAttestor{
				AttestationServiceClient: attestorv2.NewAttestationServiceClient(
					&http.Client{Transport: transport, Timeout: 5 * time.Second},
					"http://"+backing.Endpoint(), connect.WithGRPC(),
				),
				key: key,
			}
			proxy.corrupt.Store(true)
			_, handler := attestorv2.NewAttestationServiceHandler(proxy)
			server := httptest.NewUnstartedServer(handler)
			server.Config.Protocols = protocols
			server.Start()
			t.Cleanup(server.Close)

			var logPath string
			sender := e2etest.NewSigner(t)
			driver, deployment := e2etest.DeployWithRelayerConfig(t, env, sender, e2etest.NewSigner(t),
				func(cfg *ibccli.RelayerConfig) {
					logPath = filepath.Join(filepath.Dir(cfg.DBPath), "relayer.log")
					for i := range cfg.Attestors {
						if cfg.Attestors[i].Name == string(id) {
							cfg.Attestors[i].GRPC = strings.TrimPrefix(server.URL, "http://")
						}
					}
				}, route)
			app := e2etest.NewTransfer(t, env, deployment, sender, route)
			relayer := e2etest.StartRelayer(t, driver, env)
			ctx := t.Context()
			request := e2etest.TransferRequest{Amount: big.NewInt(1_234_000)}
			if phase == "timeout" {
				require.NoError(t, relayer.Stop(ctx))
				request.Timeout = packetTimeout
			}
			transfer, err := app.Send(ctx, request)
			require.NoError(t, err)
			if phase == "timeout" {
				chain, chainErr := env.Chain(route.Destination)
				require.NoError(t, chainErr)
				mining, miningErr := chain.Mining()
				require.NoError(t, miningErr)
				require.NoError(t, mining.AdvanceTime(ctx, packetTimeoutAdvance))
				relayer = e2etest.StartRelayer(t, driver, env)
				require.NoError(t, e2etest.RelayAll(ctx, relayer, transfer.PacketTx()))
			}

			// Pending alone is insufficient: the on-chain verifier could reject a
			// bad proof too. Require the aggregator's error before restoring service.
			require.Eventually(t, func() bool {
				logs, readErr := os.ReadFile(logPath)
				return readErr == nil && strings.Contains(string(logs), "attested data does not match expected claim")
			}, 30*time.Second, 100*time.Millisecond, "relayer must reject the signed commitment before submission")
			require.Positive(t, proxy.responses.Load())
			require.NoError(
				t,
				e2etest.AwaitStable(ctx, relayer, transfer.PacketTx(), relayerv2.PacketState_PACKET_STATE_PENDING),
			)

			if phase == "timeout" {
				require.Positive(t, proxy.receiptResponses.Load(), "must corrupt a zero receipt-absence commitment")
				require.NoError(t, transfer.VerifyNotMinted(ctx))
			}

			proxy.corrupt.Store(false)
			want := relayerv2.PacketState_PACKET_STATE_SUCCEEDED
			if phase == "timeout" {
				want = relayerv2.PacketState_PACKET_STATE_TIMED_OUT
			}
			status, err := e2etest.AwaitState(
				ctx,
				relayer,
				transfer.PacketTx(),
				want,
			)
			require.NoError(t, err)
			require.NoError(t, transfer.VerifyCommitmentCleared(ctx))
			if phase == "timeout" {
				require.NoError(t, transfer.VerifyRefunded(ctx, status.GetTimeoutTx().GetTxHash()))
				require.NoError(t, transfer.VerifyNotMinted(ctx))
			} else {
				require.NoError(t, transfer.VerifyDelivered(ctx))
				require.NoError(t, transfer.VerifyAcknowledgementExecuted(ctx, status.GetAckTx().GetTxHash()))
			}
		})
	}
}
