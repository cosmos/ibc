// SPDX-License-Identifier: Apache-2.0

package deploy

import (
	"context"
	"encoding/json"
	"log/slog"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc-contracts/packages/go-abigen/besumsgs"
	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/cli/internal/deploy/manifest"
)

// fakeTarget satisfies Target for engine tests.
type fakeTarget struct {
	hasCode    map[string]bool
	registered map[string]string       // clientID -> address
	apps       map[string]string       // port -> app address
	bridges    map[string]fakeBridge   // ift|clientID -> bridge
	rateLimits map[string]IFTRateLimit // ift -> rate limit
	provisions int
	registers  int

	gmpProvisions       int
	multicallProvisions int
	iftProvisions       int
	ctorProvisions      int
	rateLimitSets       int
}

func (f *fakeTarget) ProvisionCore(context.Context, CoreParams) (CoreRef, error) {
	f.provisions++
	return CoreRef{Router: "0xrouter", TargetData: map[string]string{"accessManager": "0xam"}}, nil
}

func (f *fakeTarget) ProvisionMulticall(context.Context, string) (MulticallRef, error) {
	f.multicallProvisions++
	return MulticallRef{Address: "0xmulticall", Implementation: "0xmulticallImpl"}, nil
}

func (f *fakeTarget) ProvisionClient(context.Context, string, ClientSpec) (ClientRef, error) {
	f.provisions++
	return ClientRef{Address: "0xclient"}, nil
}

func (f *fakeTarget) RegisterClient(_ context.Context, _ string, spec ClientSpec, ref ClientRef) (string, error) {
	f.registers++
	f.registered[spec.ClientID] = ref.Address
	return spec.ClientID, nil
}

func (f *fakeTarget) ClientRegistered(_ context.Context, _, clientID string) (string, bool, error) {
	addr, ok := f.registered[clientID]
	return addr, ok, nil
}

func (f *fakeTarget) HasCode(_ context.Context, address string) (bool, error) {
	return f.hasCode[address], nil
}
func (f *fakeTarget) Head(context.Context) (uint64, uint64, error) { return 10, 1000, nil }
func (f *fakeTarget) Verify(context.Context, *manifest.Manifest) (Report, error) {
	return Report{}, nil
}

// fakeBridge is the on-chain bridge state a fakeTarget records.
type fakeBridge struct{ cp, ctor string }

func newFakeTarget() *fakeTarget {
	return &fakeTarget{
		hasCode:    map[string]bool{},
		registered: map[string]string{},
		apps:       map[string]string{},
		bridges:    map[string]fakeBridge{},
		rateLimits: map[string]IFTRateLimit{},
	}
}

func (f *fakeTarget) ProvisionGMP(context.Context, string, string) (GMPRef, error) {
	f.gmpProvisions++
	return GMPRef{Address: "0xgmp", AccountLogic: "0xlogic"}, nil
}

func (f *fakeTarget) RegisterApp(_ context.Context, _, app, port string) error {
	f.apps[port] = app
	return nil
}

func (f *fakeTarget) AppRegistered(_ context.Context, _, port string) (string, bool, error) {
	addr, ok := f.apps[port]
	return addr, ok, nil
}

func (f *fakeTarget) ProvisionIFT(_ context.Context, _ string, spec IFTSpec) (IFTRef, error) {
	f.iftProvisions++
	return IFTRef{Address: "0xift-" + spec.Symbol}, nil
}

func (f *fakeTarget) SetIFTRateLimit(_ context.Context, ift string, limit IFTRateLimit) error {
	f.rateLimitSets++
	f.rateLimits[ift] = limit
	return nil
}

func (f *fakeTarget) IFTRateLimit(_ context.Context, ift string) (IFTRateLimit, error) {
	if limit, ok := f.rateLimits[ift]; ok {
		return limit, nil
	}
	return IFTRateLimit{Capacity: new(big.Int)}, nil
}

