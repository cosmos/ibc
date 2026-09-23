// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/internal/config"
	"github.com/cosmos/ibc/cli/internal/deploy"
	"github.com/cosmos/ibc/cli/internal/deploy/manifest"
	"github.com/cosmos/ibc/cli/internal/service/signer"
)

// newLocalSignerConfig generates a real local secp256k1 key, stores it to a
// temp keyfile, and returns both the config entry referencing it and its
// derived EVM address -- attestor resolution derives addresses the same way,
// so tests need a real key to exercise it end to end.
func newLocalSignerConfig(t *testing.T, alias string) (config.SignerConfig, string) {
	t.Helper()

	key, err := signer.GenerateLocalSecp256k1Signer()
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), alias+".json")
	require.NoError(t, key.StoreToFile(path))

	address, err := signer.PublicKeyToEVMAddress(key.PublicKey())
	require.NoError(t, err)

	return config.SignerConfig{Alias: alias, Type: config.SignerLocal, File: path}, address
}

func TestResolveDeployerAlias(t *testing.T) {
	chain := config.ChainConfig{ChainID: "1", Deployer: "cfg-alias"}

	require.Equal(t, "flag-alias", resolveDeployerAlias(chain, "flag-alias"))
	require.Equal(t, "cfg-alias", resolveDeployerAlias(chain, ""))

	chain.Deployer = ""
	require.Empty(t, resolveDeployerAlias(chain, ""))
}

func TestStatusChains(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{Chains: []config.ChainConfig{{ChainID: "1"}, {ChainID: "2"}}}

	// explicit unknown chain is an error, not "no manifest"
	_, err := statusChains(cfg, dir, "barfoo")
	require.ErrorContains(t, err, `chain "barfoo" not declared in config`)

	// explicit known chain
	chains, err := statusChains(cfg, dir, "2")
	require.NoError(t, err)
	require.Equal(t, []string{"2"}, chains)

	// no flag: union of config chains and manifest files, deduped and sorted
	require.NoError(t, manifest.New("2", "evm").Save(dir))
	require.NoError(t, manifest.New("5", "evm").Save(dir))
	chains, err = statusChains(cfg, dir, "")
	require.NoError(t, err)
	require.Equal(t, []string{"1", "2", "5"}, chains)
}

