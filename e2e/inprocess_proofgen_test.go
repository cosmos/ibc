// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"
	relayerv2 "github.com/cosmos/ibc/link/api/v2/relayer"
	"github.com/cosmos/ibc/link/app"
	"github.com/cosmos/ibc/link/keyfile"
	"github.com/cosmos/ibc/link/proofgen"
)

// TestInProcessRelayerWithRegisteredProofGenerator is a proof of concept for
// the pluggable proof generator work. It proves three things the existing
// suite cannot:
//
//  1. A relayer can be constructed and run entirely in-process through the
//     public link/app entrypoint, rather than by executing the ibc binary.
//     The harness daemon shells out, so there is no way to hand it a registry.
//  2. Attestors run inside that same process (dual mode), with no separate
//     attestor daemons.
//  3. A caller-registered light client type reaches proof generation. The
//     attestation client is resolved through the same registry, so relaying
//     working at all is evidence the switch-to-registry port is sound.
//
// It deliberately does not use e2etest.StartRelayer or AwaitState, both of
// which require the daemon handle. No harness files are modified.
func TestInProcessRelayerWithRegisteredProofGenerator(t *testing.T) {
	t.Parallel()

	chainSpecs := e2etest.EVMChains(t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB)
	spec, runtime := attestedMesh(chainSpecs)
	env := e2etest.Start(t, spec, runtime)

	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)

	// Capture the config the harness builds, rewriting its attestors from
	// separate gRPC daemons to in-process local ones backed by the same keys
	// the environment baked into each client's on-chain attestation set.
	var captured ibclink.RelayerConfig

	keyDir := t.TempDir()

	driver, deployment := e2etest.DeployWithRelayerConfig(
		t,
		env,
		sender,
		relayerSigner,
		func(config *ibclink.RelayerConfig) {
			config.Attestors = localAttestors(t, env, runtime, keyDir)
			captured = *config
		},
		route,
	)
	require.NotNil(t, driver)

	// The harness writes chain RPCs as ${IBC_LINK_CHAIN_RPC_...} placeholders
	// and injects the values into the environment of the subprocess it spawns.
	// Running in-process there is no such subprocess, so substitute the real
	// endpoints.
	for i := range captured.Chains {
		captured.Chains[i].RPC = rpcURLForEVMChainID(t, env, captured.Chains[i].ChainID)
	}

	// The in-process relayer gets its own config file and database so it never
	// contends with the harness driver's.
	runDir := t.TempDir()
	captured.DBPath = filepath.Join(runDir, "inprocess.db")
	configPath := filepath.Join(runDir, "ibc-link.config.yaml")
	require.NoError(t, ibclink.WriteRelayerConfig(configPath, captured), "write in-process config")

	// A client type that exists only in this test, proving the registry
	// accepts arbitrary names from outside the relayer's own packages.
	registry := proofgen.NewRegistry()
	require.NoError(t, registry.Register("poc-custom-client", pocFactory{}))

	relayer, err := app.New(t.Context(), app.Options{
		ConfigPath: configPath,
		Registry:   registry,
	})
	require.NoError(t, err, "construct relayer in-process")

	address, err := relayer.Start(t.Context())
	require.NoError(t, err, "start relayer")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		require.NoError(t, relayer.Stop(ctx), "stop relayer")
	})

	require.ElementsMatch(t, evmChainIDs(t, env), relayer.ChainIDs(), "relayer connected to both chains")

	client := relayerv2.NewRelayerApiServiceClient(
		&http.Client{Timeout: 10 * time.Second},
		"http://"+address.String(),
		connect.WithGRPC(),
	)

	ctx := t.Context()

	iftApp := e2etest.NewIFT(t, env, deployment, sender, route)
	transfer, err := iftApp.Send(ctx, e2etest.IFTRequest{Amount: big.NewInt(1_234_000)})
	require.NoError(t, err, "send transfer")

	packet := transfer.PacketTx()
	sourceChainID := evmChainID(t, env, packet.Source)

	_, err = client.Relay(ctx, connect.NewRequest(&relayerv2.RelayRequest{
		TxHash:        packet.SourceTxHash,
		SourceChainId: sourceChainID,
		Selection:     &relayerv2.RelayRequest_AllPackets{AllPackets: &relayerv2.AllPackets{}},
	}))
	require.NoError(t, err, "trigger relay")

	status := awaitSucceeded(ctx, t, client, packet.SourceTxHash, sourceChainID)
	require.NotEmpty(t, status.GetRecvTx().GetTxHash(), "recv tx recorded")
	require.NotEmpty(t, status.GetAckTx().GetTxHash(), "ack tx recorded")

	require.NoError(t, transfer.VerifyDelivered(ctx))
	require.NoError(t, transfer.VerifyBurned(ctx), "a successful ack must not also refund")
}

