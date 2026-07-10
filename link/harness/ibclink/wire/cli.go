// Package wire is the single declaration of ibc's public wire contract (CLI JSON
// output, config YAML, status API shapes), owned by the harness.
//
// The temporary stub SUT (e2e/stub) imports this package; the real binary does not — this
// package remains the harness's independent statement of the contract. As real commands take
// over entries in the SUT routing table (ibclink.commandRoutes, the swap ledger), the shapes
// here are what they must keep emitting; when the stub is fully retired this package still
// pins the contract the harness asserts against.
package wire

// Process exit codes, following BSD sysexits(3) so the harness can map an exit status to a precise
// failure class without parsing stderr. These are part of the executable contract: tests assert on
// them directly (e.g. invalid config MUST exit 64).
const (
	ExitOK             = 0  // success
	ExitConfigInvalid  = 64 // EX_USAGE:       config failed validation
	ExitRPCUnreachable = 65 // EX_DATAERR:     a --live RPC / live check was unreachable
	ExitDeployFailure  = 66 // EX_NOINPUT:     fixture deployment failed
	ExitNotReady       = 69 // EX_UNAVAILABLE: daemon not ready / readiness timeout
	ExitInternal       = 70 // EX_SOFTWARE:    unexpected internal error
)