func TestRenderRelayConfig(t *testing.T) {
	watcherSigner, watcherAddress := newLocalSignerConfig(t, "attestor-watching-2")

	// client ids follow defaultClientID's real convention: both ends of a
	// connection share the same sorted "cli-<a>-<b>" name.
	a := manifest.New("1", "evm")
	a.Core.Router = "0xrouterA"
	a.UpsertClient(manifest.Client{
		ClientID: "cli-1-2", Type: "attestation", Address: "0xca",
		CounterpartyChainID: "2", CounterpartyClientID: "cli-1-2",
		Params: map[string]any{
			"threshold": float64(2),
			"attestors": []any{watcherAddress, "0xUnresolvedAddress"},
		},
	})
	// stray client tracking another chain must not pair
	a.UpsertClient(manifest.Client{
		ClientID: "cli-1-9", Type: "attestation",
		CounterpartyChainID: "9", CounterpartyClientID: "cli-1-9",
	})
	// a second, custom-named connection between the same chain pair --
	// exercises the alias seqno suffix, and reuses the same attestor address
	// watching chain 2 to exercise cross-connection attestor deduplication.
	a.UpsertClient(manifest.Client{
		ClientID: "custom-a", Type: "attestation", Address: "0xca2",
		CounterpartyChainID: "2", CounterpartyClientID: "custom-b",
		Params: map[string]any{
			"threshold": float64(2),
			"attestors": []any{watcherAddress},
		},
	})
	b := manifest.New("2", "evm")
	b.Core.Router = "0xrouterB"
	b.UpsertClient(manifest.Client{
		ClientID: "cli-1-2", Type: "attestation", Address: "0xcb",
		CounterpartyChainID: "1", CounterpartyClientID: "cli-1-2",
		Params: map[string]any{"threshold": float64(2)},
	})
	b.UpsertClient(manifest.Client{
		ClientID: "custom-b", Type: "attestation", Address: "0xcb2",
		CounterpartyChainID: "1", CounterpartyClientID: "custom-a",
		// no configured attestors: must not surface as an attestors: entry
		Params: map[string]any{"threshold": float64(2)},
	})

	unreferencedSigner, _ := newLocalSignerConfig(t, "unused-signer")
	cfg := config.Config{
		Chains: []config.ChainConfig{
			{ChainID: "1", EVM: &config.EVMChainConfig{RPC: "http://a", ICS26Router: "0xstale"}},
		},
		Signers: config.Signers{watcherSigner, unreferencedSigner},
	}
	out, err := renderRelayConfig(cfg, a, b, "signer-a", "signer-b")
	require.NoError(t, err)

	// chain 1 is declared: its config is copied, router updated, rpc kept
	require.Len(t, out.Chains, 2)
	require.Equal(t, "1", out.Chains[0].ChainID)
	require.Equal(t, "0xrouterA", out.Chains[0].EVM.ICS26Router)
	require.Equal(t, "http://a", out.Chains[0].EVM.RPC)
	// chain 2 is undeclared: minimal entry with just the router
	require.Equal(t, "2", out.Chains[1].ChainID)
	require.Equal(t, "0xrouterB", out.Chains[1].EVM.ICS26Router)
	require.Empty(t, out.Chains[1].EVM.RPC)

	require.Len(t, out.Connections, 2)
	conn := out.Connections[0]
	require.Equal(t, "1-2", conn.Alias)
	require.Equal(t, "cli-1-2", conn.ClientA.ClientID)
	require.Equal(t, "1", conn.ClientA.ChainID)
	require.Equal(t, "cli-1-2", conn.ClientB.ClientID)
	require.Equal(t, "2", conn.ClientB.ChainID)
	require.Equal(t, "signer-a", conn.ClientA.Signer)
	require.NotNil(t, conn.ClientA.AutoRelay.Enabled)
	require.True(t, *conn.ClientA.AutoRelay.Enabled)
	require.NotNil(t, conn.ClientB.AutoRelay.Enabled)
	require.True(t, *conn.ClientB.AutoRelay.Enabled)
	require.Equal(t, "signer-b", conn.ClientB.Signer)

	// Reconciliation allocates unique aliases for new connections.
	conn2 := out.Connections[1]
	require.Equal(t, "1-2", conn2.Alias)
	require.Equal(t, "custom-a", conn2.ClientA.ClientID)
	require.Equal(t, "custom-b", conn2.ClientB.ClientID)

	// A's first client has two attestor addresses: one resolves to a
	// configured signer, one doesn't -- both still get an entry, watching
	// chain 2 (the chain that client tracks). A's second client (custom-a)
	// reuses watcherAddress and must not produce a duplicate entry.
	require.Len(t, out.Attestors, 2)
	require.Equal(t, "2", out.Attestors[0].ChainID)
	require.Equal(t, "attestor-2-"+watcherAddress, out.Attestors[0].Name)
	require.Equal(t, "attestor-watching-2", out.Attestors[0].Signer)
	require.Equal(t, config.AttestorTypeLocal, out.Attestors[0].Type)
	require.EqualValues(t, 1, out.Attestors[0].FinalityOffset)
	require.Equal(t, "attestor-2-0xUnresolvedAddress", out.Attestors[1].Name)
	require.Empty(t, out.Attestors[1].Signer)
	require.EqualValues(t, 1, out.Attestors[1].FinalityOffset)

	// no mutual pair: B has no client tracking A back
	empty := manifest.New("2", "evm")
	empty.Core.Router = "0xrouterB"
	_, err = renderRelayConfig(cfg, a, empty, "signer-a", "signer-b")
	require.ErrorContains(t, err, "no mutual client pair")

	// mismatched back-reference: B's client points at a different A client
	mismatched := manifest.New("2", "evm")
	mismatched.Core.Router = "0xrouterB"
	mismatched.UpsertClient(manifest.Client{
		ClientID: "cli-1", Type: "attestation",
		CounterpartyChainID: "1", CounterpartyClientID: "cli-other",
	})
	_, err = renderRelayConfig(cfg, a, mismatched, "signer-a", "signer-b")
	require.ErrorContains(t, err, "no mutual client pair")
}

