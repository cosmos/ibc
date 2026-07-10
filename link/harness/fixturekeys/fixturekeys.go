// Package fixturekeys is the harness-owned vocabulary of the test-fixture deployment: the well-known
// keys into wire.ChainDeployment.Fixtures and the native-token / counter-as-balance semantics behind
// them. These are the stub's MOCK mechanism, not spec-derived — the wire contract (what the real binary
// will eventually speak) carries only a generic name->address map, and this package supplies the names the
// stub writes and the harness readers read. Keeping the vocabulary here, not in wire, is the deploy seam:
// swapping the stub for a real deploy replaces this package and deletes the stub, without rewriting the
// wire schema or the readers' correlation machinery.
//
// The keys are defined once here (producer = the stub's deploy; consumers = both family readers) so a
// rename cannot drift the two sides apart silently — a fixture the harness looks up by a name the stub
// never wrote is a clear "no such fixture" error, not a zero address.
package fixturekeys

// Fixture keys: the well-known names the deploy records fixtures under and the readers look them up by.
const (
	MockIFT = "mockIFT" // test-only EVM IFT adapter address; Cosmos uses its native IFT module
	MockGMP = "mockGMP" // test-only GMP messenger fixture
	Counter = "counter" // test-only Counter (GMP target) fixture
	// IFTDenom is the cosmos native IFT tokenfactory denom registered during deployment. The reader uses it
	// for balance and event checks, and the submitter passes it to MsgIFTTransfer. Absent on EVM chains.
	IFTDenom = "iftDenom"
	// GMPDenom is the cosmos GMP analogue's denom fixture: the bank denom the GMP counter target's state
	// stands in as (e.g. "ugmpc"), the cosmos analog of the solidity Counter's count(). A cosmos chain has no
	// Counter contract — its GMP "counter" is a dedicated bank denom minted to the relayer at genesis, and one
	// increment is exactly one <gmpDenom> relayer->target send, so the target's bank balance of this denom is
	// the count. The reader reads that balance for GMPCount; the relayer's executor sends it on an increment.
	// Absent on EVM chains (they carry the Counter fixture instead).
	GMPDenom = "gmpDenom"
	// IFTFaucet is the account a route's source IFT debits on its source chain — the IFT source holder, in
	// the chain family's own native string form. deploy emits it per chain so the harness reads the same
	// holder's balance to assert the source debit, with one code path and no chain-family assumption: an EVM
	// chain emits its dev faucet 0x hex, and a cosmos chain emits the user/faucet bech32 whose tokens the native
	// IFT module burns while the transfer is pending.
	IFTFaucet = "iftFaucet"
	// AttestationsClient is the cosmos-destination GMP fixture's client id: the id of the `attestations` light
	// client the stub creates on `deploy` (e.g. "attestations-0"), whose sole attestor is the stub's test EOA.
	// Every evm->cosmos GMP delivery is a real IBC v2 MsgRecvPacket verified by this client, and the harness
	// reader keys its recv/ack tx_search on this destination client id. Absent on EVM chains (they carry no
	// real light client — GMP there is the MockGMP fixture).
	AttestationsClient = "attestationsClient"
	// ICS27Account is the cosmos-destination GMP fixture's executor account: the deterministic ICS-27 account
	// address (bech32) the 27-gmp module derives from (destination client id, the fixed evm->cosmos GMP
	// sender, empty salt) and runs the delivered CosmosTx as. `deploy` funds it with the GMP counter denom so
	// an increment (a bank MsgSend of 1 counter-denom from this account to the counter target, executed by the
	// module) has funds; the reader reads it as the inner-transfer sender to recover the increment's target.
	// Absent on EVM chains.
	ICS27Account = "ics27Account"
)
