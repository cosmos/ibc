package environment

import "slices"

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
}

func (a *Attestor) ID() AttestorID                    { return a.id }
func (a *Attestor) IBCClient() *IBCClient             { return a.client }
func (a *Attestor) ObservedIBCInstance() *IBCInstance { return a.observed }
func (a *Attestor) SignerAddress() EVMAddress         { return a.signer }
