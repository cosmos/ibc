// SPDX-License-Identifier: Apache-2.0

// Package environment describes and realizes IBC test environments.
package environment

import (
	"fmt"
	"slices"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// Graph identities are distinct types so references to different resource
// families cannot be accidentally interchanged.
type (
	ChainID       string
	IBCInstanceID string
	ConnectionID  string
	AttestorID    string

	// EndpointBindingID and AuthorityID name values supplied separately at runtime.
	EndpointBindingID string
	AuthorityID       string

	// Existing resource locators are authored identifiers whose concrete
	// interpretation belongs to realization, not to the desired graph.
	IBCInstanceLocator string
)

// Spec is the desired IBC resource graph. Typed references determine dependency
// order; protocol declarations retain authored order while Chains start concurrently.
type Spec struct {
	Chains       []ChainSpec
	IBCInstances []IBCInstanceSpec
	Connections  []ConnectionSpec
}

func (s Spec) snapshot() Spec {
	out := Spec{
		Chains:       slices.Clone(s.Chains),
		IBCInstances: slices.Clone(s.IBCInstances),
		Connections:  make([]ConnectionSpec, len(s.Connections)),
	}
	for i, connection := range s.Connections {
		connection.A = snapshotClient(connection.A)
		connection.B = snapshotClient(connection.B)
		out.Connections[i] = connection
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

// ClientSpec is sealed because creating an IBC Client and using explicitly
// identified existing state have different durability and mutation semantics.
// A Connection may combine either variant at each end.
type ClientSpec interface {
	clientSpec()
	clientAttestors() []AttestorSpec
}

// NewClient declares an attestation IBC Client to create on its host IBC
// Instance. When IBCInstance refers to a NewIBCInstance, Authority must resolve
// to the same address as that Instance's Authority. The quorum is explicit
// because no single default is safe for every attestor set.
type NewClient struct {
	IBCInstance           IBCInstanceID
	Authority             AuthorityID
	MinRequiredSignatures uint8
	Attestors             []AttestorSpec
}

func (NewClient) clientSpec()                       {}
func (c NewClient) clientAttestors() []AttestorSpec { return c.Attestors }

// ExistingClient identifies an already-created IBC Client by its protocol ID.
type ExistingClient struct {
	IBCInstance IBCInstanceID
	ID          string
	Attestors   []AttestorSpec
}

func (ExistingClient) clientSpec()                       {}
func (c ExistingClient) clientAttestors() []AttestorSpec { return c.Attestors }

func snapshotClient(spec ClientSpec) ClientSpec {
	switch client := spec.(type) {
	case NewClient:
		client.Attestors = slices.Clone(client.Attestors)
		return client
	case ExistingClient:
		client.Attestors = slices.Clone(client.Attestors)
		return client
	default:
		return spec
	}
}

// ConnectionSpec is a reciprocal pair of IBC Clients. Each end independently
// declares whether its Client will be created or resolved from an existing
// protocol ID.
type ConnectionSpec struct {
	ID ConnectionID
	A  ClientSpec
	B  ClientSpec
}

type clientEnd struct {
	label       string
	declaration ClientSpec
}

func (c ConnectionSpec) ends() [2]clientEnd {
	return [2]clientEnd{
		{label: "A", declaration: c.A},
		{label: "B", declaration: c.B},
	}
}

func clientLabel(connection ConnectionID, end string) string {
	return fmt.Sprintf("%s/%s", connection, end)
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
	if a == b {
		return errorsf("IBC Connection %q: clients must belong to distinct IBC Instances", c.ID)
	}
	return nil
}

func validateClientSpec(connectionID ConnectionID, end string, spec ClientSpec) (IBCInstanceID, error) {
	var (
		instance     IBCInstanceID
		variantField string
		variantValue string
	)
	switch declaration := spec.(type) {
	case NewClient:
		instance = declaration.IBCInstance
		variantField = "authority"
		variantValue = string(declaration.Authority)
		if declaration.MinRequiredSignatures == 0 {
			return "", errorsf(
				"IBC Client %q minimum required signatures must be greater than zero",
				clientLabel(connectionID, end),
			)
		}
	case ExistingClient:
		instance = declaration.IBCInstance
		variantField = "id"
		variantValue = declaration.ID
	default:
		return "", errorsf(
			"IBC Connection %q end %s: unsupported declaration %T; use a concrete value",
			connectionID,
			end,
			spec,
		)
	}

	if err := requireValue(
		fmt.Sprintf("IBC Client %q IBC Instance", clientLabel(connectionID, end)),
		string(instance),
	); err != nil {
		return "", err
	}
	if err := requireValue(
		fmt.Sprintf("IBC Client %q %s", clientLabel(connectionID, end), variantField),
		variantValue,
	); err != nil {
		return "", err
	}
	return instance, nil
}

func clientIBCInstance(spec ClientSpec) IBCInstanceID {
	switch declaration := spec.(type) {
	case NewClient:
		return declaration.IBCInstance
	case ExistingClient:
		return declaration.IBCInstance
	default:
		panic(fmt.Sprintf("environment: unsupported validated IBC Client declaration %T", spec))
	}
}

// AttestorSpec declares an Attestor for the Client that contains it.
type AttestorSpec struct {
	ID        AttestorID
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
	existingInstanceLocators := make(map[struct {
		chain   ChainID
		locator IBCInstanceLocator
	}]IBCInstanceID)
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
		if existing, ok := instance.(ExistingIBCInstance); ok {
			locator := existing.Locator
			if common.IsHexAddress(string(locator)) {
				locator = IBCInstanceLocator(common.HexToAddress(string(locator)).Hex())
			}
			key := struct {
				chain   ChainID
				locator IBCInstanceLocator
			}{chain: existing.Chain, locator: locator}
			if previous, exists := existingInstanceLocators[key]; exists {
				return errorsf(
					"existing IBC Instances %q and %q on Chain %q reference duplicate locator %q",
					previous, existing.ID, existing.Chain, existing.Locator,
				)
			}
			existingInstanceLocators[key] = existing.ID
		}
		instances[id] = struct{}{}
		if _, isNew := instance.(NewIBCInstance); isNew {
			newInstances[id] = struct{}{}
		}
	}

	connections := make(map[ConnectionID]struct{}, len(s.Connections))
	clientIDsByInstance := make(map[IBCInstanceID]map[string]string)
	attestors := make(map[AttestorID]struct{})
	for _, connection := range s.Connections {
		if err := connection.validate(); err != nil {
			return err
		}
		if _, exists := connections[connection.ID]; exists {
			return errorsf("duplicate IBC Connection id %q", connection.ID)
		}
		for _, end := range connection.ends() {
			label := clientLabel(connection.ID, end.label)
			instance := clientIBCInstance(end.declaration)
			if !contains(instances, instance) {
				return errorsf(
					"IBC Client %q references unknown IBC Instance %q",
					label,
					instance,
				)
			}
			if _, existing := end.declaration.(ExistingClient); existing && contains(newInstances, instance) {
				return errorsf(
					"existing IBC Client %q cannot belong to new IBC Instance %q",
					label,
					instance,
				)
			}
			id := clientID(connection.ID, end.label, end.declaration)
			ids := clientIDsByInstance[instance]
			if ids == nil {
				ids = make(map[string]string)
				clientIDsByInstance[instance] = ids
			}
			if previous, exists := ids[id]; exists {
				return errorsf(
					"IBC Clients %q and %q on IBC Instance %q resolve to duplicate id %q",
					previous, label, instance, id,
				)
			}
			ids[id] = label

			clientAttestors := end.declaration.clientAttestors()
			for _, attestor := range clientAttestors {
				if err := requireValue("Attestor id", string(attestor.ID)); err != nil {
					return err
				}
				if _, exists := attestors[attestor.ID]; exists {
					return errorsf("duplicate Attestor id %q", attestor.ID)
				}
				if err := requireValue(
					fmt.Sprintf("Attestor %q authority", attestor.ID),
					string(attestor.Authority),
				); err != nil {
					return err
				}
				attestors[attestor.ID] = struct{}{}
			}
			if newClient, isNew := end.declaration.(NewClient); isNew && len(clientAttestors) == 0 {
				return errorsf("IBC Client %q must have at least one Attestor", label)
			} else if isNew && int(newClient.MinRequiredSignatures) > len(clientAttestors) {
				return errorsf(
					"IBC Client %q requires %d signatures from %d Attestors",
					label, newClient.MinRequiredSignatures, len(clientAttestors),
				)
			}
		}
		connections[connection.ID] = struct{}{}
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