// goccy silently drops comments whose path doesn't resolve, so assert the
// paths agree with the emitted document rather than just with each other.
// CollectComments' own logic is unit-tested directly in internal/config.
func TestRenderConfigEmitsComments(t *testing.T) {
	a := manifest.New("1", "evm")
	a.Core.Router = "0xrouterA"
	a.UpsertClient(manifest.Client{
		ClientID: "cli-1-2", Type: "attestation",
		CounterpartyChainID: "2", CounterpartyClientID: "cli-1-2",
		// unresolvable address: exercises the attestor signer TODO
		Params: map[string]any{"attestors": []any{"0xUnresolvedAddress"}},
	})
	b := manifest.New("2", "evm")
	// chain 2's router is left blank on purpose: exercises the ics26Router
	// TODO alongside the signer TODOs in the same render
	b.UpsertClient(manifest.Client{
		ClientID: "cli-1-2", Type: "attestation",
		CounterpartyChainID: "1", CounterpartyClientID: "cli-1-2",
	})

	out, err := renderRelayConfig(config.Config{}, a, b, "", "")
	require.NoError(t, err)

	merged, _, err := config.Config{}.ReconcileDeployment(out)
	require.NoError(t, err)

	rendered := captureStdout(t, func() {
		require.NoError(t, config.PrintYAMLWithComments(merged, config.CollectComments(merged)))
	})

	require.Contains(t, rendered, `signer: "" # TODO: signers[] alias that submits relay txs on chainA`)
	require.Contains(t, rendered, `signer: "" # TODO: signers[] alias that submits relay txs on chainB`)
	require.Contains(t, rendered, `ics26Router: "" # TODO: fill in`)
	require.Contains(t, rendered, `signer: "" # TODO: signers[] alias backing this attestor's key`)
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()
	require.NoError(t, w.Close())

	bz, err := io.ReadAll(r)
	require.NoError(t, err)

	return string(bz)
}

func TestRenderRelayConfigBesuQBFT(t *testing.T) {
	a := manifest.New("1", "evm")
	a.Core.Router = "0xrouterA"
	a.UpsertClient(manifest.Client{
		ClientID: "cli-1-2", Type: deploy.ClientTypeBesuQBFT, Address: "0xca",
		CounterpartyChainID: "2", CounterpartyClientID: "cli-1-2",
		Params: map[string]any{"ibcRouter": "0xrouterB", "trustingPeriod": float64(7200), "maxClockDrift": float64(60)},
	})
	b := manifest.New("2", "evm")
	b.Core.Router = "0xrouterB"
	b.UpsertClient(manifest.Client{
		ClientID: "cli-1-2", Type: deploy.ClientTypeBesuQBFT, Address: "0xcb",
		CounterpartyChainID: "1", CounterpartyClientID: "cli-1-2",
		Params: map[string]any{"ibcRouter": "0xrouterA", "trustingPeriod": float64(7200), "maxClockDrift": float64(60)},
	})

	out, err := renderRelayConfig(config.Config{}, a, b, "signer-a", "signer-b")
	require.NoError(t, err)
	require.Len(t, out.Connections, 1)
	require.Equal(t, config.ClientTypeBesuQBFT, out.Connections[0].ClientA.Type)
	require.Equal(t, config.ClientTypeBesuQBFT, out.Connections[0].ClientB.Type)
	require.Empty(t, out.Connections[0].ClientA.Params)
	require.Empty(t, out.Attestors, "besu-qbft clients need no attestors")
}

