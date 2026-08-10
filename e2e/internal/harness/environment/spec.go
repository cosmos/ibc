// SPDX-License-Identifier: Apache-2.0

// Package environment describes and realizes IBC test environments.
package environment

import (
	"fmt"
	"slices"
	"time"
)

// Graph identities are distinct types so references to different resource
// families cannot be accidentally interchanged.
type (
	ChainID       string
	IBCInstanceID string
	ConnectionID  string
	ClientID      string
	AttestorID    string

	// EndpointBindingID and AuthorityID name values supplied separately at runtime.
	EndpointBindingID string
	AuthorityID       string

	// Existing resource locators are authored identifiers whose concrete
	// interpretation belongs to realization, not to the desired graph.
	IBCInstanceLocator string
	IBCClientLocator   string
)

// Spec is the desired IBC resource graph. Typed references determine dependency
// order; protocol declarations retain authored order while Chains start concurrently.
type Spec struct {
	Chains       []ChainSpec
	IBCInstances []IBCInstanceSpec
	Connections  []ConnectionSpec
	Attestors    []AttestorSpec
}

func (s Spec) snapshot() Spec {
	out := Spec{
		Chains:       slices.Clone(s.Chains),
		IBCInstances: slices.Clone(s.IBCInstances),
		Connections:  slices.Clone(s.Connections),
		Attestors:    slices.Clone(s.Attestors),
	}
	return out
}

// ChainSpec is sealed because acquisition and ownership semantics differ by
// concrete Chain declaration.
type ChainSpec interface {
	chainSpec()
	chainID() ChainID
	validateChain() error
}

// ManagedAnvil declares an environment-owned Anvil Chain.
type ManagedAnvil struct {
	ID         ChainID
	EVMChainID uint64
}

func (ManagedAnvil) chainSpec() {}
func (c ManagedAnvil) chainID() ChainID {
	return c.ID
}

func (c ManagedAnvil) validateChain() error {
	if err := requireValue("chain id", string(c.ID)); err != nil {
		return err
	}
	if c.EVMChainID == 0 {
		return errorsf("chain %q: EVM chain id must be greater than zero", c.ID)
	}
	return nil
}

// ManagedBesu declares an environment-owned Besu Chain.
type ManagedBesu struct {
	ID         ChainID
	EVMChainID uint64
}

func (ManagedBesu) chainSpec() {}
func (c ManagedBesu) chainID() ChainID {
	return c.ID
}

func (c ManagedBesu) validateChain() error {
	if err := requireValue("chain id", string(c.ID)); err != nil {
		return err
	}
	if c.EVMChainID == 0 {
		return errorsf("chain %q: EVM chain id must be greater than zero", c.ID)
	}
	return nil
}

// AttachedEVM declares a borrowed EVM Chain. Endpoint names a runtime binding
// rather than embedding an RPC URL in the durable Spec. Timing is mandatory
// because an attached Chain cannot safely inherit a local launcher default.
type AttachedEVM struct {
	ID         ChainID
	EVMChainID uint64
	Endpoint   EndpointBindingID
	Timing     Timing
}

func (AttachedEVM) chainSpec() {}
func (c AttachedEVM) chainID() ChainID {
	return c.ID
}

func (c AttachedEVM) validateChain() error {
	if err := requireValue("chain id", string(c.ID)); err != nil {
		return err
	}
	if c.EVMChainID == 0 {
		return errorsf("chain %q: EVM chain id must be greater than zero", c.ID)
	}
	if err := requireValue(fmt.Sprintf("chain %q endpoint binding", c.ID), string(c.Endpoint)); err != nil {
		return err
	}
	return c.Timing.validate(c.ID)
}

// Timing describes endpoint wait behavior without assuming Anvil or Besu
// defaults. BlockInterval may be zero for attached chains whose cadence is
// unknown; all wait budgets must be positive.
type Timing struct {
	BlockInterval    time.Duration
	CompletionBudget time.Duration
	PollInterval     time.Duration
}

func (t Timing) validate(id ChainID) error {
	if t.BlockInterval < 0 {
		return errorsf("chain %q timing: block interval must not be negative", id)
	}
	if t.CompletionBudget <= 0 {
		return errorsf("chain %q timing: completion budget must be greater than zero", id)
	}
	if t.PollInterval <= 0 {
		return errorsf("chain %q timing: poll interval must be greater than zero", id)
	}
	return nil
}

