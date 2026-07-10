// Package testkeys holds the public deterministic EVM identities shared by the harness and stub.
package testkeys

const (
	// FaucetPrivateKeyHex is the EVM dev faucet private key used by Anvil and the harness.
	FaucetPrivateKeyHex = "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
	// FaucetAddressHex is the EVM dev faucet address derived from FaucetPrivateKeyHex.
	FaucetAddressHex = "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"

	// RelayerPrivateKeyHex is the deterministic EVM relayer private key used by tests.
	RelayerPrivateKeyHex = "59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"
	// RelayerAddressHex is the deterministic EVM relayer address derived from RelayerPrivateKeyHex.
	RelayerAddressHex = "0x70997970C51812dc3A010C7d01b50e0d17dc79C8"

	// CosmosSignerPrivateKeyHex is the plain secp256k1 (not eth_secp256k1) key used by the Cosmos relayer
	// and test IFT authority. Its cosmos address is RIPEMD160(SHA256(compressed-pubkey)) — a different account
	// from the same key's EVM address, so it is a distinct identity from the EVM faucet/relayer above. Both
	// the harness (genesis funding + oracle) and the stub (signing) derive the bech32 from this one hex key,
	// each with its own copy of the derivation, so the black-box wall stays honest. Test-only, carried in the
	// clear: a real relayer config would reference the signer credential, never inline it.
	CosmosSignerPrivateKeyHex = "5de4111afa1a4b94908f83103eb1f1706367c2e68ca870fc3fb9a804cdab365a"

	// CosmosFaucetPrivateKeyHex is the plain secp256k1 key of the Cosmos user whose native IFT tokens are
	// burned when it transfers. It is the Cosmos analog of the EVM FaucetPrivateKeyHex and is deliberately
	// distinct from CosmosSignerPrivateKeyHex, so the source user and relayer/admin are separate accounts — matching
	// the EVM side, where the faucet submits `sendTransfer` and the relayer signs the destination effect. The
	// harness genesis-funds this key's bech32 and records it as the IFT source holder;
	// the stub derives the same bech32 to sign the source send. Test-only, in the clear, same as the others.
	CosmosFaucetPrivateKeyHex = "7c852118294e51e653712a81e05800f419141751be58f605c371e15141b007a6"

	// AttestorPrivateKeyHex is the EVM (secp256k1) key of the single test attestor the stub stands up for the
	// cosmos chain's `attestations` light client. On `deploy` the stub creates an attestations client whose
	// only authorized attestor is this key's Ethereum EOA, with minRequiredSigs=1; on every evm->cosmos GMP
	// delivery the stub signs the packet-commitment attestation with this key so the on-chain light client's
	// quorum check passes (1-of-1). It plays the role a real off-chain attestor set would: the stub is both
	// the mock EVM source and the attestor, so it can forge a valid attestation over the packet it fabricated
	// — which is the honest analog of a real attestor observing the source chain. Only the stub uses it (the
	// harness never signs attestations). Test-only, carried in the clear.
	AttestorPrivateKeyHex = "a11ce5ada11ce5ada11ce5ada11ce5ada11ce5ada11ce5ada11ce5ada11ce5ad"
)
