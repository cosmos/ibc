// Package configcmd owns the config command's CLI and transport contract.
package configcmd

import "github.com/spf13/cobra"

// Handler implements a config subcommand.
type Handler func(*cobra.Command, []string) error

// ValidateOptions contains config-validation flags.
type ValidateOptions struct {
	Live   bool
	Strict bool
}

// ValidateHandler implements config validate.
type ValidateHandler func(*cobra.Command, []string, ValidateOptions) error

// Handlers contains config command behavior selected by the executable.
type Handlers struct {
	New      Handler
	Validate ValidateHandler
}

// NewCommand constructs the config command with its behavior injected by the executable.
func NewCommand(handlers Handlers) *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Config commands"}
	if handlers.New != nil {
		cmd.AddCommand(&cobra.Command{Use: "new", Short: "Create new config file", RunE: handlers.New})
	}
	var options ValidateOptions
	validate := &cobra.Command{
		Use:          "validate",
		Short:        "Validate the config",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handlers.Validate(cmd, args, options)
		},
	}
	validate.Flags().BoolVar(&options.Live, "live", false, "extra validation checks")
	validate.Flags().BoolVar(&options.Strict, "strict", false, "fail on unknown fields in the config file")
	cmd.AddCommand(validate)
	return cmd
}
