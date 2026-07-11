// Package fixturekeys is the harness-owned vocabulary of the test-fixture deployment: the well-known
// keys into wire.ChainDeployment.Fixtures. These are the stub's MOCK mechanism, not spec-derived — the wire contract (what the real binary
// will eventually speak) carries only a generic name->address map, and this package supplies the names the
// stub writes and the harness readers read. Keeping the vocabulary here, not in wire, is the deploy seam:
// swapping the stub for a real deploy replaces this package and deletes the stub, without rewriting the
// wire schema or the readers' correlation machinery.
//
// The keys are defined once here (producer = the stub's deploy; consumer = the EVM reader) so a
// rename cannot drift the two sides apart silently — a fixture the harness looks up by a name the stub
// never wrote is a clear "no such fixture" error, not a zero address.
package fixturekeys

// Fixture keys: the well-known names the deploy records fixtures under and the readers look them up by.
const (
	MockIFT = "mockIFT" // test-only EVM IFT adapter address
	MockGMP = "mockGMP" // test-only GMP messenger fixture
	Counter = "counter" // test-only Counter (GMP target) fixture
	// IFTFaucet is the account a route's source IFT debits on its source chain — the IFT source holder, in
	// the chain's native string form. Deploy emits it so the harness reads the same holder's balance to
	// assert the source debit.
	IFTFaucet = "iftFaucet"
)