func TestBesuQBFTParamsBootstrap(t *testing.T) {
	previousDir := flagDeployManifestDir
	previousPeriod, previousDrift := flagDeployTrustingPeriod, flagDeployMaxClockDrift
	previousHeight := flagDeployHeight
	flagDeployManifestDir = t.TempDir()
	flagDeployHeight = 1
	t.Cleanup(func() {
		flagDeployManifestDir = previousDir
		flagDeployTrustingPeriod, flagDeployMaxClockDrift = previousPeriod, previousDrift
		flagDeployHeight = previousHeight
	})
	for _, tc := range []struct {
		name       string
		args       []string
		wantPeriod uint64
		router     string
		hostTime   uint64
		wantErr    string
		readsState bool // the error comes after the counterparty read
	}{
		{
			name: "omitted", router: "0x00000000000000000000000000000000000000bb",
			wantErr: "--trusting-period is required for a new besu-qbft client",
		},
		{name: "explicit zero", args: []string{"--trusting-period=0"}, wantErr: "--trusting-period must be positive"},
		{
			name: "finite without manifest", args: []string{"--trusting-period=2h"}, wantPeriod: 7200,
			router: "0x00000000000000000000000000000000000000bb",
		},
		{name: "missing router", args: []string{"--trusting-period=2h"}, router: "", wantErr: "evm.ics26Router"},
		{name: "malformed router", args: []string{"--trusting-period=2h"}, router: "bad", wantErr: "evm.ics26Router"},
		{name: "zero router", args: []string{"--trusting-period=2h"}, router: "0x0000000000000000000000000000000000000000", wantErr: "evm.ics26Router"},
		{
			name: "expired on this chain", args: []string{"--trusting-period=1h"},
			router: "0x00000000000000000000000000000000000000bb", hostTime: liveTimestamp + 3600,
			wantErr: "already older than the trusting period", readsState: true,
		},
		{
			name: "one second from expiry", args: []string{"--trusting-period=1h"}, wantPeriod: 3600,
			router: "0x00000000000000000000000000000000000000bb", hostTime: liveTimestamp + 3599,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flags := pflag.NewFlagSet("deploy client", pflag.ContinueOnError)
			flags.DurationVar(&flagDeployTrustingPeriod, flagNameTrustingPeriod, 0, "")
			flags.DurationVar(&flagDeployMaxClockDrift, flagNameMaxClockDrift, time.Minute, "")
			require.NoError(t, flags.Parse(tc.args))
			source := &bootstrapTarget{}
			host := &hostTarget{timestamp: liveTimestamp + 100}
			if tc.hostTime != 0 {
				host.timestamp = tc.hostTime
			}
			params, err := besuQBFTParams(t.Context(), tc.router, flags, host, source, "1", "2", "new-client")
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				require.Equal(t, tc.readsState, source.called)
				return
			}
			require.NoError(t, err)
			require.True(t, source.called)
			require.Equal(t, tc.wantPeriod, params.TrustingPeriod)
			require.Equal(t, tc.router, params.IBCRouter)
			require.Equal(t, uint64(60), params.MaxClockDrift)
			require.Equal(t, uint64(1), params.InitialHeight)
		})
	}
}

type bootstrapTarget struct {
	deploy.Target
	called bool
}

func (t *bootstrapTarget) BesuQBFTTrustedState(
	_ context.Context,
	height uint64,
) (deploy.BesuQBFTTrustedState, error) {
	t.called = true
	return deploy.BesuQBFTTrustedState{
		Height:     height,
		Timestamp:  liveTimestamp,
		StateRoot:  "0x1111111111111111111111111111111111111111111111111111111111111111",
		Validators: []string{"0x00000000000000000000000000000000000000ee"},
	}, nil
}