func (f *fakeTarget) ProvisionSendCallConstructor(context.Context) (string, error) {
	f.ctorProvisions++
	return "0xctor", nil
}

func (f *fakeTarget) RegisterIFTBridge(_ context.Context, ift string, spec BridgeSpec) error {
	f.bridges[ift+"|"+spec.ClientID] = fakeBridge{cp: spec.CounterpartyIFT, ctor: spec.SendCallConstructor}
	return nil
}

func (f *fakeTarget) IFTBridge(_ context.Context, ift, clientID string) (string, string, bool, error) {
	b, ok := f.bridges[ift+"|"+clientID]
	return b.cp, b.ctor, ok, nil
}

func TestRunStepsDryRun(t *testing.T) {
	ran := false
	steps := []Step{{Name: "a", Run: func(context.Context) error { ran = true; return nil }}}

	res, err := RunSteps(context.Background(), slog.Default(), true, steps)
	require.NoError(t, err)
	require.False(t, ran)
	require.Equal(t, "planned", res[0].Action)
}

func TestCoreStepsIdempotent(t *testing.T) {
	dir := t.TempDir()
	target := newFakeTarget()

	res, err := RunSteps(context.Background(), slog.Default(), false, CoreSteps(target, dir, "1"))
	require.NoError(t, err)
	require.Equal(t, "executed", res[0].Action)
	require.Equal(t, "executed", res[1].Action)
	require.Equal(t, 1, target.provisions)
	require.Equal(t, 1, target.multicallProvisions)

	m, err := manifest.Load(dir, "1")
	require.NoError(t, err)
	require.Equal(t, "0xrouter", m.Core.Router)
	require.Equal(t, "0xam", m.TargetData["accessManager"])
	require.Equal(t, "0xmulticall", m.Core.Multicall)
	require.Equal(t, "0xmulticallImpl", m.TargetData["ibcMulticallImplementation"])

	// second run skips: manifest has the router and the chain has its code
	target.hasCode["0xrouter"] = true
	target.hasCode["0xmulticall"] = true
	res, err = RunSteps(context.Background(), slog.Default(), false, CoreSteps(target, dir, "1"))
	require.NoError(t, err)
	require.Equal(t, "skipped", res[0].Action)
	require.Equal(t, "skipped", res[1].Action)
	require.Equal(t, 1, target.provisions)
	require.Equal(t, 1, target.multicallProvisions)
}

func TestClientStepsIdempotent(t *testing.T) {
	dir := t.TempDir()
	target := newFakeTarget()
	target.hasCode["0xrouter"] = true

	m := manifest.New("1", "test")
	m.Core.Router = "0xrouter"
	require.NoError(t, m.Save(dir))

	spec := ClientSpec{
		ClientID:             "cli-2",
		CounterpartyChainID:  "2",
		CounterpartyClientID: "cli-1",
		Params: AttestationParams{
			Attestors:        []string{"0xa"},
			Threshold:        1,
			InitialHeight:    5,
			InitialTimestamp: 500,
		},
	}

	res, err := RunSteps(context.Background(), slog.Default(), false, ClientSteps(target, dir, "1", spec))
	require.NoError(t, err)
	require.Equal(t, "executed", res[0].Action)
	require.Equal(t, 1, target.registers)

	m, err = manifest.Load(dir, "1")
	require.NoError(t, err)
	c, ok := m.Client("cli-2")
	require.True(t, ok)
	require.Equal(t, "0xclient", c.Address)
	require.Equal(t, "2", c.CounterpartyChainID)

	// second run skips: client already registered on-chain
	res, err = RunSteps(context.Background(), slog.Default(), false, ClientSteps(target, dir, "1", spec))
	require.NoError(t, err)
	require.Equal(t, "skipped", res[0].Action)
	require.Equal(t, 1, target.registers)
}

