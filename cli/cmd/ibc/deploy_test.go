// SPDX-License-Identifier: Apache-2.0

package main

import (
	"cmp"
	"context"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc-contracts/packages/go-abigen/besumsgs"
	"github.com/cosmos/ibc/cli/besu"
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
	a.Core.Multicall = "0xmulticallA"
	a.UpsertClient(manifest.Client{
		ClientID: "cli-1-2", Address: "0xca",
		CounterpartyChainID: "2", CounterpartyClientID: "cli-1-2",
		Attestation: &manifest.AttestationParams{
			Threshold: 2,
			Attestors: []string{watcherAddress, "0xUnresolvedAddress"},
		},
	})
	// stray client tracking another chain must not pair
	a.UpsertClient(manifest.Client{
		ClientID:            "cli-1-9",
		CounterpartyChainID: "9", CounterpartyClientID: "cli-1-9",
	})
	// a second, custom-named connection between the same chain pair --
	// exercises the alias seqno suffix, and reuses the same attestor address
	// watching chain 2 to exercise cross-connection attestor deduplication.
	a.UpsertClient(manifest.Client{
		ClientID: "custom-a", Address: "0xca2",
		CounterpartyChainID: "2", CounterpartyClientID: "custom-b",
		Attestation: &manifest.AttestationParams{
			Threshold: 2,
			Attestors: []string{watcherAddress},
		},
	})
	b := manifest.New("2", "evm")
	b.Core.Router = "0xrouterB"
	b.Core.Multicall = "0xmulticallB"
	b.UpsertClient(manifest.Client{
		ClientID: "cli-1-2", Address: "0xcb",
		CounterpartyChainID: "1", CounterpartyClientID: "cli-1-2",
		Attestation: &manifest.AttestationParams{Threshold: 2},
	})
	b.UpsertClient(manifest.Client{
		ClientID: "custom-b", Address: "0xcb2",
		CounterpartyChainID: "1", CounterpartyClientID: "custom-a",
		// no configured attestors: must not surface as an attestors: entry
		Attestation: &manifest.AttestationParams{Threshold: 2},
	})

	unreferencedSigner, _ := newLocalSignerConfig(t, "unused-signer")
	cfg := config.Config{
		Chains: []config.ChainConfig{
			{
				ChainID: "1",
				EVM: &config.EVMChainConfig{
					RPC:          "http://a",
					ICS26Router:  "0xstale",
					IBCMulticall: "0xstaleMulticall",
				},
			},
		},
		Signers: config.Signers{watcherSigner, unreferencedSigner},
	}
	out, err := renderRelayConfig(cfg, a, b, "signer-a", "signer-b")
	require.NoError(t, err)

	// chain 1 is declared: its config is copied, router updated, rpc kept
	require.Len(t, out.Chains, 2)
	require.Equal(t, "1", out.Chains[0].ChainID)
	require.Equal(t, "0xrouterA", out.Chains[0].EVM.ICS26Router)
	require.Equal(t, "0xmulticallA", out.Chains[0].EVM.IBCMulticall)
	require.Equal(t, "http://a", out.Chains[0].EVM.RPC)
	// chain 2 is undeclared: minimal entry with the deployed addresses
	require.Equal(t, "2", out.Chains[1].ChainID)
	require.Equal(t, "0xrouterB", out.Chains[1].EVM.ICS26Router)
	require.Equal(t, "0xmulticallB", out.Chains[1].EVM.IBCMulticall)
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
		ClientID:            "cli-1",
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
		ClientID:            "cli-1-2",
		CounterpartyChainID: "2", CounterpartyClientID: "cli-1-2",
		// unresolvable address: exercises the attestor signer TODO
		Attestation: &manifest.AttestationParams{Attestors: []string{"0xUnresolvedAddress"}},
	})
	b := manifest.New("2", "evm")
	// chain 2's router is left blank on purpose: exercises the ics26Router
	// TODO alongside the signer TODOs in the same render
	b.UpsertClient(manifest.Client{
		ClientID:            "cli-1-2",
		CounterpartyChainID: "1", CounterpartyClientID: "cli-1-2",
		Attestation: &manifest.AttestationParams{},
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
	require.Contains(t, rendered, `ibcMulticall: "" # TODO: fill in`)
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

func TestRenderRelayConfigBesu(t *testing.T) {
	a := manifest.New("1", "evm")
	a.Core.Router = "0xrouterA"
	a.UpsertClient(manifest.Client{
		ClientID: "cli-1-2", Address: "0xca",
		CounterpartyChainID: "2", CounterpartyClientID: "cli-1-2",
		Besu: &manifest.BesuParams{IBCRouter: "0xrouterB", TrustingPeriod: 7200, MaxClockDrift: 60},
	})
	b := manifest.New("2", "evm")
	b.Core.Router = "0xrouterB"
	b.UpsertClient(manifest.Client{
		ClientID: "cli-1-2", Address: "0xcb",
		CounterpartyChainID: "1", CounterpartyClientID: "cli-1-2",
		Besu: &manifest.BesuParams{IBCRouter: "0xrouterA", TrustingPeriod: 7200, MaxClockDrift: 60},
	})

	out, err := renderRelayConfig(config.Config{}, a, b, "signer-a", "signer-b")
	require.NoError(t, err)
	require.Len(t, out.Connections, 1)
	require.Equal(t, config.ClientTypeBesu, out.Connections[0].ClientA.Type)
	require.Equal(t, config.ClientTypeBesu, out.Connections[0].ClientB.Type)
	require.Empty(t, out.Connections[0].ClientA.Params)
	require.Empty(t, out.Attestors, "besu clients need no attestors")
}

func TestBesuParamsFlags(t *testing.T) {
	previousPeriod, previousDrift, previousLevel := flagDeployTrustingPeriod, flagDeployMaxClockDrift, flagDeployTrustLevel
	t.Cleanup(func() {
		flagDeployTrustingPeriod, flagDeployMaxClockDrift, flagDeployTrustLevel = previousPeriod, previousDrift, previousLevel
	})
	const router = "0x00000000000000000000000000000000000000bb"
	for _, tc := range []struct {
		name      string
		period    time.Duration
		drift     time.Duration
		level     string
		router    string
		wantLevel besumsgs.IBesuLightClientMsgsTrustThreshold
		wantErr   string
	}{
		{name: "zero", router: router, wantErr: "--trusting-period must be positive"},
		{name: "negative drift", period: time.Hour, drift: -time.Second, router: router, wantErr: "--max-clock-drift: must not be negative"},
		{name: "finite", period: 2 * time.Hour, router: router},
		{
			name: "trust level", period: 2 * time.Hour, level: "1/2", router: router,
			wantLevel: besumsgs.IBesuLightClientMsgsTrustThreshold{Numerator: 1, Denominator: 2},
		},
		{name: "bad trust level", period: 2 * time.Hour, level: "1/4", router: router, wantErr: "--trust-level: must be from 1/3 to 1"},
		{name: "missing router", period: 2 * time.Hour, router: "", wantErr: "evm.ics26Router: required"},
		{name: "malformed router", period: 2 * time.Hour, router: "bad", wantErr: "evm.ics26Router: invalid EVM address"},
		{
			name: "zero router", period: 2 * time.Hour,
			router: "0x0000000000000000000000000000000000000000", wantErr: "evm.ics26Router: must not be the zero address",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flagDeployTrustingPeriod, flagDeployMaxClockDrift, flagDeployTrustLevel = tc.period, time.Minute, cmp.Or(
				tc.level,
				"2/3",
			)
			if tc.drift != 0 {
				flagDeployMaxClockDrift = tc.drift
			}
			source := &besuHeadTarget{}
			params, err := besuParams(t.Context(), tc.router, source)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				require.False(t, source.called)
				return
			}
			require.NoError(t, err)
			require.True(t, source.called)
			require.Equal(t, deploy.BesuParams{
				Consensus:             besu.IBFT2,
				IBCRouter:             common.HexToAddress(tc.router),
				InitialHeight:         besuHeadHeight,
				InitialConsensusState: besumsgs.IBesuLightClientMsgsConsensusState{Timestamp: besuHeadTimestamp},
				TrustingPeriod:        uint64(tc.period / time.Second),
				MaxClockDrift:         60,
				TrustLevel:            cmp.Or(tc.wantLevel, besu.DefaultTrustLevel),
			}, params)
		})
	}

	_, err := besuParams(t.Context(), router, &hostOnlyTarget{})
	require.ErrorContains(t, err, "cannot serve a besu consensus state")
}

