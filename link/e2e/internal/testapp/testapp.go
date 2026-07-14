// Package testapp binds the concrete applications used by the end-to-end tests
// to already-realized Chains. It owns application protocol details, but no
// environment or relayer lifecycle.
package testapp

import (
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"

	"github.com/cosmos/ibc/link/harness/environment"
)

type RouteID string

type IFTContracts struct {
	Source      string
	Destination string
}

type GMPContracts struct {
	Source      string
	Destination string
	Counter     string
}

type Application string

const (
	ApplicationIFT Application = "IFT"
	ApplicationGMP Application = "GMP"
)

// Packet identifies the protocol packet originated by a test application.
type Packet struct {
	RouteID      RouteID
	Source       environment.ChainID
	SourceTxHash string
	Sequence     uint64

	application Application
}

func (p Packet) Application() Application { return p.application }

func (p Packet) reference() string {
	return fmt.Sprintf("route %q sequence %d", p.RouteID, p.Sequence)
}

type endpoint struct {
	chain *environment.Chain
	evm   *environment.EVM
}

func bindRoute(
	routeID RouteID,
	source, destination *environment.Chain,
) (endpoint, endpoint, error) {
	if routeID == "" {
		return endpoint{}, endpoint{}, errors.New("testapp: route id is required")
	}
	if source == nil {
		return endpoint{}, endpoint{}, errors.New("testapp: source Chain is required")
	}
	if destination == nil {
		return endpoint{}, endpoint{}, errors.New("testapp: destination Chain is required")
	}
	if source.ID() == destination.ID() {
		return endpoint{}, endpoint{}, fmt.Errorf(
			"testapp: route %q must connect different Chains",
			routeID,
		)
	}
	sourceEVM, err := source.EVM()
	if err != nil {
		return endpoint{}, endpoint{}, fmt.Errorf("testapp: source Chain %q: %w", source.ID(), err)
	}
	destinationEVM, err := destination.EVM()
	if err != nil {
		return endpoint{}, endpoint{}, fmt.Errorf(
			"testapp: destination Chain %q: %w",
			destination.ID(),
			err,
		)
	}
	return endpoint{chain: source, evm: sourceEVM}, endpoint{chain: destination, evm: destinationEVM}, nil
}

func address(label, value string) (common.Address, error) {
	if !common.IsHexAddress(value) {
		return common.Address{}, fmt.Errorf("testapp: %s %q is not a valid EVM address", label, value)
	}
	return common.HexToAddress(value), nil
}
