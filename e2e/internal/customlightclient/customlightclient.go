// SPDX-License-Identifier: Apache-2.0

// Package customlightclient is an external light-client implementation used
// to validate custom-compiled IBC binaries.
package customlightclient

import (
	"context"
	"errors"
	"os"
	"time"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"

	"github.com/cosmos/ibc/link/lightclient"
)

const Type = "e2e-custom-client"

type Factory struct{}

type params struct {
	MarkerFile string `yaml:"markerFile"`
}

func (Factory) Type() string { return Type }

func (Factory) New(
	_ context.Context,
	options lightclient.ProverFactoryOptions,
) (lightclient.Prover, error) {
	self := options.Client
	var p params
	if err := self.ClientParams.Decode(&p); err != nil {
		return nil, err
	}
	if p.MarkerFile == "" {
		return nil, errors.New("markerFile is required")
	}
	if err := os.WriteFile(
		p.MarkerFile, []byte(options.HostChain.ChainID+"/"+self.ClientID), 0o600,
	); err != nil {
		return nil, err
	}

	return Generator{}, nil
}

type Generator struct{}

func (Generator) LatestProvableHeight(context.Context) (uint64, time.Time, error) {
	return 0, time.Time{}, errors.New("e2e custom generator does not generate proofs")
}

func (Generator) StateProof(context.Context, uint64) ([]byte, error) {
	return nil, errors.New("e2e custom generator does not generate proofs")
}

func (Generator) PacketProofs(
	context.Context,
	uint64,
	lightclient.ProofKind,
	[]channeltypesv2.Packet,
) ([][]byte, error) {
	return nil, errors.New("e2e custom generator does not generate proofs")
}