// A rerun rebuilds the constructor params from the manifest instead of the
// counterparty chain, whose historical state may already be pruned.
func TestBesuQBFTParamsReusesRecordedClient(t *testing.T) {
	dir := t.TempDir()
	previous := flagDeployManifestDir
	previousPeriod, previousDrift := flagDeployTrustingPeriod, flagDeployMaxClockDrift
	flagDeployManifestDir = dir
	t.Cleanup(func() {
		flagDeployManifestDir = previous
		flagDeployTrustingPeriod, flagDeployMaxClockDrift = previousPeriod, previousDrift
	})
	newFlags := func() *pflag.FlagSet {
		flags := pflag.NewFlagSet("deploy client", pflag.ContinueOnError)
		flags.DurationVar(&flagDeployTrustingPeriod, flagNameTrustingPeriod, 0, "")
		flags.DurationVar(&flagDeployMaxClockDrift, flagNameMaxClockDrift, time.Minute, "")
		return flags
	}

	recorded := deploy.BesuQBFTParams{
		IBCRouter:         "0x00000000000000000000000000000000000000cc",
		InitialHeight:     112,
		InitialTimestamp:  1788192445,
		InitialStateRoot:  "0x69c8d1758a0375ec0d4ee22f16e3119c84ecb3aaaaaaaaaaaaaaaaaaaaaaaaaa",
		InitialValidators: []string{"0x00000000000000000000000000000000000000aa"},
		TrustingPeriod:    7200,
		MaxClockDrift:     15,
	}
	m := manifest.New("1", "evm")
	m.Core.Router = "0xrouterA"
	m.UpsertClient(manifest.Client{
		ClientID: "cli-1-2", Type: deploy.ClientTypeBesuQBFT, Address: "0xca",
		CounterpartyChainID: "2", CounterpartyClientID: "cli-1-2",
		Params: map[string]any{
			"ibcRouter": recorded.IBCRouter, "initialHeight": float64(recorded.InitialHeight),
			"initialTimestamp": float64(recorded.InitialTimestamp), "initialStateRoot": recorded.InitialStateRoot,
			"initialValidators": []any{recorded.InitialValidators[0]},
			"trustingPeriod":    recorded.TrustingPeriod, "maxClockDrift": recorded.MaxClockDrift,
		},
	})
	require.NoError(t, m.Save(dir))

	flags := newFlags()
	require.NoError(t, flags.Parse([]string{"--trusting-period=2h"}))
	_, err := besuQBFTParams(
		context.Background(), recorded.IBCRouter, flags, nil, &sourcelessTarget{}, "1", "2", "cli-new",
	)
	require.ErrorContains(t, err, "cannot serve a besu-qbft trusted state")

	for _, tc := range []struct {
		name         string
		args         []string
		router       string
		wantParamErr string
		wantConflict string
	}{
		{name: "defaults preserve recorded settings"},
		{name: "matching settings", args: []string{"--trusting-period=2h", "--max-clock-drift=15s"}},
		{name: "changed period", args: []string{"--trusting-period=1h"}, wantConflict: "trustingPeriod"},
		{name: "changed router", router: "0x00000000000000000000000000000000000000dd", wantConflict: "ibcRouter"},
		{name: "missing router", router: "0x0000000000000000000000000000000000000000", wantParamErr: "evm.ics26Router"},
		{name: "explicit zero period", args: []string{"--trusting-period=0s"}, wantParamErr: "must be positive"},
		{name: "zero drift", args: []string{"--max-clock-drift=0s"}, wantConflict: "maxClockDrift"},
		{name: "explicit default drift", args: []string{"--max-clock-drift=60s"}, wantConflict: "maxClockDrift"},
		{name: "negative period", args: []string{"--trusting-period=-1s"}, wantParamErr: "must not be negative"},
		{name: "fractional period", args: []string{"--trusting-period=500ms"}, wantParamErr: "must be whole seconds"},
		{name: "negative drift", args: []string{"--max-clock-drift=-1s"}, wantParamErr: "must not be negative"},
		{name: "fractional drift", args: []string{"--max-clock-drift=500ms"}, wantParamErr: "must be whole seconds"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flags := newFlags()
			require.NoError(t, flags.Parse(tc.args))
			router := tc.router
			if router == "" {
				router = recorded.IBCRouter
			}
			params, err := besuQBFTParams(
				context.Background(), router, flags, &registeredClientTarget{}, nil, "1", "2", "cli-1-2",
			)
			if tc.wantParamErr != "" {
				require.ErrorContains(t, err, tc.wantParamErr)
				return
			}
			require.NoError(t, err)

			spec := deploy.ClientSpec{
				ClientID: "cli-1-2", Type: deploy.ClientTypeBesuQBFT,
				CounterpartyChainID: "2", CounterpartyClientID: "cli-1-2", Params: params,
			}
			steps := deploy.ClientSteps(&registeredClientTarget{}, dir, "1", spec)
			done, err := steps[0].Done(context.Background())
			if tc.wantConflict != "" {
				require.ErrorContains(t, err, tc.wantConflict)
				require.False(t, done)
				return
			}
			require.NoError(t, err)
			require.True(t, done)
			require.Equal(t, recorded, params)
		})
	}

	// the chain no longer knows the client (reset): the trusted state is read
	// live again, with the recorded trust settings as defaults
	t.Run("recorded but unregistered bootstraps live", func(t *testing.T) {
		previousHeight := flagDeployHeight
		flagDeployHeight = 1
		t.Cleanup(func() { flagDeployHeight = previousHeight })

		for _, tc := range []struct {
			name              string
			args              []string
			wantPeriod, drift uint64
		}{
			{name: "recorded trust settings", wantPeriod: recorded.TrustingPeriod, drift: recorded.MaxClockDrift},
			{
				name: "overridden trust settings", args: []string{"--trusting-period=1h", "--max-clock-drift=30s"},
				wantPeriod: 3600, drift: 30,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				flags := newFlags()
				require.NoError(t, flags.Parse(tc.args))
				source := &bootstrapTarget{}
				params, err := besuQBFTParams(
					context.Background(),
					recorded.IBCRouter,
					flags,
					&hostTarget{timestamp: liveTimestamp + 100},
					source,
					"1",
					"2",
					"cli-1-2",
				)
				require.NoError(t, err)
				require.True(t, source.called)

				live, _ := source.BesuQBFTTrustedState(context.Background(), 1)
				require.Equal(t, deploy.BesuQBFTParams{
					IBCRouter:         recorded.IBCRouter,
					InitialHeight:     live.Height,
					InitialTimestamp:  live.Timestamp,
					InitialStateRoot:  live.StateRoot,
					InitialValidators: live.Validators,
					TrustingPeriod:    tc.wantPeriod,
					MaxClockDrift:     tc.drift,
				}, params)
			})
		}
	})
}