func TestIFTRateLimit(t *testing.T) {
	const (
		deployer = "0x00000000000000000000000000000000000000aA"
		other    = "0x00000000000000000000000000000000000000bb"
	)
	for _, tc := range []struct {
		name     string
		owner    string
		capacity string
		window   time.Duration // zero means the flag was not given
		want     *deploy.IFTRateLimit
		wantErr  string
	}{
		{
			name: "explicit", owner: deployer, capacity: "1000", window: time.Hour,
			want: &deploy.IFTRateLimit{Capacity: big.NewInt(1000), Window: 3600},
		},
		{
			name: "default window", owner: deployer, capacity: "1000",
			want: &deploy.IFTRateLimit{Capacity: big.NewInt(1000), Window: 86400},
		},
		{
			name: "owner spelled differently", owner: strings.ToLower(deployer[2:]), capacity: "1000",
			want: &deploy.IFTRateLimit{Capacity: big.NewInt(1000), Window: 86400},
		},
		{name: "capacity required", owner: deployer, wantErr: "--rate-limit-capacity is required"},
		{name: "other owner", owner: other},
		{name: "other owner with capacity", owner: other, capacity: "1", wantErr: "only the owner"},
		{name: "other owner with window", owner: other, window: time.Hour, wantErr: "only the owner"},
		{name: "zero capacity", owner: deployer, capacity: "0", wantErr: "must be a positive uint208"},
		{name: "malformed capacity", owner: deployer, capacity: "1e18", wantErr: "must be a positive uint208"},
		{
			name: "capacity overflow", owner: deployer, capacity: new(big.Int).Lsh(big.NewInt(1), 208).String(),
			wantErr: "must be a positive uint208",
		},
		{name: "sub-second window", owner: deployer, capacity: "1", window: time.Millisecond, wantErr: "must be at least 1s"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldCapacity, oldWindow := flagDeployIFTCapacity, flagDeployIFTWindow
			t.Cleanup(func() { flagDeployIFTCapacity, flagDeployIFTWindow = oldCapacity, oldWindow })
			flagDeployIFTCapacity, flagDeployIFTWindow = tc.capacity, 24*time.Hour
			if tc.window != 0 {
				flagDeployIFTWindow = tc.window
			}

			got, err := iftRateLimit(tc.owner, deployer, tc.window != 0)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestWholeSeconds(t *testing.T) {
	for _, tc := range []struct {
		d       time.Duration
		want    uint64
		wantErr string
	}{
		{d: 2 * time.Hour, want: 7200},
		{d: 0, want: 0},
		{d: -time.Second, wantErr: "must not be negative"},
		{d: 1500 * time.Millisecond, want: 1},
	} {
		got, err := wholeSeconds(tc.d)
		if tc.wantErr != "" {
			require.ErrorContains(t, err, tc.wantErr, tc.d)
			continue
		}
		require.NoError(t, err, tc.d)
		require.Equal(t, tc.want, got, tc.d)
	}
}

func TestTrustLevel(t *testing.T) {
	for _, tc := range []struct {
		s       string
		want    besumsgs.IBesuLightClientMsgsTrustThreshold
		wantErr string
	}{
		{s: "2/3", want: besumsgs.IBesuLightClientMsgsTrustThreshold{Numerator: 2, Denominator: 3}},
		{s: "1/3", want: besumsgs.IBesuLightClientMsgsTrustThreshold{Numerator: 1, Denominator: 3}},
		{s: "1/2", want: besumsgs.IBesuLightClientMsgsTrustThreshold{Numerator: 1, Denominator: 2}},
		{s: "1/1", want: besumsgs.IBesuLightClientMsgsTrustThreshold{Numerator: 1, Denominator: 1}},
		{s: "85/255", want: besumsgs.IBesuLightClientMsgsTrustThreshold{Numerator: 85, Denominator: 255}},
		{s: "1/4", wantErr: "must be from 1/3 to 1"},
		{s: "0/1", wantErr: "must be from 1/3 to 1"},
		{s: "3/2", wantErr: "must be from 1/3 to 1"},
		{s: "1/0", wantErr: "denominator must not be zero"},
		{s: "0/0", wantErr: "denominator must not be zero"},
		{s: "", wantErr: "want n/d"},
		{s: "2", wantErr: "want n/d"},
		{s: "0.5", wantErr: "want n/d"},
		{s: "1/2/3", wantErr: "want n/d"},
		{s: " 1/2", wantErr: "want n/d"},
		{s: "-1/2", wantErr: "want n/d"},
		{s: "256/256", wantErr: "want n/d"},
	} {
		got, err := trustLevel(tc.s)
		if tc.wantErr != "" {
			require.ErrorContains(t, err, tc.wantErr, tc.s)
			continue
		}
		require.NoError(t, err, tc.s)
		require.Equal(t, tc.want, got, tc.s)
	}
}

// The flag's default is the client's default trust level.
func TestTrustLevelFlagDefault(t *testing.T) {
	got, err := trustLevel(cmdDeployClientBesu.Flags().Lookup(flagNameTrustLevel).DefValue)
	require.NoError(t, err)
	require.Equal(t, besu.DefaultTrustLevel, got)
}

const (
	besuHeadHeight    = 112
	besuHeadTimestamp = 1788200000
)

// besuHeadTarget is an IBFT2 Besu chain whose head is besuHeadHeight.
type besuHeadTarget struct {
	deploy.Target
	called bool
}

func (t *besuHeadTarget) BesuHead(context.Context) (*besu.ParsedHeader, error) {
	t.called = true
	return &besu.ParsedHeader{Height: besuHeadHeight, Timestamp: besuHeadTimestamp, Consensus: besu.IBFT2}, nil
}

// hostOnlyTarget is a chain that cannot serve Besu consensus states.
type hostOnlyTarget struct {
	deploy.Target
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
			Attestation: &manifest.AttestationParams{},
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
			Attestation: &manifest.AttestationParams{Attestors: []string{attestorAddress}},
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
