package diag

import "runtime/debug"

const goEthereumModule = "github.com/ethereum/go-ethereum"

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
