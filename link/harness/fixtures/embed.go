// Package fixtures provides embedded, test-only Solidity contract artifacts.
package fixtures

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"

	_ "embed"
)

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

type Contract struct {
	Name     string
	ABI      string
	Bytecode []byte
}

func (c Contract) ParsedABI() (abi.ABI, error) {
	parsed, err := abi.JSON(strings.NewReader(c.ABI))
	if err != nil {
		return abi.ABI{}, fmt.Errorf("parse %s ABI: %w", c.Name, err)
	}
	return parsed, nil
}

// MustABI panics if the embedded ABI is invalid.
func (c Contract) MustABI() abi.ABI {
	parsed, err := c.ParsedABI()
	if err != nil {
		panic(fmt.Sprintf("fixtures: %v", err))
	}
	return parsed
}

var (
	Counter         = mustLoad("Counter", counterArtifact)
	FixtureDeployer = mustLoad("FixtureDeployer", fixtureDeployerArtifact)
	MockGMP         = mustLoad("MockGMP", mockGMPArtifact)
	MockIFT         = mustLoad("MockIFT", mockIFTArtifact)
)

type forgeArtifact struct {
	ABI      json.RawMessage `json:"abi"`
	Bytecode struct {
		Object string `json:"object"`
	} `json:"bytecode"`
}

func load(name string, raw []byte) (Contract, error) {
	var a forgeArtifact
	if err := json.Unmarshal(raw, &a); err != nil {
		return Contract{}, fmt.Errorf("unmarshal %s artifact: %w", name, err)
	}
	if len(a.ABI) == 0 {
		return Contract{}, fmt.Errorf("%s artifact has no abi", name)
	}
	code := common.FromHex(a.Bytecode.Object)
	if len(code) == 0 {
		return Contract{}, fmt.Errorf("%s artifact has empty bytecode", name)
	}
	return Contract{Name: name, ABI: string(a.ABI), Bytecode: code}, nil
}

func mustLoad(name string, raw []byte) Contract {
	c, err := load(name, raw)
	if err != nil {
		panic(fmt.Sprintf("fixtures: %v", err))
	}
	return c
}
