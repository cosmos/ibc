package config

import "github.com/spf13/cobra"

// FlagSet composed set of cli args parsed into a nice struct.
type FlagSet struct {
	// Home IBC home directory where files are stored
	Home string
}

// DefaultFlagSet returns the default flag set.
func DefaultFlagSet() FlagSet {
	return FlagSet{
		Home: "~/.ibc",
	}
}

// DeclarePersistentFlags declares the persistent flags for the command.
func DeclarePersistentFlags(cmd *cobra.Command, flags *FlagSet) {
	pf := cmd.PersistentFlags()

	pf.StringVarP(&flags.Home, "home", "", flags.Home, "IBC home directory")
}
