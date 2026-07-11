// Package wire defines the CLI, config, and status API contracts asserted by the harness.
package wire

// Exit codes follow BSD sysexits(3); tests assert on them directly.
const (
	ExitOK             = 0
	ExitConfigInvalid  = 64
	ExitRPCUnreachable = 65
	ExitDeployFailure  = 66
	ExitNotReady       = 69
	ExitInternal       = 70
)