// A client registered on-chain with no manifest entry (a deployment that
// died between RegisterClient and Save) cannot have its deployed parameters
// recovered reliably, so the precheck fails rather than trusting the
// rerun's spec.
func TestClientStepsUnrecordedClientError(t *testing.T) {
	dir := t.TempDir()
	target := newFakeTarget()
	target.hasCode["0xrouter"] = true

	m := manifest.New("1", "test")
	m.Core.Router = "0xrouter"
	require.NoError(t, m.Save(dir))

	spec := ClientSpec{
		ClientID:             "cli-2",
		CounterpartyChainID:  "2",
		CounterpartyClientID: "cli-1",
		Params: AttestationParams{
			Attestors:        []string{"0xa"},
			Threshold:        1,
			InitialHeight:    5,
			InitialTimestamp: 500,
		},
	}
	// registered on-chain without this engine ever recording it
	target.registered[spec.ClientID] = "0xlive"

	_, err := RunSteps(context.Background(), slog.Default(), false, ClientSteps(target, dir, "1", spec))
	require.ErrorContains(t, err, "registered on-chain but missing from the manifest")
	require.ErrorContains(t, err, "--client-id")
	require.Equal(t, 0, target.registers)

	// the manifest is left untouched
	m, err = manifest.Load(dir, "1")
	require.NoError(t, err)
	_, ok := m.Client("cli-2")
	require.False(t, ok)
}

// A rerun whose spec conflicts with the recorded deployment on identity
// fields must fail loudly: on-chain client params are constructor-fixed, so
// skipping cannot satisfy the new spec and rewriting the manifest would
// desync it from the chain.
func TestClientStepsDivergentSpecError(t *testing.T) {
	dir := t.TempDir()
	target := newFakeTarget()
	target.hasCode["0xrouter"] = true

	m := manifest.New("1", "test")
	m.Core.Router = "0xrouter"
	require.NoError(t, m.Save(dir))

	spec := ClientSpec{
		ClientID:             "cli-1-2",
		CounterpartyChainID:  "2",
		CounterpartyClientID: "cli-1-2",
		Params: AttestationParams{
			Attestors:        []string{"0xa"},
			Threshold:        1,
			InitialHeight:    5,
			InitialTimestamp: 500,
		},
	}
	_, err := RunSteps(context.Background(), slog.Default(), false, ClientSteps(target, dir, "1", spec))
	require.NoError(t, err)

	divergent := spec
	divergent.Params = AttestationParams{
		Attestors:        []string{"0xb", "0xc"},
		Threshold:        2,
		InitialHeight:    5,
		InitialTimestamp: 500,
	}
	_, err = RunSteps(context.Background(), slog.Default(), false, ClientSteps(target, dir, "1", divergent))
	require.ErrorContains(t, err, "already deployed with different values")
	require.ErrorContains(t, err, "attestors")
	require.ErrorContains(t, err, `["0xb","0xc"]`)
	require.ErrorContains(t, err, "threshold: deployed 1, requested 2")
	require.ErrorContains(t, err, "--client-id")

	// a different client type conflicts without comparing params
	retyped := spec
	retyped.Params = besuSpec().Params
	_, err = RunSteps(context.Background(), slog.Default(), false, ClientSteps(target, dir, "1", retyped))
	require.ErrorContains(t, err, `type: deployed "attestation", requested "besu"`)

	// manifest must still describe the deployed contract
	m, loadErr := manifest.Load(dir, "1")
	require.NoError(t, loadErr)
	c, _ := m.Client("cli-1-2")
	require.Equal(t, []string{"0xa"}, c.Attestation.Attestors)
}

