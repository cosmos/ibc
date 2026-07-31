package e2e_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"

	relayerv2 "github.com/cosmos/ibc/link/api/v2/relayer"
)

// TestTransfer_RemoteAttestor delivers one transfer through a client whose
// attestor set is a single remote attestor process. The destination client
// only accepts attestations signed by that process's registered key, so
// delivery proves the relayer fetched them over gRPC rather than signing
// locally.
func TestTransfer_RemoteAttestor(t *testing.T) {
	t.Parallel()
	e2etest.RequireAnvilLane(t)

	const (
		instanceA  environment.IBCInstanceID = "ibc-chain-a"
		instanceB  environment.IBCInstanceID = "ibc-chain-b"
		clientA    environment.ClientID      = "conn-a-b-a"
		clientB    environment.ClientID      = "conn-a-b-b"
		attestorID environment.AttestorID    = "remote-attestor"
		deployer   environment.AuthorityID   = "deployer"
		signer     environment.AuthorityID   = "attestor-signer"
	)
	// The deployer must be the shared protocol authority: the block nudger
	// broadcasts from that identity, and instamine chains need its nudges for
	// packet heights to become final.
	const (
		deployerKeyHex = "0000000000000000000000000000000000000000000000000000000000000005"
		signerKeyHex   = "0000000000000000000000000000000000000000000000000000000000000001"
	)

	spec := environment.Spec{
		Chains: []environment.ChainSpec{
			environment.ManagedAnvil{ID: e2etest.ChainA, EVMChainID: 41537},
			environment.ManagedAnvil{ID: e2etest.ChainB, EVMChainID: 41538},
		},
		IBCInstances: []environment.IBCInstanceSpec{
			environment.NewIBCInstance{ID: instanceA, Chain: e2etest.ChainA, Authority: deployer},
			environment.NewIBCInstance{ID: instanceB, Chain: e2etest.ChainB, Authority: deployer},
		},
		Connections: []environment.ConnectionSpec{{
			ID: "conn-a-b",
			A: environment.NewClient{
				ID:                    clientA,
				IBCInstance:           instanceA,
				Authority:             deployer,
				MinRequiredSignatures: 1,
			},
			B: environment.DummyClient{ID: clientB, IBCInstance: instanceB, Authority: deployer},
		}},
		Attestors: []environment.AttestorSpec{{ID: attestorID, Client: clientA, Authority: signer}},
	}
	runtime := environment.Runtime{Authorities: map[environment.AuthorityID]environment.EVMAuthority{
		deployer: {PrivateKeyHex: deployerKeyHex},
		signer:   {PrivateKeyHex: signerKeyHex},
	}}

	env := e2etest.Start(t, e2etest.SuiteFor(spec, runtime))
	signers := e2etest.NewSigners(t)
	// Packets flow B to A so the mandatory receive leg lands on the attested
	// client: recv on chain A is verified by clientA against chain B state.
	route := e2etest.AtoB(e2etest.ChainB, e2etest.ChainA)

	attestor, err := env.Attestor(attestorID)
	require.NoError(t, err)
	require.NotEmpty(t, attestor.Endpoint())
	locator := string(attestor.IBCClient().Locator())

	driver, deployment := e2etest.DeployWithRelayerConfig(t, env, signers,
		func(cfg *ibclink.RelayerConfig) {
			set := &ibclink.RelayerAttestorSet{
				Threshold: 1,
				Attestors: []ibclink.RelayerAttestor{{
					Name: string(attestor.ID()),
					Type: ibclink.RelayerAttestorRemote,
					GRPC: attestor.Endpoint(),
				}},
			}
			// Match by client locator: connection ends are ordered
			// lexicographically by the harness, not by route direction.
			for i := range cfg.Connections {
				switch locator {
				case cfg.Connections[i].ClientA:
					cfg.Connections[i].AttestorSetA = set
				case cfg.Connections[i].ClientB:
					cfg.Connections[i].AttestorSetB = set
				}
			}
		}, route)
	transferApp := e2etest.BindTransfer(t, env, deployment, signers, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	transfer, err := transferApp.Send(ctx, e2etest.TransferRequest{Amount: big.NewInt(1_234_000)})
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyEscrowed(ctx))

	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	err = e2etest.AwaitState(ctx, relayer, transfer.Packet(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED, destination.Timing())
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(ctx))
}
