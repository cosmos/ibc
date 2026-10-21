// SPDX-License-Identifier: Apache-2.0

// Package besufixtures_test regenerates the Besu light-client fixtures from
// chains the e2e harness boots. Run it with make -C e2e generate-besu-fixtures;
// it skips unless BESU_FIXTURES_DIR names the directory to write to.
package besufixtures_test

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient/gethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc-contracts/packages/go-abigen/besumsgs"
	"github.com/cosmos/ibc-contracts/packages/go-abigen/ics26router"
	hostv2 "github.com/cosmos/ibc-go/v11/modules/core/24-host/v2"
	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/cli/besu/besutest"
	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
)

const (
	fixturesDirEnv = "BESU_FIXTURES_DIR"

	// The fixture tests deploy their clients with these, and warp time
	// relative to them.
	fixtureTrustingPeriod = 1_209_600
	fixtureMaxClockDrift  = 15
)

// TestGenerateBesuFixtures writes <consensus>.json for every consensus: a
// transfer's packet proofs and the headers around it on the sending chain,
// plus synthetic updates the client rejects.
func TestGenerateBesuFixtures(t *testing.T) {
	dir := os.Getenv(fixturesDirEnv)
	if dir == "" {
		t.Skipf("set %s to the directory to write the fixtures to", fixturesDirEnv)
	}
	for _, consensus := range besu.Consensuses {
		t.Run(string(consensus), func(t *testing.T) {
			t.Parallel()
			data, err := json.MarshalIndent(generate(t, consensus), "", "  ")
			require.NoError(t, err)
			path := filepath.Join(dir, string(consensus)+".json")
			require.NoError(t, os.WriteFile(path, append(data, '\n'), 0o644))
		})
	}
}

type generator struct {
	t       *testing.T
	ctx     context.Context
	source  *environment.EVM
	headers e2etest.BesuHeaders
	rpc     *rpc.Client
	router  common.Address
}

// generate sends a transfer from A to B and captures the fixture from A: the
// trusted header two below the send, an adjacent and a non-adjacent update to
// the send, and both proofs at the send. Synthetic updates reseal the headers
// at and after the send.
func generate(t *testing.T, consensus besu.Consensus) besutest.Fixture {
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	spec, runtime := e2etest.BesuMesh(e2etest.BesuChains(t, consensus, consensus))
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	_, deployment := e2etest.Deploy(t, env, sender, e2etest.NewSigner(t), route)
	transferApp := e2etest.NewTransfer(t, env, deployment, sender, route)
	ctx := t.Context()

	transfer, err := transferApp.Send(ctx, e2etest.TransferRequest{Amount: big.NewInt(1_000_000)})
	require.NoError(t, err)

	source, err := env.Chain(route.Source)
	require.NoError(t, err)
	instance, err := env.IBCInstanceForChain(route.Source)
	require.NoError(t, err)
	g := generator{
		t:       t,
		ctx:     ctx,
		headers: e2etest.NewBesuHeaders(t, env, route.Source, consensus),
		router:  common.HexToAddress(string(instance.Locator())),
	}
	g.source, err = source.EVM()
	require.NoError(t, err)
	g.rpc, err = rpc.DialContext(ctx, source.RPCURL())
	require.NoError(t, err)
	t.Cleanup(g.rpc.Close)

	clients, ok := deployment.RouteClients(route.ID)
	require.True(t, ok)
	packetTx := transfer.PacketTx()
	send := packetTx.SourceBlockNumber
	require.Greater(t, send, uint64(2))

	trusted := g.headers.Sealed(send - 2)
	nonAdjacent := g.headers.Sealed(send)
	commitmentPath := hostv2.PacketCommitmentKey(clients.SourceClientID, packetTx.Sequence)
	return besutest.Fixture{
		RouterAddress:            g.router,
		InitialTrustedHeight:     trusted.Height,
		InitialTrustedTimestamp:  trusted.Timestamp,
		InitialTrustedStateRoot:  trusted.StateRoot,
		InitialTrustedValidators: trusted.Validators,
		TrustingPeriod:           fixtureTrustingPeriod,
		MaxClockDrift:            fixtureMaxClockDrift,
		AdjacentUpdate:           update(trusted, g.headers.Sealed(send-1)),
		NonAdjacentUpdate:        update(trusted, nonAdjacent),
		LowQuorumUpdate:          g.lowQuorumUpdate(trusted, nonAdjacent),
		ConflictingUpdate:        g.conflictingUpdate(trusted, nonAdjacent),
		LowOverlapUpdate:         g.lowOverlapUpdate(trusted, send+1),
		Membership:               g.proof(nonAdjacent, commitmentPath, g.commitment(commitmentPath, send)),
		NonMembership: g.proof(
			nonAdjacent,
			hostv2.PacketReceiptKey(clients.DestClientID, packetTx.Sequence),
			nil,
		),
	}
}