// initialHeight/initialTimestamp default from the live counterparty head and
// change every invocation; they are launch-time trusted state, not client
// identity, so a rerun differing only in them skips cleanly and leaves the
// deploy-time values in the manifest.
func TestGMPStepsIdempotent(t *testing.T) {
	dir := t.TempDir()
	target := newFakeTarget()
	target.hasCode["0xrouter"] = true

	m := manifest.New("1", "test")
	m.Core.Router = "0xrouter"
	m.TargetData = map[string]string{"accessManager": "0xam"}
	require.NoError(t, m.Save(dir))

	res, err := RunSteps(context.Background(), slog.Default(), false, GMPSteps(target, dir, "1"))
	require.NoError(t, err)
	require.Equal(t, "executed", res[0].Action)
	require.Equal(t, 1, target.gmpProvisions)

	m, err = manifest.Load(dir, "1")
	require.NoError(t, err)
	require.NotNil(t, m.GMP)
	require.Equal(t, "0xgmp", m.GMP.Address)
	require.Equal(t, GMPPortID, m.GMP.Port)

	// second run skips: gmp recorded, has code, app registered
	target.hasCode["0xgmp"] = true
	res, err = RunSteps(context.Background(), slog.Default(), false, GMPSteps(target, dir, "1"))
	require.NoError(t, err)
	require.Equal(t, "skipped", res[0].Action)
	require.Equal(t, 1, target.gmpProvisions)
}

func TestGMPStepsRequiresCore(t *testing.T) {
	dir := t.TempDir()
	target := newFakeTarget()
	_, err := RunSteps(context.Background(), slog.Default(), false, GMPSteps(target, dir, "1"))
	require.ErrorContains(t, err, "run `ibc deploy core` first")
}

func TestGMPStepsInterruptedDeploy(t *testing.T) {
	dir := t.TempDir()
	target := newFakeTarget()
	target.hasCode["0xrouter"] = true

	m := manifest.New("1", "test")
	m.Core.Router = "0xrouter"
	m.TargetData = map[string]string{"accessManager": "0xam"}
	require.NoError(t, m.Save(dir))

	// registered on-chain but absent from the manifest
	target.apps[GMPPortID] = "0xlive"

	_, err := RunSteps(context.Background(), slog.Default(), false, GMPSteps(target, dir, "1"))
	require.ErrorContains(t, err, "missing from the manifest")
	require.Equal(t, 0, target.gmpProvisions)
}

func TestClientStepsRerunIgnoresTrustedStateDrift(t *testing.T) {
	dir := t.TempDir()
	target := newFakeTarget()
	target.hasCode["0xrouter"] = true

	m := manifest.New("1", "test")
	m.Core.Router = "0xrouter"
	require.NoError(t, m.Save(dir))

	spec := ClientSpec{
		ClientID:             "cli-1-2",
		CounterpartyChainID:  "2",
		CounterpartyClientID: "cli-1-2",
		Params: AttestationParams{
			Attestors:        []string{"0xa"},
			Threshold:        1,
			InitialHeight:    5,
			InitialTimestamp: 500,
		},
	}
	_, err := RunSteps(context.Background(), slog.Default(), false, ClientSteps(target, dir, "1", spec))
	require.NoError(t, err)

	drifted := spec
	drifted.Params = AttestationParams{
		Attestors:        []string{"0xa"},
		Threshold:        1,
		InitialHeight:    99,
		InitialTimestamp: 9999,
	}
	res, err := RunSteps(context.Background(), slog.Default(), false, ClientSteps(target, dir, "1", drifted))
	require.NoError(t, err)
	require.Equal(t, "skipped", res[0].Action)

	m, err = manifest.Load(dir, "1")
	require.NoError(t, err)
	c, _ := m.Client("cli-1-2")
	require.Equal(t, uint64(5), c.Attestation.InitialHeight)
	require.Equal(t, uint64(500), c.Attestation.InitialTimestamp)
}

