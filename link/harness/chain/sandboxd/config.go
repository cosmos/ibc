package sandboxd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"

	cmtcfg "github.com/cometbft/cometbft/config"
)

// patchConfigTOML applies the localnet recipe's CometBFT config.toml edits through cometbft's typed config
// package: load, modify, write. The pinned v0.39.3 round-trip is faithful to real `sandboxd init` output: all
// 114 keys survive, including the fork's [p2p.libp2p*] sections. Consensus and broadcast timeouts are shortened
// for sub-second blocks. The pprof listener is disabled to avoid the fixed localhost:6060 cross-instance
// collision.
func patchConfigTOML(path string) error {
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("sandboxd: read config.toml %s: %w", path, err)
	}
	conf := cmtcfg.DefaultConfig()
	// viper's default decoder carries the string->time.Duration hook, so the "3s"/"500ms" literals decode
	// into the typed duration fields.
	if err := v.Unmarshal(conf); err != nil {
		return fmt.Errorf("sandboxd: parse config.toml %s: %w", path, err)
	}

	conf.Consensus.TimeoutPropose = 2 * time.Second
	conf.Consensus.TimeoutProposeDelta = 200 * time.Millisecond
	conf.Consensus.TimeoutPrevote = 500 * time.Millisecond
	conf.Consensus.TimeoutPrevoteDelta = 200 * time.Millisecond
	conf.Consensus.TimeoutPrecommit = 500 * time.Millisecond
	conf.Consensus.TimeoutPrecommitDelta = 200 * time.Millisecond
	conf.Consensus.TimeoutCommit = 500 * time.Millisecond
	conf.RPC.TimeoutBroadcastTxCommit = 5 * time.Second
	conf.RPC.PprofListenAddress = "" // disable pprof (avoid the fixed localhost:6060 cross-instance collision)

	return writeCometConfig(path, conf)
}

// writeCometConfig renders conf back through cometbft's config template. cmtcfg.WriteConfigFile panics on a
// template or write failure (it has no error return), so it is wrapped to recover that into a normal error and
// keep the harness's returned-error convention (a broken write is a hard failure, never swallowed).
func writeCometConfig(path string, conf *cmtcfg.Config) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("sandboxd: write config.toml %s: %v", path, r)
		}
	}()
	cmtcfg.WriteConfigFile(path, conf)
	return nil
}

// patchAppTOML line-patches app.toml because sdk server/config drops sandboxd's [evm], [evm.mempool],
// [json-rpc], and [tls] surface on typed round-trip. Verified against real `sandboxd init` output, that loses
// 39 keys, including evm.evm-chain-id and json-rpc.address. The exact-line target is a drift guard for the
// pinned node default. Combined with feemarket no_base_fee, 0<denom> admits the zero-gas-price txs the stub and
// harness send.
func patchAppTOML(path, denom string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("sandboxd: read app.toml %s: %w", path, err)
	}
	target := `minimum-gas-prices = "0aatom"`
	s := string(data)
	if !strings.Contains(s, target) {
		return fmt.Errorf(
			"sandboxd: app.toml: expected line %q not found (pinned node default drifted from the recipe)",
			target,
		)
	}
	s = strings.Replace(s, target, fmt.Sprintf(`minimum-gas-prices = "0%s"`, denom), 1)
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		return fmt.Errorf("sandboxd: write app.toml %s: %w", path, err)
	}
	return nil
}
