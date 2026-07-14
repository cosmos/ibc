// Package stub implements the temporary Link behavior selected at the CLI composition root.
package stub

import (
	"fmt"
	"os"

	"github.com/cosmos/ibc/link/cmd/configcmd"

	internalconfig "github.com/cosmos/ibc/link/internal/config"
)

func setupConfig(flags *internalconfig.FlagSet) (*configcmd.Config, error) {
	home, err := internalconfig.ExpandHome(flags.Home)
	if err != nil {
		return nil, fmt.Errorf("home: %w", err)
	}

	path, err := flags.ConfigPath()
	if err != nil {
		return nil, fmt.Errorf("unable to get config path: %w", err)
	}

	if chdirErr := os.Chdir(home); chdirErr != nil {
		return nil, fmt.Errorf("unable to change working directory to %s: %w", home, chdirErr)
	}

	c, err := loadConfig(path)
	if err != nil {
		return nil, err
	}
	if flags.DB != "" {
		db, dbErr := dbConfigFromPath(flags.DB)
		if dbErr != nil {
			return nil, fmt.Errorf("invalid --db: %w", dbErr)
		}
		c.DB = db
	}

	return c, nil
}

func loadConfig(path string) (*configcmd.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}
	c, err := configcmd.Unmarshal(data)
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

func requireStore(c *configcmd.Config) error {
	if err := configcmd.ValidateDB(c.DB); err != nil {
		return fmt.Errorf("db: %w", err)
	}
	return nil
}

func dbConfigFromPath(path string) (configcmd.DB, error) {
	db := configcmd.DB{Type: configcmd.DBTypeSQLite, URL: path}
	if err := configcmd.ValidateDB(db); err != nil {
		return configcmd.DB{}, err
	}
	return db, nil
}