func TestIFTStepsIdempotentAndDuplicateSymbol(t *testing.T) {
	dir := t.TempDir()
	target := newFakeTarget()

	m := manifest.New("1", "test")
	m.Core.Router = "0xrouter"
	m.GMP = &manifest.GMP{Address: "0xgmp", Port: GMPPortID}
	require.NoError(t, m.Save(dir))

	spec := IFTSpec{Owner: "0xowner", Name: "Foo", Symbol: "FOO"}

	res, err := RunSteps(context.Background(), slog.Default(), false, IFTSteps(target, dir, "1", spec))
	require.NoError(t, err)
	require.Equal(t, "executed", res[0].Action)
	require.Equal(t, 1, target.iftProvisions)

	m, err = manifest.Load(dir, "1")
	require.NoError(t, err)
	tok, ok := m.TokenByAddress("0xift-FOO")
	require.True(t, ok)
	require.Equal(t, "0xift-FOO", tok.Address)

	// identical rerun skips
	target.hasCode["0xift-FOO"] = true
	res, err = RunSteps(context.Background(), slog.Default(), false, IFTSteps(target, dir, "1", spec))
	require.NoError(t, err)
	require.Equal(t, "skipped", res[0].Action)
	require.Equal(t, 1, target.iftProvisions)

	// same symbol, different name is a new token
	other := IFTSpec{Owner: "0xowner", Name: "Bar", Symbol: "FOO"}
	res, err = RunSteps(context.Background(), slog.Default(), false, IFTSteps(target, dir, "1", other))
	require.NoError(t, err)
	require.Equal(t, "executed", res[0].Action)
	require.Equal(t, 2, target.iftProvisions)

	m, err = manifest.Load(dir, "1")
	require.NoError(t, err)
	require.Len(t, m.Tokens, 2)
}

func TestIFTStepsRateLimit(t *testing.T) {
	dir := t.TempDir()
	target := newFakeTarget()

	m := manifest.New("1", "test")
	m.Core.Router = "0xrouter"
	m.GMP = &manifest.GMP{Address: "0xgmp", Port: GMPPortID}
	require.NoError(t, m.Save(dir))

	limit := IFTRateLimit{Capacity: big.NewInt(1000), Window: 3600}
	spec := IFTSpec{Owner: "0xowner", Name: "Foo", Symbol: "FOO", RateLimit: &limit}
	actions := func(spec IFTSpec) []string {
		t.Helper()
		res, err := RunSteps(context.Background(), slog.Default(), false, IFTSteps(target, dir, "1", spec))
		require.NoError(t, err)
		out := make([]string, len(res))
		for i, r := range res {
			out[i] = r.Action
		}
		return out
	}

	require.Equal(t, []string{"executed", "executed"}, actions(spec))
	require.Equal(t, limit, target.rateLimits["0xift-FOO"])

	// identical rerun skips both steps
	target.hasCode["0xift-FOO"] = true
	require.Equal(t, []string{"skipped", "skipped"}, actions(spec))
	require.Equal(t, 1, target.rateLimitSets)

	// a changed rate limit updates the existing token
	raised := IFTRateLimit{Capacity: big.NewInt(2000), Window: 3600}
	spec.RateLimit = &raised
	require.Equal(t, []string{"skipped", "executed"}, actions(spec))
	require.Equal(t, 1, target.iftProvisions)
	require.Equal(t, raised, target.rateLimits["0xift-FOO"])

	// without a rate limit only the token step runs
	spec.RateLimit = nil
	require.Equal(t, []string{"skipped"}, actions(spec))
}

func TestIFTStepsRequiresGMP(t *testing.T) {
	dir := t.TempDir()
	target := newFakeTarget()
	m := manifest.New("1", "test")
	m.Core.Router = "0xrouter"
	require.NoError(t, m.Save(dir))

	_, err := RunSteps(context.Background(), slog.Default(), false,
		IFTSteps(target, dir, "1", IFTSpec{Owner: "0xowner", Name: "Foo", Symbol: "FOO"}))
	require.ErrorContains(t, err, "run `ibc deploy gmp` first")
}

func iftBridgeManifest(t *testing.T, dir string) {
	t.Helper()
	m := manifest.New("1", "test")
	m.Core.Router = "0xrouter"
	m.GMP = &manifest.GMP{Address: "0xgmp", Port: GMPPortID}
	m.UpsertToken(manifest.Token{Symbol: "FOO", Name: "Foo", Address: "0xift-FOO", Owner: "0xowner"})
	require.NoError(t, m.Save(dir))
}

