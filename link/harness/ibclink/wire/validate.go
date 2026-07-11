package wire

// ValidationError is one structural problem found in a config, located by a JSON-pathish string the
// harness (and a human) can use to find the offending field, e.g. "relayer.routes[0].source".
type ValidationError struct {
	Path string `json:"path"`
	Msg  string `json:"msg"`
}

// ValidateResult is the machine-readable output of `ibc config validate`. The stub marshals it and
// the harness Runner parses the same type, so the validate wire format stays single-sourced.
//
// Valid reflects STRUCTURAL validity only. With --live, a structurally-valid config whose chain RPC
// is unreachable still reports Valid:true but carries the reachability problem in Errors and makes the
// process exit 65 (RPC unreachable) — distinct from the structural-failure exit 64. Callers key off
// the exit code for the failure class and read Errors for the human-readable reason.
type ValidateResult struct {
	Valid          bool              `json:"valid"`
	ResolvedChains []string          `json:"resolvedChains,omitempty"`
	Warnings       []string          `json:"warnings"` // always present (possibly empty), never null
	Errors         []ValidationError `json:"errors,omitempty"`
}
