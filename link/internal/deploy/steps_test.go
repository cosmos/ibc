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
	provisions int
	registers  int
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
	return &fakeTarget{hasCode: map[string]bool{}, registered: map[string]string{}}
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
