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
		Params:               AttestationParams{Attestors: []string{"0xa"}, Threshold: 1, InitialHeight: 5, InitialTimestamp: 500},
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

// TestClientStepsSyncsManifestOnSkip covers `deploy import` -> `deploy
// connect` and crash-between-RegisterClient-and-Save: the client is already
// registered on-chain but the manifest has no entry for it. The precheck
// must repair the manifest even though it reports the step as satisfied.
func TestClientStepsSyncsManifestOnSkip(t *testing.T) {
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
		Params:               AttestationParams{Attestors: []string{"0xa"}, Threshold: 1, InitialHeight: 5, InitialTimestamp: 500},
	}
	// simulate already registered on-chain (e.g. via `deploy import`),
	// without ever calling RegisterClient through this engine.
	target.registered[spec.ClientID] = "0xlive"

	res, err := RunSteps(context.Background(), slog.Default(), false, ClientSteps(target, dir, "1", spec))
	require.NoError(t, err)
	require.Equal(t, "skipped", res[0].Action)
	require.Equal(t, 0, target.registers)

	m, err = manifest.Load(dir, "1")
	require.NoError(t, err)
	c, ok := m.Client("link-2")
	require.True(t, ok)
	require.Equal(t, "0xlive", c.Address)
	require.Equal(t, "2", c.CounterpartyChainID)
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
