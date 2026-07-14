// Package cfg loads config for the stub.
package cfg

import (
	"fmt"
	"os"

	"github.com/cosmos/ibc/e2e/internal/harness/ibclink/wire"
	"github.com/cosmos/ibc/e2e/stub/internal/exitcode"
)

func Setup(flags *FlagSet) (*wire.ConfigYAML, error) {
	home, err := ExpandHome(flags.Home)
	if err != nil {
		return nil, setupError(fmt.Errorf("home: %w", err))
	}

	path, err := flags.ConfigPath()
	if err != nil {
		return nil, setupError(fmt.Errorf("unable to get config path: %w", err))
	}

	if chdirErr := os.Chdir(home); chdirErr != nil {
		return nil, setupError(
			fmt.Errorf("unable to change working directory to %s: %w", home, chdirErr),
		)
	}

	c, err := Load(path)
	if err != nil {
		return nil, setupError(err)
	}
	if flags.DB != "" {
		db, dbErr := dbConfigFromPath(flags.DB)
		if dbErr != nil {
			return nil, setupError(fmt.Errorf("invalid --db: %w", dbErr))
		}
		c.DB = db
	}

	return c, nil
}

func setupError(err error) error {
	return exitcode.New(wire.ExitConfigInvalid, err)
}

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
		c.Chains[i].RPC.URL, err = resolveEnvRef(c.Chains[i].RPC.URL)
		if err != nil {
			return nil, fmt.Errorf("expand chains[%d].rpc.url: %w", i, err)
		}
	}
	for i := range c.Signers {
		c.Signers[i].File, err = resolveEnvRef(c.Signers[i].File)
		if err != nil {
			return nil, fmt.Errorf("expand signers[%d].file: %w", i, err)
		}
	}
	c.DB.Type, err = resolveEnvRef(c.DB.Type)
	if err != nil {
		return nil, fmt.Errorf("expand db.type: %w", err)
	}
	c.DB.URL, err = resolveEnvRef(c.DB.URL)
	if err != nil {
		return nil, fmt.Errorf("expand db.url: %w", err)
	}
	return c, nil
}

func RequireStore(c *wire.ConfigYAML) error {
	if err := wire.ValidateDB(c.DB); err != nil {
		return fmt.Errorf("db: %w", err)
	}
	return nil
}
