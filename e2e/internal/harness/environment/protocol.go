package environment

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"
)

// EVMAddress is a non-secret, checksummed contract or signer address returned
// by the concrete EVM realization. It remains a string at the Environment
// boundary so workflows do not need the deployment adapter's Go types.
type EVMAddress string

// IBCInstance is a ready ICS26 installation on one resolved Chain.
type IBCInstance struct {
	id            IBCInstanceID
	chain         *Chain
	locator       IBCInstanceLocator
	accessManager EVMAddress
	ics20Transfer EVMAddress
	ics27GMP      EVMAddress
}

func (i *IBCInstance) ID() IBCInstanceID           { return i.id }
func (i *IBCInstance) Chain() *Chain               { return i.chain }
func (i *IBCInstance) Locator() IBCInstanceLocator { return i.locator }
func (i *IBCInstance) AccessManagerAddress() EVMAddress {
	return i.accessManager
}

// ICS20TransferAddress returns the ICS20 Transfer proxy. It is zero for
// attached instances, which resolve only the router and access manager.
func (i *IBCInstance) ICS20TransferAddress() EVMAddress {
	return i.ics20Transfer
}

// ICS27GMPAddress returns the ICS27 GMP proxy. It is zero for attached
// instances, which resolve only the router and access manager.
func (i *IBCInstance) ICS27GMPAddress() EVMAddress {
	return i.ics27GMP
}

// IBCClient is one resolved end of an IBC Connection. ID is the stable
// authored identity; Locator is the actual client identifier registered in
// the host IBC Instance.
type IBCClient struct {
	id                    ClientID
	instance              *IBCInstance
	locator               IBCClientLocator
	lightClient           EVMAddress
	counterparty          IBCClientLocator
	attestors             []EVMAddress
	minRequiredSignatures uint8
}

func (c *IBCClient) ID() ClientID                          { return c.id }
func (c *IBCClient) IBCInstance() *IBCInstance             { return c.instance }
func (c *IBCClient) Locator() IBCClientLocator             { return c.locator }
func (c *IBCClient) LightClientAddress() EVMAddress        { return c.lightClient }
func (c *IBCClient) CounterpartyLocator() IBCClientLocator { return c.counterparty }
func (c *IBCClient) AttestorAddresses() []EVMAddress       { return slices.Clone(c.attestors) }
func (c *IBCClient) MinRequiredSignatures() uint8          { return c.minRequiredSignatures }

// Connection is a ready reciprocal IBC Client pair. A and B preserve the
// authored end labels; callers can also locate either Client directly through
// Environment.IBCClient.
type Connection struct {
	id ConnectionID
	a  *IBCClient
	b  *IBCClient
}

func (c *Connection) ID() ConnectionID { return c.id }
func (c *Connection) A() *IBCClient    { return c.a }
func (c *Connection) B() *IBCClient    { return c.b }

// Attestor is one running, config-loaded IBC Link Attestor process.
// ObservedIBCInstance is derived from the counterparty end of the Client's
// Connection.
//
// Its startup probe does not establish chain-derived attestation or proof
// capability, so this value intentionally exposes neither until the product
// process can provide them truthfully.
type Attestor struct {
	id       AttestorID
	client   *IBCClient
	observed *IBCInstance
	signer   EVMAddress
	endpoint string
	lease    *environmentLease

	mu       sync.Mutex
	process  *ibclink.AttestorProcess
	launch   ibclink.AttestorLaunch
	restarts int
}

func (a *Attestor) bindLease(lease *environmentLease) { a.lease = lease }

func (a *Attestor) use(use func() error) error {
	err := a.lease.use(use)
	if errors.Is(err, ErrEnvironmentClosed) {
		err = fmt.Errorf("%w: Attestor %q", err, a.id)
	}
	return err
}

func (a *Attestor) ID() AttestorID                    { return a.id }
func (a *Attestor) IBCClient() *IBCClient             { return a.client }
func (a *Attestor) ObservedIBCInstance() *IBCInstance { return a.observed }
func (a *Attestor) SignerAddress() EVMAddress         { return a.signer }

// Endpoint is the Attestor's gRPC listen address as a bare host:port, the
// form Link relayer configuration requires for remote attestor entries. It is
// stable across Restart.
func (a *Attestor) Endpoint() string { return a.endpoint }

// LatestHeight queries the attestor process's LatestHeight RPC.
func (a *Attestor) LatestHeight(ctx context.Context) (uint64, error) {
	var height uint64
	err := a.use(func() error {
		a.mu.Lock()
		process := a.process
		a.mu.Unlock()
		if process == nil {
			return fmt.Errorf("environment: Attestor %q has no running process", a.id)
		}
		var err error
		height, err = process.LatestHeight(ctx)
		return err
	})
	return height, err
}

// Stop terminates the attestor process. It is safe to call more than once;
// Restart brings the Attestor back afterwards.
func (a *Attestor) Stop(ctx context.Context) error {
	return a.use(func() error { return a.stopProcess(ctx) })
}

// stopProcess is the unleased stop shared with Environment cleanup, which
// runs after the lease is closed.
func (a *Attestor) stopProcess(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.process == nil {
		return nil
	}
	return a.process.Stop(ctx)
}

// Restart replaces the attestor process with one that keeps the attestor
// name, signer key, and listen endpoint, so a running relayer configuration
// stays valid. Each restart uses a fresh work directory because startup
// requires a nonexistent one.
func (a *Attestor) Restart(ctx context.Context) error {
	return a.use(func() error { return a.restartProcess(ctx) })
}

func (a *Attestor) restartProcess(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.process != nil {
		if err := a.process.Stop(ctx); err != nil {
			return fmt.Errorf("environment: restart Attestor %q: stop previous process: %w", a.id, err)
		}
	}
	launch := a.launch
	a.restarts++
	launch.WorkDir = fmt.Sprintf("%s-restart-%d", a.launch.WorkDir, a.restarts)
	launch.ListenAddress = a.endpoint
	process, err := ibclink.StartAttestor(ctx, launch)
	// A non-nil process alongside an error still owns a child; keep it so
	// Close can retry stopping it.
	a.process = process
	if err != nil {
		return fmt.Errorf("environment: restart Attestor %q: %w", a.id, err)
	}
	if process.Endpoint() != a.endpoint {
		return errors.Join(
			fmt.Errorf(
				"environment: restart Attestor %q: announced endpoint %q, want %q",
				a.id, process.Endpoint(), a.endpoint,
			),
			process.Stop(ctx),
		)
	}
	return nil
}
