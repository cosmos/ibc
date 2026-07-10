// Package fixtures provides the compiled artifacts of the TEST-ONLY Solidity fixtures and their atomic
// deployer as go:embed-ed bytes, so tests can deploy them without invoking forge at runtime
// (decision D6). The .sol sources live in src/ and are rebuilt with `make fixtures`; the resulting
// forge artifacts under out/ are committed and embedded here.
//
// These are trivial mocks, NOT the real Eureka contract stack — see the per-file headers in src/.
package fixtures

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"

	_ "embed"
)

// Raw forge artifacts. The paths are the forge default layout out/<Name>.sol/<Name>.json; embedding
// the specific files (not the whole out/ tree) keeps build-info and other contracts out of the binary.
var (
	//go:embed out/Counter.sol/Counter.json
	counterArtifact []byte
	//go:embed out/FixtureDeployer.sol/FixtureDeployer.json
	fixtureDeployerArtifact []byte
	//go:embed out/MockGMP.sol/MockGMP.json
	mockGMPArtifact []byte
	//go:embed out/MockIFT.sol/MockIFT.json
	mockIFTArtifact []byte
)

// Contract is a deployable fixture: its ABI (as the JSON string forge emitted) and its creation
// bytecode. M3/M4 deploy these via the EVM client; ABI drives event decoding and call encoding.
type Contract struct {
	Name     string // contract name; matches the src/<Name>.sol and out/<Name>.sol/<Name>.json basename
	ABI      string // contract ABI as a JSON string (parse with ParsedABI, or feed to abi.JSON directly)
	Bytecode []byte // creation/deploy bytecode, decoded from the artifact's bytecode.object
}

// ParsedABI parses the embedded ABI string into a go-ethereum abi.ABI for encoding calls / decoding
// events. It re-parses on each call; callers that parse hot should cache the result.
func (c Contract) ParsedABI() (abi.ABI, error) {
	parsed, err := abi.JSON(strings.NewReader(c.ABI))
	if err != nil {
		return abi.ABI{}, fmt.Errorf("parse %s ABI: %w", c.Name, err)
	}
	return parsed, nil
}

// MustABI is ParsedABI for package-level initialisation: the embedded artifact is build-time-fixed, so
// a parse failure is a packaging bug, not a runtime condition. It exists so callers that cache the ABI
// at init (the harness's apps/correlate packages) share one parse-or-panic helper instead of each
// reimplementing it.
func (c Contract) MustABI() abi.ABI {
	parsed, err := c.ParsedABI()
	if err != nil {
		panic(fmt.Sprintf("fixtures: %v", err))
	}
	return parsed
}

// The fixtures the harness deploys. Loaded once at package init from the embedded artifacts; a parse
// failure here means the committed artifact is malformed (a build-time invariant), so it panics
// rather than forcing every caller to handle an impossible error.
var (
	Counter         = mustLoad("Counter", counterArtifact)
	FixtureDeployer = mustLoad("FixtureDeployer", fixtureDeployerArtifact)
	MockGMP         = mustLoad("MockGMP", mockGMPArtifact)
	MockIFT         = mustLoad("MockIFT", mockIFTArtifact)
)

// forgeArtifact is the subset of the forge build artifact this package consumes: the ABI and the
// creation bytecode object. Everything else (deployedBytecode, metadata, …) is intentionally ignored.
type forgeArtifact struct {
	ABI      json.RawMessage `json:"abi"`
	Bytecode struct {
		Object string `json:"object"` // 0x-prefixed creation bytecode hex
	} `json:"bytecode"`
}

// load parses one forge artifact into a Contract.
func load(name string, raw []byte) (Contract, error) {
	var a forgeArtifact
	if err := json.Unmarshal(raw, &a); err != nil {
		return Contract{}, fmt.Errorf("unmarshal %s artifact: %w", name, err)
	}
	if len(a.ABI) == 0 {
		return Contract{}, fmt.Errorf("%s artifact has no abi", name)
	}
	// common.FromHex tolerates the 0x prefix and returns empty for an empty object; the caller/test
	// guards against empty bytecode.
	code := common.FromHex(a.Bytecode.Object)
	if len(code) == 0 {
		return Contract{}, fmt.Errorf("%s artifact has empty bytecode", name)
	}
	return Contract{Name: name, ABI: string(a.ABI), Bytecode: code}, nil
}

// mustLoad is load for package-level initialisation from embedded (i.e. build-time-fixed) data.
func mustLoad(name string, raw []byte) Contract {
	c, err := load(name, raw)
	if err != nil {
		panic(fmt.Sprintf("fixtures: %v", err))
	}
	return c
}
