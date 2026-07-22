package e2etest

import (
	"errors"
	"fmt"

	"github.com/cosmos/ibc/e2e/internal/harness/environment"
)

type RouteID string

// Packet identifies the protocol packet originated by a test application.
type Packet struct {
	RouteID      RouteID
	Source       environment.ChainID
	SourceTxHash string
	Sequence     uint64
}

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
		return endpoint{}, endpoint{}, errors.New("e2etest: route id is required")
	}
	if source == nil {
		return endpoint{}, endpoint{}, errors.New("e2etest: source Chain is required")
	}
	if destination == nil {
		return endpoint{}, endpoint{}, errors.New("e2etest: destination Chain is required")
	}
	if source.ID() == destination.ID() {
		return endpoint{}, endpoint{}, fmt.Errorf(
			"e2etest: route %q must connect different Chains",
			routeID,
		)
	}
	sourceEVM, err := source.EVM()
	if err != nil {
		return endpoint{}, endpoint{}, fmt.Errorf("e2etest: source Chain %q: %w", source.ID(), err)
	}
	destinationEVM, err := destination.EVM()
	if err != nil {
		return endpoint{}, endpoint{}, fmt.Errorf(
			"e2etest: destination Chain %q: %w",
			destination.ID(),
			err,
		)
	}
	return endpoint{chain: source, evm: sourceEVM}, endpoint{chain: destination, evm: destinationEVM}, nil
}
