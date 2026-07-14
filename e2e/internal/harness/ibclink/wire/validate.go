package wire

type ValidationError struct {
	Path string `json:"path"`
	Msg  string `json:"msg"`
}

// Valid covers structure only; live RPC failures preserve it and use ExitRPCUnreachable.
type ValidateResult struct {
	Valid          bool              `json:"valid"`
	ResolvedChains []string          `json:"resolvedChains,omitempty"`
	Warnings       []string          `json:"warnings"`
	Errors         []ValidationError `json:"errors,omitempty"`
}
