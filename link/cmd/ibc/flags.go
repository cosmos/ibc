// SPDX-License-Identifier: Apache-2.0

package main

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/cosmos/ibc/link/internal/fsutil"
)

type flagSet struct {
	Home    string
	Config  string
	DB      string
	Quiet   bool
	LogJSON bool

	skipConfigValidation bool
}

func defaultFlagSet() flagSet {
	return flagSet{Home: "~/.ibc", Config: "ibc.yml"}
}

func declarePersistentFlags(cmd *cobra.Command, flags *flagSet) {
	pf := cmd.PersistentFlags()

	pf.StringVarP(&flags.Home, "home", "", flags.Home, "IBC home directory")
	pf.StringVarP(&flags.Config, "config", "", flags.Config, "Config file relative to home")
	pf.StringVarP(&flags.DB, "db", "", flags.DB, "Database URL override")
	pf.BoolVarP(&flags.Quiet, "quiet", "q", flags.Quiet, "Quiet mode")
	pf.BoolVarP(&flags.LogJSON, "log-json", "", flags.LogJSON, "Enable JSON logging")
}

func (fs *flagSet) ConfigPath() (string, error) {
	home, err := fsutil.ExpandHome(fs.Home)
	if err != nil {
		return "", err
	}

	return filepath.Abs(filepath.Join(home, fs.Config))
}

func (fs *flagSet) ValidateConfig() bool {
	return !fs.skipConfigValidation
}

func (fs *flagSet) SkipConfigValidation() {
	fs.skipConfigValidation = true
}
