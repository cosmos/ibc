package solidityibc

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"

	_ "embed"
)

//go:generate bun install --cwd contracts --frozen-lockfile
//go:generate forge build --force --root contracts

//go:embed contracts/out/AccessManager.sol/AccessManager.json
var accessManagerArtifactJSON []byte

type forgeArtifact struct {
	ABI      json.RawMessage `json:"abi"`
	Bytecode struct {
		Object string `json:"object"`
	} `json:"bytecode"`
}

func loadAccessManagerArtifact() (abi.ABI, []byte, error) {
	var artifact forgeArtifact
	if err := json.Unmarshal(accessManagerArtifactJSON, &artifact); err != nil {
		return abi.ABI{}, nil, fmt.Errorf("decode AccessManager artifact: %w", err)
	}
	parsed, err := abi.JSON(strings.NewReader(string(artifact.ABI)))
	if err != nil {
		return abi.ABI{}, nil, fmt.Errorf("parse AccessManager ABI: %w", err)
	}
	bytecode := common.FromHex(artifact.Bytecode.Object)
	if len(bytecode) == 0 {
		return abi.ABI{}, nil, fmt.Errorf("AccessManager artifact has empty creation bytecode")
	}
	return parsed, bytecode, nil
}