func TestIFTBridgeStepsAutoConstructor(t *testing.T) {
	dir := t.TempDir()
	target := newFakeTarget()
	target.registered["cli-2"] = "0xclient" // client deployed on the router
	iftBridgeManifest(t, dir)

	spec := BridgeSpec{ClientID: "cli-2", CounterpartyIFT: "0xcp"}
	res, err := RunSteps(
		context.Background(),
		slog.Default(),
		false,
		IFTBridgeSteps(target, dir, "1", "0xift-FOO", "", spec),
	)
	require.NoError(t, err)
	require.Equal(t, "executed", res[0].Action) // constructor deployed
	require.Equal(t, "executed", res[1].Action) // bridge registered
	require.Equal(t, 1, target.ctorProvisions)

	m, err := manifest.Load(dir, "1")
	require.NoError(t, err)
	require.Equal(t, "0xctor", m.EVMSendCallConstructor)
	tok, _ := m.TokenByAddress("0xift-FOO")
	b, ok := tok.Bridge("cli-2")
	require.True(t, ok)
	require.Equal(t, "0xcp", b.CounterpartyIFT)
	require.Equal(t, "0xctor", b.SendCallConstructor)

	// rerun skips both
	res, err = RunSteps(
		context.Background(),
		slog.Default(),
		false,
		IFTBridgeSteps(target, dir, "1", "0xift-FOO", "", spec),
	)
	require.NoError(t, err)
	require.Equal(t, "skipped", res[0].Action)
	require.Equal(t, "skipped", res[1].Action)
	require.Equal(t, 1, target.ctorProvisions)
}

func TestIFTBridgeStepsOverrideSkipsConstructor(t *testing.T) {
	dir := t.TempDir()
	target := newFakeTarget()
	target.registered["cli-2"] = "0xclient"
	iftBridgeManifest(t, dir)

	spec := BridgeSpec{ClientID: "cli-2", CounterpartyIFT: "0xcp"}
	res, err := RunSteps(
		context.Background(),
		slog.Default(),
		false,
		IFTBridgeSteps(target, dir, "1", "0xift-FOO", "0xoverride", spec),
	)
	require.NoError(t, err)
	require.Equal(t, "skipped", res[0].Action) // constructor step skipped: override supplied
	require.Equal(t, "executed", res[1].Action)
	require.Equal(t, 0, target.ctorProvisions)

	m, err := manifest.Load(dir, "1")
	require.NoError(t, err)
	tok, _ := m.TokenByAddress("0xift-FOO")
	b, _ := tok.Bridge("cli-2")
	require.Equal(t, "0xoverride", b.SendCallConstructor)
}

func TestIFTBridgeStepsCounterpartyConflict(t *testing.T) {
	dir := t.TempDir()
	target := newFakeTarget()
	target.registered["cli-2"] = "0xclient"
	iftBridgeManifest(t, dir)
	// on-chain bridge points at a different counterparty
	target.bridges["0xift-FOO|cli-2"] = fakeBridge{cp: "0xother"}

	spec := BridgeSpec{ClientID: "cli-2", CounterpartyIFT: "0xcp"}
	_, err := RunSteps(context.Background(), slog.Default(), false,
		IFTBridgeSteps(target, dir, "1", "0xift-FOO", "0xoverride", spec))
	require.ErrorContains(t, err, "already registered to counterparty")
}

