package cfg

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cosmos/ibc/e2e/internal/harness/ibclink/wire"
)

type FlagSet struct {
	Home   string
	Config string
	DB     string
}

func DefaultFlagSet() FlagSet {
	return FlagSet{Home: "~/.ibc", Config: "ibc.yml"}
}

func DeclarePersistentFlags(cmd *cobra.Command, flags *FlagSet) {
	pf := cmd.PersistentFlags()
	pf.StringVar(&flags.Home, "home", flags.Home, "IBC home directory")
	pf.StringVar(&flags.Config, "config", flags.Config, "Config file relative to home")
	pf.StringVar(&flags.DB, "db", flags.DB, "SQLite database path override")
}

func (f *FlagSet) ConfigPath() (string, error) {
	home, err := ExpandHome(f.Home)
	if err != nil {
		return "", err
	}
	return filepath.Abs(filepath.Join(home, f.Config))
}

func ExpandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
}

func dbConfigFromPath(path string) (wire.DB, error) {
	db := wire.DB{Type: wire.DBTypeSQLite, URL: path}
	if err := wire.ValidateDB(db); err != nil {
		return wire.DB{}, err
	}
	return db, nil
}

func PrintJSON(value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Println(string(data))
	return err
}
