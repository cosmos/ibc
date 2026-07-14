// Package contracts exposes the contracts used by the synthetic e2e test applications.
package contracts

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
	//go:embed out/TestAppDeployer.sol/TestAppDeployer.json
	testAppDeployerArtifact []byte
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

func (c Contract) MustABI() abi.ABI {
	parsed, err := c.ParsedABI()
	if err != nil {
		panic(fmt.Sprintf("test app contracts: %v", err))
	}
	return parsed
}

var (
	Counter         = mustLoad("Counter", counterArtifact)
	TestAppDeployer = mustLoad("TestAppDeployer", testAppDeployerArtifact)
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
	var artifact forgeArtifact
	if err := json.Unmarshal(raw, &artifact); err != nil {
		return Contract{}, fmt.Errorf("unmarshal %s artifact: %w", name, err)
	}
	if len(artifact.ABI) == 0 {
		return Contract{}, fmt.Errorf("%s artifact has no abi", name)
	}
	code := common.FromHex(artifact.Bytecode.Object)
	if len(code) == 0 {
		return Contract{}, fmt.Errorf("%s artifact has empty bytecode", name)
	}
	return Contract{Name: name, ABI: string(artifact.ABI), Bytecode: code}, nil
}

func mustLoad(name string, raw []byte) Contract {
	contract, err := load(name, raw)
	if err != nil {
		panic(fmt.Sprintf("test app contracts: %v", err))
	}
	return contract
}