// IBCInstanceSpec is sealed because creating protocol state and using
// explicitly identified existing state have different semantics.
type IBCInstanceSpec interface {
	ibcInstanceSpec()
	ibcInstanceID() IBCInstanceID
	ibcInstanceChain() ChainID
	validateIBCInstance() error
}

// NewIBCInstance declares a new IBC installation on Chain, including the
// ICS20 Transfer and ICS27 GMP application stack. Its lifetime is bounded by
// a managed host or durable on an attached host. Authority names the
// runtime-provided signer permitted to create it.
type NewIBCInstance struct {
	ID        IBCInstanceID
	Chain     ChainID
	Authority AuthorityID
}

func (NewIBCInstance) ibcInstanceSpec() {}
func (i NewIBCInstance) ibcInstanceID() IBCInstanceID {
	return i.ID
}

func (i NewIBCInstance) ibcInstanceChain() ChainID {
	return i.Chain
}

func (i NewIBCInstance) validateIBCInstance() error {
	if err := requireValue("IBC Instance id", string(i.ID)); err != nil {
		return err
	}
	if err := requireValue(fmt.Sprintf("IBC Instance %q chain", i.ID), string(i.Chain)); err != nil {
		return err
	}
	return requireValue(fmt.Sprintf("IBC Instance %q authority", i.ID), string(i.Authority))
}

// ExistingIBCInstance declares explicitly identified, borrowed protocol state.
type ExistingIBCInstance struct {
	ID      IBCInstanceID
	Chain   ChainID
	Locator IBCInstanceLocator
}

func (ExistingIBCInstance) ibcInstanceSpec() {}
func (i ExistingIBCInstance) ibcInstanceID() IBCInstanceID {
	return i.ID
}

func (i ExistingIBCInstance) ibcInstanceChain() ChainID {
	return i.Chain
}

func (i ExistingIBCInstance) validateIBCInstance() error {
	if err := requireValue("IBC Instance id", string(i.ID)); err != nil {
		return err
	}
	if err := requireValue(fmt.Sprintf("IBC Instance %q chain", i.ID), string(i.Chain)); err != nil {
		return err
	}
	return requireValue(fmt.Sprintf("IBC Instance %q locator", i.ID), string(i.Locator))
}

// clientIdentityValue is the common internal identity carried by both Client
// declaration variants.
type clientIdentityValue struct {
	ID          ClientID
	IBCInstance IBCInstanceID
}

// ClientSpec is sealed because creating an IBC Client and using explicitly
// identified existing state have different durability and mutation semantics.
// A Connection may combine either variant at each end.
type ClientSpec interface {
	clientSpec()
}

// NewClient declares an attestation IBC Client to create on its host IBC
// Instance. When IBCInstance refers to a NewIBCInstance, Authority must resolve
// to the same address as that Instance's Authority. The quorum is explicit
// because no single default is safe for every attestor set.
type NewClient struct {
	ID                    ClientID
	IBCInstance           IBCInstanceID
	Authority             AuthorityID
	MinRequiredSignatures uint8
}

func (NewClient) clientSpec() {}

// ExistingClient identifies an already-created IBC Client.
type ExistingClient struct {
	ID          ClientID
	IBCInstance IBCInstanceID
	Locator     IBCClientLocator
}

func (ExistingClient) clientSpec() {}

// ConnectionSpec is a reciprocal pair of IBC Clients. Each end independently
// declares whether the authored Client identity will be created or resolved
// from an existing protocol locator.
type ConnectionSpec struct {
	ID ConnectionID
	A  ClientSpec
	B  ClientSpec
}

func (c ConnectionSpec) validate() error {
	if err := requireValue("IBC Connection id", string(c.ID)); err != nil {
		return err
	}
	a, err := validateClientSpec(c.ID, "A", c.A)
	if err != nil {
		return err
	}
	b, err := validateClientSpec(c.ID, "B", c.B)
	if err != nil {
		return err
	}
	if a.ID == b.ID {
		return errorsf("IBC Connection %q: clients must have distinct ids", c.ID)
	}
	if a.IBCInstance == b.IBCInstance {
		return errorsf("IBC Connection %q: clients must belong to distinct IBC Instances", c.ID)
	}
	return nil
}