// sourcelessTarget is a deploy.Target that is not a deploy.BesuQBFTSource.
type sourcelessTarget struct{ deploy.Target }

type registeredClientTarget struct{ deploy.Target }

func (*registeredClientTarget) ClientRegistered(context.Context, string, string) (string, bool, error) {
	return "0xca", true, nil
}

// hostTarget is a host chain without the client, whose head sits at timestamp.
type hostTarget struct {
	deploy.Target
	timestamp uint64
}

func (*hostTarget) ClientRegistered(context.Context, string, string) (string, bool, error) {
	return "", false, nil
}

func (h *hostTarget) Head(context.Context) (uint64, uint64, error) {
	return 0, h.timestamp, nil
}

// liveTimestamp is the trusted-state timestamp bootstrapTarget serves.
const liveTimestamp = 1788200000

func TestRecordedClientLoadFailuresAreNotBootstrapFallbacks(t *testing.T) {
	previous := flagDeployManifestDir
	t.Cleanup(func() { flagDeployManifestDir = previous })
	for _, mode := range []string{"missing", "corrupt", "unreadable", "missing-client"} {
		t.Run(mode, func(t *testing.T) {
			flagDeployManifestDir = t.TempDir()
			path := manifest.Path(flagDeployManifestDir, "1")
			switch mode {
			case "corrupt":
				require.NoError(t, os.WriteFile(path, []byte("{"), 0o600))
			case "unreadable":
				require.NoError(t, os.Mkdir(path, 0o700)) // deterministic even when running as root
			case "missing-client":
				require.NoError(t, manifest.New("1", "evm").Save(flagDeployManifestDir))
			}
			_, _, found, err := recordedClient("1", "client")
			require.False(t, found)
			if mode == "corrupt" || mode == "unreadable" {
				require.ErrorContains(t, err, path)
				_, err = besuQBFTParams(
					t.Context(),
					"0x00000000000000000000000000000000000000cc",
					pflag.NewFlagSet("test", pflag.ContinueOnError),
					nil,
					&sourcelessTarget{},
					"1",
					"2",
					"client",
				)
				require.ErrorContains(t, err, path) // would report unavailable proof source if swallowed
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRenderConfigRejectsDuplicateSignerAliasesBeforeRendering(t *testing.T) {
	home := setupRenderConfigTest(t)
	cfg := config.DefaultConfig()
	// No credentials or manifests are needed to detect ambiguous aliases.
	cfg.Signers = config.Signers{{Alias: "key"}, {Alias: "key"}}
	path := filepath.Join(home, "ibc.yml")
	require.NoError(t, cfg.StoreToFile(path))
	original, err := os.ReadFile(path)
	require.NoError(t, err)
	flagDeployRenderSignerA = "key"
	require.ErrorContains(t, deployRenderConfig(nil, []string{"1", "2"}), "duplicate alias")
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, original, got)
}

func TestPopulatePermitsIncompleteDraft(t *testing.T) {
	previous := globalFlags
	t.Cleanup(func() { globalFlags = previous })
	globalFlags = config.DefaultFlagSet()
	globalFlags.Home = t.TempDir()
	globalFlags.Quiet = true
	cfg := config.DefaultConfig()
	cfg.Chains = config.Chains{{ChainID: "1", EVM: &config.EVMChainConfig{}}}
	require.Error(t, cfg.Validate())
	stderr := os.Stderr
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = writer
	t.Cleanup(func() { os.Stderr = stderr })
	require.NoError(t, populateRenderedConfig(cfg))
	require.NoError(t, writer.Close())
	os.Stderr = stderr
	warning, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	require.Contains(t, string(warning), "Config is not runnable yet:")
	path, err := globalFlags.ConfigPath()
	require.NoError(t, err)
	loaded, err := config.LoadFromFile(path, false)
	require.NoError(t, err)
	require.Len(t, loaded.Chains, 1)
}

func TestRenderConfigCommandPreservesUnspecifiedSigners(t *testing.T) {
	setupRenderConfigTest(t)

	key, _ := newLocalSignerConfig(t, "existing-relayer")
	cfg := config.DefaultConfig()
	cfg.Signers = []config.SignerConfig{key}
	for _, id := range []string{"1", "2"} {
		cfg.Chains = append(cfg.Chains, config.ChainConfig{
			ChainID: id,
			EVM: &config.EVMChainConfig{
				RPC:         "http://localhost:8545",
				ICS26Router: "0x0000000000000000000000000000000000000001",
			},
		})
	}
	cfg.Relayer.Connections = []config.ConnectionConfig{{
		Alias:   "custom-route",
		ClientA: config.ClientEnd{ChainID: "2", ClientID: "b", Signer: key.Alias, Type: config.ClientTypeAttestation},
		ClientB: config.ClientEnd{ChainID: "1", ClientID: "a", Signer: key.Alias, Type: config.ClientTypeAttestation},
	}}
	require.NoError(t, cfg.Validate())
	path, err := globalFlags.ConfigPath()
	require.NoError(t, err)
	require.NoError(t, cfg.StoreToFile(path))
	for _, pair := range [][4]string{{"1", "a", "2", "b"}, {"2", "b", "1", "a"}} {
		m := manifest.New(pair[0], "evm")
		m.Core.Router = cfg.Chains[0].EVM.ICS26Router
		m.UpsertClient(manifest.Client{
			ClientID: pair[1], CounterpartyChainID: pair[2], CounterpartyClientID: pair[3],
			Type: deploy.ClientTypeAttestation,
		})
		require.NoError(t, m.Save(flagDeployManifestDir))
	}
	require.NoError(t, deployRenderConfig(nil, []string{"1", "2"}))
	loaded, err := config.LoadFromFile(path, true)
	require.NoError(t, err)
	require.Equal(t, cfg.Relayer.Connections, loaded.Relayer.Connections)

	flagDeployRenderSignerA = "unknown"
	require.ErrorContains(t, deployRenderConfig(nil, []string{"1", "2"}), "signer \"unknown\" not found")
}

func TestRenderConfigCommandRepairsPopulatedDraft(t *testing.T) {
	home := setupRenderConfigTest(t)
	run := func(signerA, signerB string) error {
		// Each invocation starts with normal validation defaults, like a new CLI process.
		globalFlags = config.DefaultFlagSet()
		globalFlags.Home = home
		globalFlags.Quiet = true
		flagDeployRenderSignerA, flagDeployRenderSignerB = signerA, signerB
		return deployRenderConfig(nil, []string{"1", "2"})
	}
	relayerKey, _ := newLocalSignerConfig(t, "relayer")
	attestorKey, attestorAddress := newLocalSignerConfig(t, "watcher-key")
	cfg := config.DefaultConfig()
	cfg.Signers = config.Signers{relayerKey}
	for _, id := range []string{"1", "2"} {
		cfg.Chains = append(cfg.Chains, config.ChainConfig{
			ChainID: id,
			EVM: &config.EVMChainConfig{
				RPC: "http://localhost:8545", WS: "ws://localhost:8546",
				ICS26Router: "0x0000000000000000000000000000000000000001",
			},
		})
	}
	path := filepath.Join(home, "ibc.yml")
	require.NoError(t, cfg.Validate())
	require.NoError(t, cfg.StoreToFile(path))
	for _, pair := range [][4]string{{"1", "a", "2", "b"}, {"2", "b", "1", "a"}} {
		m := manifest.New(pair[0], "evm")
		m.Core.Router = cfg.Chains[0].EVM.ICS26Router
		m.UpsertClient(manifest.Client{
			ClientID: pair[1], CounterpartyChainID: pair[2], CounterpartyClientID: pair[3],
			Type:   deploy.ClientTypeAttestation,
			Params: map[string]any{"attestors": []any{attestorAddress}},
		})
		require.NoError(t, m.Save(flagDeployManifestDir))
	}

	require.NoError(t, run("", ""))
	draft, err := config.LoadFromFile(path, false)
	require.NoError(t, err)
	require.Error(t, draft.Validate())
	require.Len(t, draft.Relayer.Connections, 1)
	require.Empty(t, draft.Relayer.Connections[0].ClientA.Signer)
	require.Empty(t, draft.Relayer.Connections[0].ClientB.Signer)
	require.Len(t, draft.Attestors, 2)
	for _, a := range draft.Attestors {
		require.Empty(t, a.Signer)
	}

	// A key becomes available after the draft was written; preserve user settings.
	draft.Signers = append(draft.Signers, attestorKey)
	draft.Relayer.Connections[0].Alias = "custom-route"
	draft.Attestors[0].FinalityOffset = 42
	require.NoError(t, draft.StoreToFile(path))
	require.NoError(t, run(relayerKey.Alias, relayerKey.Alias))
	repaired, err := config.LoadFromFile(path, true)
	require.NoError(t, err)
	require.Len(t, repaired.Relayer.Connections, 1)
	require.Equal(t, "custom-route", repaired.Relayer.Connections[0].Alias)
	require.Equal(t, relayerKey.Alias, repaired.Relayer.Connections[0].ClientA.Signer)
	require.Equal(t, relayerKey.Alias, repaired.Relayer.Connections[0].ClientB.Signer)
	require.Len(t, repaired.Attestors, 2)
	for i, a := range repaired.Attestors {
		require.Equal(t, attestorKey.Alias, a.Signer)
		require.Equal(t, draft.Attestors[i].Name, a.Name)
		require.Equal(t, draft.Attestors[i].FinalityOffset, a.FinalityOffset)
	}
	require.NoError(t, run("", ""))
	repeated, err := config.LoadFromFile(path, true)
	require.NoError(t, err)
	require.Equal(t, repaired, repeated)

	// Draft loading still rejects identity conflicts and unknown YAML fields before writing.
	duplicate := repaired.Attestors[0]
	duplicate.Name = "duplicate"
	repaired.Attestors = append(repaired.Attestors, duplicate)
	require.NoError(t, repaired.StoreToFile(path))
	invalid, err := os.ReadFile(path)
	require.NoError(t, err)
	require.ErrorContains(t, run("", ""), "duplicate local attestor signer")
	unchanged, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, invalid, unchanged)

	invalid = append(invalid, []byte("\nunknownField: true\n")...)
	require.NoError(t, os.WriteFile(path, invalid, 0o600))
	require.Error(t, run("", ""))
	unchanged, err = os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, invalid, unchanged)
}

func setupRenderConfigTest(t *testing.T) string {
	t.Helper()
	previousFlags := globalFlags
	previousDir := flagDeployManifestDir
	previousA, previousB := flagDeployRenderSignerA, flagDeployRenderSignerB
	previousPopulate, previousYes := flagDeployRenderPopulate, flagDeployYes
	t.Cleanup(func() {
		globalFlags = previousFlags
		flagDeployManifestDir = previousDir
		flagDeployRenderSignerA, flagDeployRenderSignerB = previousA, previousB
		flagDeployRenderPopulate, flagDeployYes = previousPopulate, previousYes
	})
	home := t.TempDir()
	t.Chdir(home)
	globalFlags = config.DefaultFlagSet()
	globalFlags.Home = home
	globalFlags.Quiet = true
	flagDeployManifestDir = filepath.Join(home, "deployments")
	flagDeployRenderSignerA, flagDeployRenderSignerB = "", ""
	flagDeployRenderPopulate, flagDeployYes = true, true

	return home
}