func TestIFTBridgeStepsConstructorChange(t *testing.T) {
	dir := t.TempDir()
	target := newFakeTarget()
	target.registered["cli-2"] = "0xclient"
	iftBridgeManifest(t, dir)

	spec := BridgeSpec{ClientID: "cli-2", CounterpartyIFT: "0xcp"}
	_, err := RunSteps(context.Background(), slog.Default(), false,
		IFTBridgeSteps(target, dir, "1", "0xift-FOO", "0xctorA", spec))
	require.NoError(t, err)

	// different constructor re-registers
	res, err := RunSteps(context.Background(), slog.Default(), false,
		IFTBridgeSteps(target, dir, "1", "0xift-FOO", "0xctorB", spec))
	require.NoError(t, err)
	require.Equal(t, "executed", res[1].Action)

	m, err := manifest.Load(dir, "1")
	require.NoError(t, err)
	tok, _ := m.TokenByAddress("0xift-FOO")
	b, ok := tok.Bridge("cli-2")
	require.True(t, ok)
	require.Equal(t, "0xctorB", b.SendCallConstructor)

	// rerun with the same constructor skips
	res, err = RunSteps(context.Background(), slog.Default(), false,
		IFTBridgeSteps(target, dir, "1", "0xift-FOO", "0xctorB", spec))
	require.NoError(t, err)
	require.Equal(t, "skipped", res[1].Action)
}

func TestIFTBridgeStepsRequiresClient(t *testing.T) {
	dir := t.TempDir()
	target := newFakeTarget()
	iftBridgeManifest(t, dir) // token present, but no client registered

	spec := BridgeSpec{ClientID: "cli-2", CounterpartyIFT: "0xcp"}
	_, err := RunSteps(context.Background(), slog.Default(), false,
		IFTBridgeSteps(target, dir, "1", "0xift-FOO", "0xoverride", spec))
	require.ErrorContains(t, err, "run `ibc deploy client` first")
}

func TestIFTBridgeStepsPlanRejectsMissingClient(t *testing.T) {
	dir := t.TempDir()
	target := newFakeTarget() // no client registered
	iftBridgeManifest(t, dir)

	spec := BridgeSpec{ClientID: "cli-2", CounterpartyIFT: "0xcp"}
	_, err := RunSteps(context.Background(), slog.Default(), true, // dry-run
		IFTBridgeSteps(target, dir, "1", "0xift-FOO", "0xoverride", spec))
	require.ErrorContains(t, err, "run `ibc deploy client` first")
	require.Empty(t, target.bridges)
}

func TestIFTBridgeStepsDryRunAfterOverride(t *testing.T) {
	dir := t.TempDir()
	target := newFakeTarget()
	target.registered["cli-2"] = "0xclient"
	iftBridgeManifest(t, dir)

	spec := BridgeSpec{ClientID: "cli-2", CounterpartyIFT: "0xcp"}
	_, err := RunSteps(context.Background(), slog.Default(), false,
		IFTBridgeSteps(target, dir, "1", "0xift-FOO", "0xoverride", spec))
	require.NoError(t, err)

	m, err := manifest.Load(dir, "1")
	require.NoError(t, err)
	require.Empty(t, m.EVMSendCallConstructor)

	res, err := RunSteps(context.Background(), slog.Default(), true,
		IFTBridgeSteps(target, dir, "1", "0xift-FOO", "", spec))
	require.NoError(t, err)
	require.Equal(t, "skipped", res[1].Action)
}

func TestIFTBridgeStepsUnrecordedToken(t *testing.T) {
	dir := t.TempDir()
	target := newFakeTarget()
	target.registered["cli-2"] = "0xclient"
	m := manifest.New("1", "test")
	m.Core.Router = "0xrouter"
	require.NoError(t, m.Save(dir))

	spec := BridgeSpec{ClientID: "cli-2", CounterpartyIFT: "0xcp"}
	res, err := RunSteps(context.Background(), slog.Default(), false,
		IFTBridgeSteps(target, dir, "1", "0xexternal", "0xoverride", spec))
	require.NoError(t, err)
	require.Equal(t, "executed", res[1].Action)

	m, err = manifest.Load(dir, "1")
	require.NoError(t, err)
	require.Empty(t, m.Tokens)
}

