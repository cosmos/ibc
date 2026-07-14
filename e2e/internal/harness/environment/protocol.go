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
}

func (i *IBCInstance) ID() IBCInstanceID           { return i.id }
func (i *IBCInstance) Chain() *Chain               { return i.chain }
func (i *IBCInstance) Locator() IBCInstanceLocator { return i.locator }
func (i *IBCInstance) AccessManagerAddress() EVMAddress {
	return i.accessManager
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

type EVMSubmissionStatus string

const (
	EVMSubmissionAccepted  EVMSubmissionStatus = "accepted"
	EVMSubmissionAmbiguous EVMSubmissionStatus = "ambiguous"
)

// EVMTransactionEvidence is the safe setup-transaction evidence available when
// Start returns. Hash is present once a transaction is signed, before broadcast.
// Submission distinguishes RPC acceptance from an ambiguous broadcast result;
// Mined reports whether a receipt was observed. PredictedContractAddress is
// known from a deployment transaction before broadcast; ContractAddress is
// populated only from a mined receipt. Mined does not claim finality.
type EVMTransactionEvidence struct {
	Hash                     string              `json:"hash"`
	Submission               EVMSubmissionStatus `json:"submission"`
	Mined                    bool                `json:"mined"`
	BlockNumber              uint64              `json:"blockNumber"`
	Status                   uint64              `json:"status"`
	PredictedContractAddress EVMAddress          `json:"predictedContractAddress,omitempty"`
	ContractAddress          EVMAddress          `json:"contractAddress,omitempty"`
}

// IBCInstanceReceipt preserves the known deployment prefix for a new IBC
// Instance. Later fields are nil when setup failed before signing them.
type IBCInstanceReceipt struct {
	ID                   IBCInstanceID           `json:"id"`
	Chain                ChainID                 `json:"chain"`
	AccessManager        *EVMTransactionEvidence `json:"accessManager,omitempty"`
	RouterImplementation *EVMTransactionEvidence `json:"routerImplementation,omitempty"`
	RouterProxy          *EVMTransactionEvidence `json:"routerProxy,omitempty"`
}

// IBCClientReceipt preserves one Connection end. Locator is the intended
// custom protocol identifier; Registration is nil until that identifier has
// been durably registered. Existing Clients have no setup transactions.
type IBCClientReceipt struct {
	ID                    ClientID                `json:"id"`
	IBCInstance           IBCInstanceID           `json:"ibcInstance"`
	Locator               IBCClientLocator        `json:"locator"`
	LightClientAddress    EVMAddress              `json:"lightClientAddress,omitempty"`
	LightClientDeployment *EVMTransactionEvidence `json:"lightClientDeployment,omitempty"`
	Registration          *EVMTransactionEvidence `json:"registration,omitempty"`
}

// IBCConnectionReceipt preserves each resolved or submitted end independently
// so failure on B cannot hide work already performed on A.
type IBCConnectionReceipt struct {
	ID ConnectionID      `json:"id"`
	A  *IBCClientReceipt `json:"a,omitempty"`
	B  *IBCClientReceipt `json:"b,omitempty"`
}

func cloneEVMTransactionEvidence(in *EVMTransactionEvidence) *EVMTransactionEvidence {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func cloneIBCInstanceReceipt(in IBCInstanceReceipt) IBCInstanceReceipt {
	in.AccessManager = cloneEVMTransactionEvidence(in.AccessManager)
	in.RouterImplementation = cloneEVMTransactionEvidence(in.RouterImplementation)
	in.RouterProxy = cloneEVMTransactionEvidence(in.RouterProxy)
	return in
}

func cloneIBCClientReceipt(in *IBCClientReceipt) *IBCClientReceipt {
	if in == nil {
		return nil
	}
	out := *in
	out.LightClientDeployment = cloneEVMTransactionEvidence(in.LightClientDeployment)
	out.Registration = cloneEVMTransactionEvidence(in.Registration)
	return &out
}

func cloneIBCConnectionReceipt(in IBCConnectionReceipt) IBCConnectionReceipt {
	in.A = cloneIBCClientReceipt(in.A)
	in.B = cloneIBCClientReceipt(in.B)
	return in
}
