package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// FlagSet composed set of cli args parsed into a nice struct.
type FlagSet struct {
	// Home IBC home directory where files are stored
	Home   string
	Config string
	Quiet  bool
}

// DefaultFlagSet returns the default flag set.
func DefaultFlagSet() FlagSet {
	return FlagSet{
		Home:   "~/.ibc",
		Config: "ibc.yml",
		Quiet:  false,
	}
}

// DeclarePersistentFlags declares the persistent flags for the command.
func DeclarePersistentFlags(cmd *cobra.Command, flags *FlagSet) {
	pf := cmd.PersistentFlags()

	pf.StringVarP(&flags.Home, "home", "", flags.Home, "IBC home directory")
	pf.StringVarP(&flags.Config, "config", "", flags.Config, "Config file relative to home")
	pf.BoolVarP(&flags.Quiet, "quiet", "q", flags.Quiet, "Quiet mode")
}

func (fs *FlagSet) ConfigPath() (string, error) {
	home, err := expandHome(fs.Home)
	if err != nil {
		return "", err
	}

	return filepath.Abs(filepath.Join(home, fs.Config))
}

// converts path with ~ to absolute path
func expandHome(path string) (string, error) {
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

func ensureDirectory(directoryOrFile string) error {
	dir := filepath.Dir(directoryOrFile)
	if dir == "." {
		return nil
	}

	return os.MkdirAll(dir, 0o755)
}