func validateClientSpec(connectionID ConnectionID, end string, spec ClientSpec) (clientIdentityValue, error) {
	var (
		client       clientIdentityValue
		variantField string
		variantValue string
	)
	switch declaration := spec.(type) {
	case NewClient:
		client = clientIdentityValue{ID: declaration.ID, IBCInstance: declaration.IBCInstance}
		variantField = "authority"
		variantValue = string(declaration.Authority)
		if declaration.MinRequiredSignatures == 0 {
			return clientIdentityValue{}, errorsf(
				"IBC Connection %q client %q minimum required signatures must be greater than zero",
				connectionID,
				client.ID,
			)
		}
	case ExistingClient:
		client = clientIdentityValue{ID: declaration.ID, IBCInstance: declaration.IBCInstance}
		variantField = "locator"
		variantValue = string(declaration.Locator)
	default:
		return clientIdentityValue{}, errorsf(
			"IBC Connection %q end %s: unsupported declaration %T; use a concrete value",
			connectionID,
			end,
			spec,
		)
	}

	if err := requireValue(fmt.Sprintf("IBC Connection %q client id", connectionID), string(client.ID)); err != nil {
		return clientIdentityValue{}, err
	}
	if err := requireValue(
		fmt.Sprintf("IBC Connection %q client %q IBC Instance", connectionID, client.ID),
		string(client.IBCInstance),
	); err != nil {
		return clientIdentityValue{}, err
	}
	if err := requireValue(
		fmt.Sprintf("IBC Connection %q client %q %s", connectionID, client.ID, variantField),
		variantValue,
	); err != nil {
		return clientIdentityValue{}, err
	}
	return client, nil
}

func clientIdentity(spec ClientSpec) clientIdentityValue {
	switch declaration := spec.(type) {
	case NewClient:
		return clientIdentityValue{ID: declaration.ID, IBCInstance: declaration.IBCInstance}
	case ExistingClient:
		return clientIdentityValue{ID: declaration.ID, IBCInstance: declaration.IBCInstance}
	default:
		panic(fmt.Sprintf("environment: unsupported validated IBC Client declaration %T", spec))
	}
}

// AttestorSpec declares an Attestor scoped to one authored IBC Client. It signs
// that Client's attestations using a stable runtime authority binding. Its
// observed counterparty IBC Instance is derived from the Client's Connection.
type AttestorSpec struct {
	ID        AttestorID
	Client    ClientID
	Authority AuthorityID
}

