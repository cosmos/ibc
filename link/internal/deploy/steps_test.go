// SPDX-License-Identifier: Apache-2.0

package deploy

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/deploy/manifest"
)

// fakeTarget satisfies Target for engine tests.
type fakeTarget struct {
	hasCode    map[string]bool
	registered map[string]string // clientID -> address
	apps       map[string]string // port -> app address
	bridges    map[string]string // ift|clientID -> counterparty ift
	provisions int
	registers  int

	gmpProvisions  int
	iftProvisions  int
	ctorProvisions int
}

func (f *fakeTarget) ProvisionCore(context.Context, CoreParams) (CoreRef, error) {
	f.provisions++
	return CoreRef{Router: "0xrouter", TargetData: map[string]string{"accessManager": "0xam"}}, nil
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
func (f *fakeTarget) SupportedClientTypes() []string { return []string{ClientTypeAttestation} }

func newFakeTarget() *fakeTarget {
	return &fakeTarget{
		hasCode:    map[string]bool{},
		registered: map[string]string{},
		apps:       map[string]string{},
		bridges:    map[string]string{},
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

func (f *fakeTarget) ProvisionSendCallConstructor(context.Context) (string, error) {
	f.ctorProvisions++
	return "0xctor", nil
}

func (f *fakeTarget) RegisterIFTBridge(_ context.Context, ift string, spec BridgeSpec) error {
	f.bridges[ift+"|"+spec.ClientID] = spec.CounterpartyIFT
	return nil
}

func (f *fakeTarget) IFTBridge(_ context.Context, ift, clientID string) (string, bool, error) {
	cp, ok := f.bridges[ift+"|"+clientID]
	return cp, ok, nil
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
	require.Equal(t, 1, target.provisions)

	m, err := manifest.Load(dir, "1")
	require.NoError(t, err)
	require.Equal(t, "0xrouter", m.Core.Router)
	require.Equal(t, "0xam", m.TargetData["accessManager"])

	// second run skips: manifest has the router and the chain has its code
	target.hasCode["0xrouter"] = true
	res, err = RunSteps(context.Background(), slog.Default(), false, CoreSteps(target, dir, "1"))
	require.NoError(t, err)
	require.Equal(t, "skipped", res[0].Action)
	require.Equal(t, 1, target.provisions)
}

func TestClientStepsIdempotent(t *testing.T) {
	dir := t.TempDir()
	target := newFakeTarget()
	target.hasCode["0xrouter"] = true

	m := manifest.New("1", "test")
	m.Core.Router = "0xrouter"
	require.NoError(t, m.Save(dir))

	spec := ClientSpec{
		ClientID:             "link-2",
		Type:                 ClientTypeAttestation,
		CounterpartyChainID:  "2",
		CounterpartyClientID: "link-1",
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
	c, ok := m.Client("link-2")
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
		ClientID:             "link-2",
		Type:                 ClientTypeAttestation,
		CounterpartyChainID:  "2",
		CounterpartyClientID: "link-1",
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
	_, ok := m.Client("link-2")
	require.False(t, ok)
}

func TestClientStepsParamsMismatch(t *testing.T) {
	dir := t.TempDir()
	target := newFakeTarget()
	target.hasCode["0xrouter"] = true

	m := manifest.New("1", "test")
	m.Core.Router = "0xrouter"
	require.NoError(t, m.Save(dir))

	spec := ClientSpec{
		ClientID:             "link-2",
		Type:                 ClientTypeAttestation,
		CounterpartyChainID:  "2",
		CounterpartyClientID: "link-1",
		Params:               "not-attestation-params",
	}
	_, err := RunSteps(context.Background(), slog.Default(), false, ClientSteps(target, dir, "1", spec))
	require.ErrorContains(t, err, "does not match client type")
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
		ClientID:             "link-1-2",
		Type:                 ClientTypeAttestation,
		CounterpartyChainID:  "2",
		CounterpartyClientID: "link-1-2",
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
	require.ErrorContains(t, err, "--client-id")

	// manifest must still describe the deployed contract
	m, loadErr := manifest.Load(dir, "1")
	require.NoError(t, loadErr)
	c, _ := m.Client("link-1-2")
	require.Equal(t, []any{"0xa"}, c.Params["attestors"].([]any))
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

// An interrupted GMP deploy — app registered on-chain but not yet saved to
// the manifest — must fail loudly on rerun, not silently re-provision (the
// fixed "gmpport" cannot be re-registered).
func TestGMPStepsInterruptedDeploy(t *testing.T) {
	dir := t.TempDir()
	target := newFakeTarget()
	target.hasCode["0xrouter"] = true

	m := manifest.New("1", "test")
	m.Core.Router = "0xrouter"
	m.TargetData = map[string]string{"accessManager": "0xam"}
	require.NoError(t, m.Save(dir))

	// app registered on-chain at the fixed port, but manifest has no GMP
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
		ClientID:             "link-1-2",
		Type:                 ClientTypeAttestation,
		CounterpartyChainID:  "2",
		CounterpartyClientID: "link-1-2",
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
	c, _ := m.Client("link-1-2")
	require.InDelta(t, 5, c.Params["initialHeight"], 0)
	require.InDelta(t, 500, c.Params["initialTimestamp"], 0)
}

func TestIFTStepsIdempotentAndConflict(t *testing.T) {
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
	tok, ok := m.Token("FOO")
	require.True(t, ok)
	require.Equal(t, "0xift-FOO", tok.Address)

	// rerun skips: token recorded and has code
	target.hasCode["0xift-FOO"] = true
	res, err = RunSteps(context.Background(), slog.Default(), false, IFTSteps(target, dir, "1", spec))
	require.NoError(t, err)
	require.Equal(t, "skipped", res[0].Action)
	require.Equal(t, 1, target.iftProvisions)

	// same symbol, different name conflicts
	conflicting := IFTSpec{Owner: "0xowner", Name: "Bar", Symbol: "FOO"}
	_, err = RunSteps(context.Background(), slog.Default(), false, IFTSteps(target, dir, "1", conflicting))
	require.ErrorContains(t, err, "already deployed with different values")
	require.ErrorContains(t, err, "--symbol")
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
	target.registered["link-2"] = "0xclient" // client deployed on the router
	iftBridgeManifest(t, dir)

	spec := BridgeSpec{ClientID: "link-2", CounterpartyIFT: "0xcp"}
	res, err := RunSteps(context.Background(), slog.Default(), false, IFTBridgeSteps(target, dir, "1", "FOO", "", spec))
	require.NoError(t, err)
	require.Equal(t, "executed", res[0].Action) // constructor deployed
	require.Equal(t, "executed", res[1].Action) // bridge registered
	require.Equal(t, 1, target.ctorProvisions)

	m, err := manifest.Load(dir, "1")
	require.NoError(t, err)
	require.Equal(t, "0xctor", m.SendCallConstructor)
	tok, _ := m.Token("FOO")
	b, ok := tok.Bridge("link-2")
	require.True(t, ok)
	require.Equal(t, "0xcp", b.CounterpartyIFT)
	require.Equal(t, "0xctor", b.SendCallConstructor)

	// rerun skips both: constructor recorded, bridge registered on-chain
	res, err = RunSteps(context.Background(), slog.Default(), false, IFTBridgeSteps(target, dir, "1", "FOO", "", spec))
	require.NoError(t, err)
	require.Equal(t, "skipped", res[0].Action)
	require.Equal(t, "skipped", res[1].Action)
	require.Equal(t, 1, target.ctorProvisions)
}

func TestIFTBridgeStepsOverrideSkipsConstructor(t *testing.T) {
	dir := t.TempDir()
	target := newFakeTarget()
	target.registered["link-2"] = "0xclient"
	iftBridgeManifest(t, dir)

	spec := BridgeSpec{ClientID: "link-2", CounterpartyIFT: "0xcp"}
	res, err := RunSteps(
		context.Background(),
		slog.Default(),
		false,
		IFTBridgeSteps(target, dir, "1", "FOO", "0xoverride", spec),
	)
	require.NoError(t, err)
	require.Equal(t, "skipped", res[0].Action) // constructor step skipped: override supplied
	require.Equal(t, "executed", res[1].Action)
	require.Equal(t, 0, target.ctorProvisions)

	m, err := manifest.Load(dir, "1")
	require.NoError(t, err)
	tok, _ := m.Token("FOO")
	b, _ := tok.Bridge("link-2")
	require.Equal(t, "0xoverride", b.SendCallConstructor)
}

// A bridge already registered on-chain to a different counterparty than
// requested must fail loudly rather than skip or overwrite.
func TestIFTBridgeStepsCounterpartyConflict(t *testing.T) {
	dir := t.TempDir()
	target := newFakeTarget()
	target.registered["link-2"] = "0xclient"
	iftBridgeManifest(t, dir)
	// on-chain bridge points at a different counterparty
	target.bridges["0xift-FOO|link-2"] = "0xother"

	spec := BridgeSpec{ClientID: "link-2", CounterpartyIFT: "0xcp"}
	_, err := RunSteps(context.Background(), slog.Default(), false,
		IFTBridgeSteps(target, dir, "1", "FOO", "0xoverride", spec))
	require.ErrorContains(t, err, "already registered to counterparty")
}

func TestIFTBridgeStepsRequiresClientAndToken(t *testing.T) {
	dir := t.TempDir()
	target := newFakeTarget()
	iftBridgeManifest(t, dir) // token present, but no client registered

	spec := BridgeSpec{ClientID: "link-2", CounterpartyIFT: "0xcp"}
	_, err := RunSteps(
		context.Background(),
		slog.Default(),
		false,
		IFTBridgeSteps(target, dir, "1", "FOO", "0xoverride", spec),
	)
	require.ErrorContains(t, err, "run `ibc deploy client` first")

	// missing token
	dir2 := t.TempDir()
	m := manifest.New("1", "test")
	m.Core.Router = "0xrouter"
	require.NoError(t, m.Save(dir2))
	target2 := newFakeTarget()
	target2.registered["link-2"] = "0xclient"
	_, err = RunSteps(
		context.Background(),
		slog.Default(),
		false,
		IFTBridgeSteps(target2, dir2, "1", "FOO", "0xoverride", spec),
	)
	require.ErrorContains(t, err, "run `ibc deploy ift` first")
}