func update(trusted, header *besu.ParsedHeader) besutest.UpdateFixture {
	return besutest.UpdateFixture{
		Height:             header.Height,
		HeaderRLP:          header.RLP,
		TrustedHeight:      trusted.Height,
		ExpectedTimestamp:  header.Timestamp,
		ExpectedStateRoot:  header.StateRoot,
		ExpectedValidators: header.Validators,
	}
}

func rejected(trusted, header *besu.ParsedHeader) besutest.RejectedUpdateFixture {
	return besutest.RejectedUpdateFixture{
		Height:        header.Height,
		HeaderRLP:     header.RLP,
		TrustedHeight: trusted.Height,
	}
}

// lowQuorumUpdate is live resealed by one validator short of a quorum.
func (g generator) lowQuorumUpdate(trusted, live *besu.ParsedHeader) besutest.RejectedUpdateFixture {
	quorum := g.headers.Quorum()
	return rejected(trusted, g.headers.Seal(g.headers.Raw(live.Height), live.Validators, quorum[:len(quorum)-1]))
}

// conflictingUpdate is live with another state root, sealed by a quorum. It
// keeps live's timestamp, so it is as current as live is.
func (g generator) conflictingUpdate(trusted, live *besu.ParsedHeader) besutest.RejectedUpdateFixture {
	header := g.headers.Raw(live.Height)
	header.Root = crypto.Keccak256Hash([]byte("conflicting state root"))
	return rejected(trusted, g.headers.Seal(header, live.Validators, g.headers.Quorum()))
}

// lowOverlapUpdate is the header at height with one trusted validator and the
// rest new, sealed by a quorum of that set holding only the trusted one.
func (g generator) lowOverlapUpdate(trusted *besu.ParsedHeader, height uint64) besutest.RejectedUpdateFixture {
	quorum := g.headers.Quorum()
	signers := []*ecdsa.PrivateKey{quorum[0]}
	validators := []common.Address{crypto.PubkeyToAddress(quorum[0].PublicKey)}
	for range len(trusted.Validators) - 1 {
		key, err := crypto.GenerateKey()
		require.NoError(g.t, err)
		signers = append(signers, key)
		validators = append(validators, crypto.PubkeyToAddress(key.PublicKey))
	}
	return rejected(trusted, g.headers.Seal(g.headers.Raw(height), validators, signers[:len(quorum)]))
}

func (g generator) commitment(path []byte, height uint64) []byte {
	g.t.Helper()
	var commitment [32]byte
	err := g.source.UseContractCaller(func(caller bind.ContractCaller) error {
		router, err := ics26router.NewContractCaller(g.router, caller)
		if err != nil {
			return err
		}
		commitment, err = router.GetCommitment(
			&bind.CallOpts{Context: g.ctx, BlockNumber: new(big.Int).SetUint64(height)},
			crypto.Keccak256Hash(path),
		)
		return err
	})
	require.NoError(g.t, err)
	require.NotEqual(g.t, [32]byte{}, commitment)
	return commitment[:]
}

// proof is the router's storage proof for path at header, with the account
// proof anchoring it to header's state root. value is empty for absence.
func (g generator) proof(header *besu.ParsedHeader, path, value []byte) besutest.MembershipFixture {
	g.t.Helper()
	slot := besu.CommitmentSlot(path)
	proof, err := gethclient.New(g.rpc).GetProof(
		g.ctx, g.router, []string{slot.Hex()}, new(big.Int).SetUint64(header.Height),
	)
	require.NoError(g.t, err)
	require.Len(g.t, proof.StorageProof, 1)
	return besutest.MembershipFixture{
		Proof:             g.proofNodes(proof.StorageProof[0].Proof),
		AccountProof:      g.proofNodes(proof.AccountProof),
		ProofHeight:       header.Height,
		Path:              path,
		Value:             value,
		ExpectedTimestamp: header.Timestamp,
	}
}

// proofNodes is abi.encode(bytes[]) of the proof nodes.
func (g generator) proofNodes(nodes []string) []byte {
	g.t.Helper()
	decoded := make([][]byte, len(nodes))
	for i, node := range nodes {
		decoded[i] = common.FromHex(node)
	}
	packed, err := besumsgs.NewBindings().TryPackProofNodes(decoded)
	require.NoError(g.t, err)
	return packed[4:]
}
