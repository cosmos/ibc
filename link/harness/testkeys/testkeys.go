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
)
