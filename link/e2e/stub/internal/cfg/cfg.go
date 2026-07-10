// Package cfg loads the IBC Link config the mock consumes.
package cfg

import (
	"fmt"
	"os"

	"github.com/cosmos/ibc/link/e2e/stub/internal/exitcode"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/internal/config"
)

// Setup resolves --home/--config/--db, chdirs to home, and loads the wire config view (the real
// binary's config machinery is used for path/DB resolution only).
func Setup(flags *config.FlagSet) (*wire.ConfigYAML, error) {
	home, err := config.ExpandHome(flags.Home)
	if err != nil {
		return nil, setupError(fmt.Errorf("home: %w", err))
	}

	path, err := flags.ConfigPath()
	if err != nil {
		return nil, setupError(fmt.Errorf("unable to get config path: %w", err))
	}

	if ensureErr := config.EnsureDirectory(path); ensureErr != nil {
		return nil, setupError(fmt.Errorf("unable to create home directory %s: %w", home, ensureErr))
	}

	if chdirErr := os.Chdir(home); chdirErr != nil {
		return nil, setupError(
			fmt.Errorf("unable to change working directory to %s: %w", home, chdirErr),
		)
	}

	realCfg, err := config.LoadFromFile(path, true, false)
	if err != nil {
		return nil, setupError(err)
	}

	if flags.DB != "" {
		realCfg.DB, err = config.DBConfigFromURL(flags.DB)
		if err != nil {
			return nil, setupError(fmt.Errorf("invalid --db: %w", err))
		}
	}

	wireCfg, err := Load(path)
	if err != nil {
		return nil, setupError(err)
	}
	wireCfg.DB = wire.DB{Type: realCfg.DB.Type, URL: realCfg.DB.URL}

	return wireCfg, nil
}

func setupError(err error) error {
	return exitcode.New(wire.ExitConfigInvalid, err)
}

// Load reads the config file at path, unmarshals it into wire.ConfigYAML, and resolves the environment
// references carried by EVM RPC URLs. The returned config is resolved in memory; the file is never
// rewritten.
func Load(path string) (*wire.ConfigYAML, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}
	c, err := wire.Unmarshal(data)
	if err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}
	for i := range c.Chains {
		c.Chains[i].RPC.URL, err = config.ExpandEnvRefs(c.Chains[i].RPC.URL)
		if err != nil {
			return nil, fmt.Errorf("expand chains[%d].rpc.url: %w", i, err)
		}
	}
	return c, nil
}

// RequireStore validates the configured store.
func RequireStore(c *wire.ConfigYAML) error {
	if err := wire.ValidateDB(c.DB); err != nil {
		return fmt.Errorf("db: %w", err)
	}
	return nil
}