// awaitSucceeded polls the relayer's own status API until the packet reports
// success. A local reimplementation of e2etest.AwaitState, which needs the
// daemon handle this test does not have.
func awaitSucceeded(
	ctx context.Context,
	t *testing.T,
	client relayerv2.RelayerApiServiceClient,
	txHash, sourceChainID string,
) *relayerv2.PacketStatus {
	t.Helper()

	deadline := time.Now().Add(90 * time.Second)

	var last relayerv2.PacketState

	for time.Now().Before(deadline) {
		res, err := client.Status(ctx, connect.NewRequest(&relayerv2.StatusRequest{
			TxHash: txHash, SourceChainId: sourceChainID,
		}))
		if err == nil {
			for _, packet := range res.Msg.GetPacketStatuses() {
				last = packet.GetState()
				if last == relayerv2.PacketState_PACKET_STATE_SUCCEEDED {
					return packet
				}

				require.NotEqual(
					t,
					relayerv2.PacketState_PACKET_STATE_RELAY_FAILED,
					last,
					"packet permanently failed to relay",
				)
			}
		}

		select {
		case <-ctx.Done():
			t.Fatalf("context cancelled awaiting relay: %v", ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}

	t.Fatalf("packet did not succeed within budget; last observed state %v", last)

	return nil
}

// localAttestors converts the environment's attestors into in-process entries.
// Each attestor watches the chain on the far side of the client it backs, and
// signs with the authority key the environment registered in that client's
// on-chain attestation set.
func localAttestors(
	t *testing.T,
	env *environment.Environment,
	runtime environment.Runtime,
	keyDir string,
) []ibclink.RelayerAttestor {
	t.Helper()

	chains := env.Chains()
	require.Len(t, chains, 2, "poc assumes a two chain mesh")

	attestors := make([]ibclink.RelayerAttestor, 0, 2)

	for _, host := range chains {
		counterparty := chains[0]
		if counterparty == host {
			counterparty = chains[1]
		}

		// The attestor backing the client on host watches counterparty.
		id := meshAttestorFor(host, counterparty)
		authority, ok := runtime.Authorities[environment.AuthorityID(id)]
		require.True(t, ok, "no authority for attestor %q", id)

		attestors = append(attestors, ibclink.RelayerAttestor{
			Name:    string(id),
			Type:    ibclink.RelayerAttestorLocal,
			ChainID: evmChainID(t, env, counterparty),
			KeyFile: writeKeyFile(t, keyDir, string(id), authority.PrivateKeyHex),
		})
	}

	return attestors
}

func writeKeyFile(t *testing.T, dir, name, privateKeyHex string) string {
	t.Helper()

	require.NoError(t, os.MkdirAll(dir, 0o700))

	key, err := crypto.HexToECDSA(privateKeyHex)
	require.NoError(t, err, "parse attestor key %q", name)

	path := filepath.Join(dir, name+".json")
	require.NoError(t, keyfile.Store(path, keyfile.ECDSA, crypto.FromECDSA(key)), "store attestor key")

	return path
}

// rpcURLForEVMChainID resolves the live RPC endpoint of the chain whose EVM
// chain id matches evmID.
func rpcURLForEVMChainID(t *testing.T, env *environment.Environment, evmID string) string {
	t.Helper()

	for _, id := range env.Chains() {
		chain, err := env.Chain(id)
		require.NoError(t, err, "resolve chain %q", id)

		if strconv.FormatUint(chain.EVMChainID(), 10) == evmID {
			return chain.RPCURL()
		}
	}

	t.Fatalf("no chain with EVM chain id %s", evmID)

	return ""
}

func evmChainID(t *testing.T, env *environment.Environment, id environment.ChainID) string {
	t.Helper()

	chain, err := env.Chain(id)
	require.NoError(t, err, "resolve chain %q", id)

	return strconv.FormatUint(chain.EVMChainID(), 10)
}

func evmChainIDs(t *testing.T, env *environment.Environment) []string {
	t.Helper()

	ids := make([]string, 0, len(env.Chains()))
	for _, id := range env.Chains() {
		ids = append(ids, evmChainID(t, env, id))
	}

	return ids
}

// pocFactory is a light client type defined entirely outside the relayer,
// registered only through app.Options.Registry. It is never selected by this
// test's config; its presence proves an arbitrary type name is accepted by
// config validation and generator resolution.
type pocFactory struct{}

type pocParams struct {
	ProverURL string `yaml:"proverUrl"`
}

func (pocFactory) ValidateParams(params *proofgen.RawParams) error {
	var p pocParams

	return params.Decode(&p)
}

func (pocFactory) New(
	context.Context, proofgen.Deps, proofgen.ClientEnd, proofgen.ClientEnd,
) (proofgen.ProofGenerator, error) {
	return nil, fmt.Errorf("poc-custom-client generates no proofs")
}
