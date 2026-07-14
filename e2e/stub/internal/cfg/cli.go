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

const (
	dbTypeSQLite   = "sqlite"
	dbTypePostgres = "postgres"
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
	pf.StringVar(&flags.DB, "db", flags.DB, "Database URL override")
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

func EnsureDirectory(path string) error {
	dir := filepath.Dir(path)
	if dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

func dbConfigFromURL(raw string) (wire.DB, error) {
	switch raw {
	case "":
		return wire.DB{}, fmt.Errorf(".url must not be empty")
	case ":memory:":
		return wire.DB{}, fmt.Errorf(".url must not be :memory: for sqlite")
	}
	dbType := dbTypeSQLite
	if strings.HasPrefix(raw, "postgres://") || strings.HasPrefix(raw, "postgresql://") {
		dbType = dbTypePostgres
	}
	return wire.DB{Type: dbType, URL: raw}, nil
}

func PrintJSON(value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Println(string(data))
	return err
}