func besuSpec() ClientSpec {
	return ClientSpec{
		ClientID:             "cli-2",
		CounterpartyChainID:  "2",
		CounterpartyClientID: "cli-1",
		Params: BesuParams{
			Consensus:     besu.QBFT,
			IBCRouter:     common.HexToAddress("0x00000000000000000000000000000000000000cc"),
			InitialHeight: 112,
			InitialConsensusState: besumsgs.IBesuLightClientMsgsConsensusState{
				Timestamp:  1788192445,
				StateRoot:  common.HexToHash("0x69c8d1758a0375ec0d4ee22f16e3119c84ecb3aaaaaaaaaaaaaaaaaaaaaaaaaa"),
				Validators: []common.Address{common.HexToAddress("0x00000000000000000000000000000000000000aa")},
			},
			TrustingPeriod: 1209600,
			MaxClockDrift:  15,
			TrustLevel:     besumsgs.IBesuLightClientMsgsTrustThreshold{Numerator: 2, Denominator: 3},
		},
	}
}

// besuRecord is the manifest record of besuSpec's params.
const besuRecord = `{
	"consensus": "qbft",
	"ibcRouter": "0x00000000000000000000000000000000000000cc",
	"initialHeight": 112,
	"initialTimestamp": 1788192445,
	"initialStateRoot": "0x69c8d1758a0375ec0d4ee22f16e3119c84ecb3aaaaaaaaaaaaaaaaaaaaaaaaaa",
	"initialValidators": ["0x00000000000000000000000000000000000000AA"],
	"trustingPeriod": 1209600,
	"maxClockDrift": 15,
	"trustLevel": {"numerator": 2, "denominator": 3}
}`

func TestClientStepsBesu(t *testing.T) {
	dir := t.TempDir()
	target := newFakeTarget()
	target.hasCode["0xrouter"] = true

	m := manifest.New("1", "test")
	m.Core.Router = "0xrouter"
	require.NoError(t, m.Save(dir))

	spec := besuSpec()

	res, err := RunSteps(context.Background(), slog.Default(), false, ClientSteps(target, dir, "1", spec))
	require.NoError(t, err)
	require.Equal(t, "executed", res[0].Action)

	m, err = manifest.Load(dir, "1")
	require.NoError(t, err)
	recorded, ok := m.Client("cli-2")
	require.True(t, ok)

	// the manifest records the constructor params
	got, err := json.Marshal(recorded.Besu)
	require.NoError(t, err)
	require.JSONEq(t, besuRecord, string(got))

	// rerun skips; initial trusted state drift is not an identity conflict
	drifted := besuSpec()
	p := drifted.Params.(BesuParams)
	p.InitialHeight, p.InitialConsensusState.Timestamp = 200, 1788192600
	drifted.Params = p
	res, err = RunSteps(context.Background(), slog.Default(), false, ClientSteps(target, dir, "1", drifted))
	require.NoError(t, err)
	require.Equal(t, "skipped", res[0].Action)
	require.Equal(t, 1, target.registers)

	// identity fields do conflict
	for name, mutate := range map[string]func(*BesuParams){
		"ibcRouter": func(p *BesuParams) {
			p.IBCRouter = common.HexToAddress("0x00000000000000000000000000000000000000dd")
		},
		"trustingPeriod": func(p *BesuParams) { p.TrustingPeriod = 1 },
		"maxClockDrift":  func(p *BesuParams) { p.MaxClockDrift = 1 },
		"trustLevel": func(p *BesuParams) {
			p.TrustLevel = besumsgs.IBesuLightClientMsgsTrustThreshold{Numerator: 1, Denominator: 2}
		},
		"consensus": func(p *BesuParams) { p.Consensus = besu.IBFT2 },
	} {
		t.Run(name, func(t *testing.T) {
			conflicting := besuSpec()
			p := conflicting.Params.(BesuParams)
			mutate(&p)
			conflicting.Params = p
			_, runErr := RunSteps(
				context.Background(),
				slog.Default(),
				false,
				ClientSteps(target, dir, "1", conflicting),
			)
			require.ErrorContains(t, runErr, name)
		})
	}
}
