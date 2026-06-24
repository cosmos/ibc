package config

import "github.com/spf13/cobra"

type FlagSet struct {
	// Home IBC home directory where files are stored
	Home string
}

func DefaultFlagSet() FlagSet {
	return FlagSet{
		Home: "~/.ibc",
	}
}

func DeclarePersistentFlags(cmd *cobra.Command, flags *FlagSet) {
	pf := cmd.PersistentFlags()

	pf.StringVarP(&flags.Home, "home", "", flags.Home, "IBC home directory")
}
