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
	Home    string
	Config  string
	DB      string
	Quiet   bool
	LogJSON bool

	skipConfigValidation bool
}

// DefaultFlagSet returns the default flag set.
func DefaultFlagSet() FlagSet {
	return FlagSet{
		Home:   "~/.ibc",
		Config: "ibc.yml",
		DB:     "",
		Quiet:  false,

		skipConfigValidation: false,
	}
}

// DeclarePersistentFlags declares the persistent flags for the command.
func DeclarePersistentFlags(cmd *cobra.Command, flags *FlagSet) {
	pf := cmd.PersistentFlags()

	pf.StringVarP(&flags.Home, "home", "", flags.Home, "IBC home directory")
	pf.StringVarP(&flags.Config, "config", "", flags.Config, "Config file relative to home")
	pf.StringVarP(&flags.DB, "db", "", flags.DB, "Database URL override")
	pf.BoolVarP(&flags.Quiet, "quiet", "q", flags.Quiet, "Quiet mode")
	pf.BoolVarP(&flags.LogJSON, "log-json", "", flags.LogJSON, "Enable JSON logging")
}

func (fs *FlagSet) ConfigPath() (string, error) {
	home, err := ExpandHome(fs.Home)
	if err != nil {
		return "", err
	}

	return filepath.Abs(filepath.Join(home, fs.Config))
}

func (fs *FlagSet) ValidateConfig() bool {
	return !fs.skipConfigValidation
}

func (fs *FlagSet) SkipConfigValidation() {
	fs.skipConfigValidation = true
}

// ExpandHome converts path with ~ to an absolute path.
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

// Given a path to a file or a directory, ensure the directory exists.
func EnsureDirectory(directoryOrFile string) error {
	dir := filepath.Dir(directoryOrFile)
	if dir == "." {
		return nil
	}

	return os.MkdirAll(dir, 0o755)
}
