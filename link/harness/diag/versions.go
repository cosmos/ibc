package diag

import "runtime/debug"

// goEthereumModule is the EVM client dependency whose version the bundle records, so a failure report
// names which go-ethereum the harness was built against without shelling out.
const goEthereumModule = "github.com/ethereum/go-ethereum"

// GoEthereumVersion reads the go-ethereum dependency version from the binary's build info (no exec). A
// replace directive is reported as the replacement's version. Returns "unknown" when build info is
// unavailable (e.g. a binary built without module info).
func GoEthereumVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, d := range bi.Deps {
		if d.Path != goEthereumModule {
			continue
		}
		if d.Replace != nil && d.Replace.Version != "" {
			return d.Replace.Version
		}
		if d.Version != "" {
			return d.Version
		}
	}
	return "unknown"
}