// validate checks the authored graph without acquiring resources or resolving
// runtime endpoint and authority bindings.
func (s Spec) validate() error {
	chains := make(map[ChainID]struct{}, len(s.Chains))
	attachedChains := make(map[ChainID]struct{}, len(s.Chains))
	evmChainIDs := make(map[uint64]ChainID, len(s.Chains))
	for n, chain := range s.Chains {
		switch chain.(type) {
		case ManagedAnvil, ManagedBesu, AttachedEVM:
		default:
			return errorsf("chains[%d]: unsupported declaration %T; use a concrete value", n, chain)
		}
		if err := chain.validateChain(); err != nil {
			return err
		}
		id := chain.chainID()
		if _, exists := chains[id]; exists {
			return errorsf("duplicate Chain id %q", id)
		}
		evmID := chainEVMID(chain)
		if previous, exists := evmChainIDs[evmID]; exists {
			return errorsf("Chains %q and %q use duplicate EVM chain id %d", previous, id, evmID)
		}
		chains[id] = struct{}{}
		if _, attached := chain.(AttachedEVM); attached {
			attachedChains[id] = struct{}{}
		}
		evmChainIDs[evmID] = id
	}

	instances := make(map[IBCInstanceID]struct{}, len(s.IBCInstances))
	newInstances := make(map[IBCInstanceID]struct{}, len(s.IBCInstances))
	for n, instance := range s.IBCInstances {
		switch instance.(type) {
		case NewIBCInstance, ExistingIBCInstance:
		default:
			return errorsf("IBCInstances[%d]: unsupported declaration %T; use a concrete value", n, instance)
		}
		if err := instance.validateIBCInstance(); err != nil {
			return err
		}
		id := instance.ibcInstanceID()
		if _, exists := instances[id]; exists {
			return errorsf("duplicate IBC Instance id %q", id)
		}
		if chain := instance.ibcInstanceChain(); !contains(chains, chain) {
			return errorsf("IBC Instance %q references unknown Chain %q", id, chain)
		}
		if existing, ok := instance.(ExistingIBCInstance); ok && !contains(attachedChains, existing.Chain) {
			return errorsf(
				"existing IBC Instance %q must belong to an attached Chain, but %q is managed",
				existing.ID,
				existing.Chain,
			)
		}
		instances[id] = struct{}{}
		if _, isNew := instance.(NewIBCInstance); isNew {
			newInstances[id] = struct{}{}
		}
	}

	connections := make(map[ConnectionID]struct{}, len(s.Connections))
	clients := make(map[ClientID]ConnectionID, 2*len(s.Connections))
	for _, connection := range s.Connections {
		if err := connection.validate(); err != nil {
			return err
		}
		if _, exists := connections[connection.ID]; exists {
			return errorsf("duplicate IBC Connection id %q", connection.ID)
		}
		for _, declaration := range []ClientSpec{connection.A, connection.B} {
			client := clientIdentity(declaration)
			if !contains(instances, client.IBCInstance) {
				return errorsf(
					"IBC Connection %q client %q references unknown IBC Instance %q",
					connection.ID,
					client.ID,
					client.IBCInstance,
				)
			}
			if _, existing := declaration.(ExistingClient); existing && contains(newInstances, client.IBCInstance) {
				return errorsf(
					"IBC Connection %q existing client %q cannot belong to new IBC Instance %q",
					connection.ID,
					client.ID,
					client.IBCInstance,
				)
			}
			if owner, exists := clients[client.ID]; exists {
				return errorsf("duplicate IBC Client id %q in Connections %q and %q", client.ID, owner, connection.ID)
			}
			clients[client.ID] = connection.ID
		}
		connections[connection.ID] = struct{}{}
	}

	attestors := make(map[AttestorID]struct{}, len(s.Attestors))
	attestorsByClient := make(map[ClientID]int, len(s.Attestors))
	for _, attestor := range s.Attestors {
		if err := requireValue("Attestor id", string(attestor.ID)); err != nil {
			return err
		}
		if _, exists := attestors[attestor.ID]; exists {
			return errorsf("duplicate Attestor id %q", attestor.ID)
		}
		if err := requireValue(fmt.Sprintf("Attestor %q client", attestor.ID), string(attestor.Client)); err != nil {
			return err
		}
		if _, exists := clients[attestor.Client]; !exists {
			return errorsf("Attestor %q references unknown IBC Client %q", attestor.ID, attestor.Client)
		}
		if err := requireValue(
			fmt.Sprintf("Attestor %q authority", attestor.ID),
			string(attestor.Authority),
		); err != nil {
			return err
		}
		attestors[attestor.ID] = struct{}{}
		attestorsByClient[attestor.Client]++
	}
	for _, connection := range s.Connections {
		for _, declaration := range []ClientSpec{connection.A, connection.B} {
			client, isNew := declaration.(NewClient)
			if isNew && attestorsByClient[client.ID] == 0 {
				return errorsf("IBC Connection %q client %q must have at least one Attestor", connection.ID, client.ID)
			}
			if isNew && int(client.MinRequiredSignatures) > attestorsByClient[client.ID] {
				return errorsf(
					"IBC Connection %q client %q requires %d signatures from %d Attestors",
					connection.ID,
					client.ID,
					client.MinRequiredSignatures,
					attestorsByClient[client.ID],
				)
			}
		}
	}

	return nil
}

func chainEVMID(spec ChainSpec) uint64 {
	switch chain := spec.(type) {
	case ManagedAnvil:
		return chain.EVMChainID
	case ManagedBesu:
		return chain.EVMChainID
	case AttachedEVM:
		return chain.EVMChainID
	default:
		panic(fmt.Sprintf("environment: unsupported validated Chain declaration %T", spec))
	}
}

func contains[ID comparable](set map[ID]struct{}, id ID) bool {
	_, ok := set[id]
	return ok
}

func requireValue(field, value string) error {
	if value == "" {
		return errorsf("%s is required", field)
	}
	return nil
}

func errorsf(format string, args ...any) error {
	return fmt.Errorf("environment Spec: "+format, args...)
}
