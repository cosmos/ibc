// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/ecdsa"
	"math/big"
	"strings"
	"time"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ift"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/cosmos/ibc/link/internal/config"
)

var (
	cmdTxIFT = &cobra.Command{
		Use:   useIFT,
		Short: "IFT transaction subcommands",
		Long: "IFT transaction subcommands. Each takes its core transfer arguments positionally " +
			"(e.g. to/amount, client-id/receiver/amount) and --chain/--ift/--from as flags shared " +
			"across mint and send -- run a subcommand's --help for its exact positional order.",
	}

	cmdTxIFTMint = &cobra.Command{
		Use:   "mint [to] [amount]",
		Short: "Mint IFT supply to an address",
		Long:  "Mint amount of the IFT token at --ift to the to address. The --from signer must be the token's owner.",
		Example: "  ibc tx ift mint 0xRecipient... 1000000000000000000 \\\n" +
			"    --chain 1 --ift 0xIFTTokenAddress... --from deployer",
		Args: cobra.ExactArgs(2),
		RunE: txIFTMint,
	}

	cmdTxIFTSend = &cobra.Command{
		Use:   "send [client-id] [receiver] [amount]",
		Short: "Send IFT across a bridge to a counterparty chain",
		Long: "Initiate a cross-chain transfer of amount of the IFT token at --ift, over the bridge " +
			"registered for client-id, to receiver on the counterparty chain.",
		Example: "  ibc tx ift send link-1-2 0xReceiver... 500000000000000000 \\\n" +
			"    --chain 1 --ift 0xIFTTokenAddress... --from deployer",
		Args: cobra.ExactArgs(3),
		RunE: txIFTSend,
	}
)

var (
	flagTxIFTChain   string
	flagTxIFTAddress string
	flagTxIFTFrom    string
	flagTxIFTTimeout time.Duration
)

func txIFTMint(cmd *cobra.Command, args []string) error {
	to := args[0]
	amount := parseIFTAmount(args[1])
	if amount == nil {
		return errors.Errorf("invalid amount %q", args[1])
	}
	if !common.IsHexAddress(to) {
		return errors.Errorf("invalid to address %q", to)
	}

	backend, key, chainID, err := dialIFTChain(cmd.Context())
	if err != nil {
		return err
	}

	contract, err := ift.NewContract(common.HexToAddress(flagTxIFTAddress), backend)
	if err != nil {
		return err
	}

	opts, err := bind.NewKeyedTransactorWithChainID(key, chainID)
	if err != nil {
		return err
	}
	opts.Context = cmd.Context()

	tx, err := contract.Mint(opts, common.HexToAddress(to), amount)
	if err != nil {
		return errors.Wrap(err, "mint")
	}

	receipt, err := bind.WaitMined(cmd.Context(), backend, tx)
	if err != nil {
		return errors.Wrap(err, "mint: wait mined")
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return errors.Errorf("mint: transaction %s reverted", tx.Hash())
	}

	return printTxHash(tx.Hash().Hex())
}

func txIFTSend(cmd *cobra.Command, args []string) error {
	clientID, receiver := args[0], args[1]
	amount := parseIFTAmount(args[2])
	if amount == nil {
		return errors.Errorf("invalid amount %q", args[2])
	}

	backend, key, chainID, err := dialIFTChain(cmd.Context())
	if err != nil {
		return err
	}

	contract, err := ift.NewContract(common.HexToAddress(flagTxIFTAddress), backend)
	if err != nil {
		return err
	}

	opts, err := bind.NewKeyedTransactorWithChainID(key, chainID)
	if err != nil {
		return err
	}
	opts.Context = cmd.Context()

	if flagTxIFTTimeout <= 0 {
		return errors.Errorf("invalid --timeout %s: must be positive", flagTxIFTTimeout)
	}
	timeout := uint64(time.Now().Add(flagTxIFTTimeout).Unix())

	tx, err := contract.IftTransfer(opts, clientID, receiver, amount, timeout)
	if err != nil {
		return errors.Wrapf(err, "iftTransfer %q", clientID)
	}

	receipt, err := bind.WaitMined(cmd.Context(), backend, tx)
	if err != nil {
		return errors.Wrapf(err, "iftTransfer %q: wait mined", clientID)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return errors.Errorf("iftTransfer %q: transaction %s reverted", clientID, tx.Hash())
	}

	return printTxHash(tx.Hash().Hex())
}

func printTxHash(txHash string) error {
	return config.PrintJSON(map[string]string{"txHash": txHash})
}

func parseIFTAmount(s string) *big.Int {
	amount, ok := new(big.Int).SetString(s, 10)
	if !ok || amount.Sign() < 0 {
		return nil
	}
	return amount
}

// dialIFTChain resolves --chain's RPC and --from's signer from config, and
// dials the chain.
func dialIFTChain(ctx context.Context) (*ethclient.Client, *ecdsa.PrivateKey, *big.Int, error) {
	cfg, err := setupHomeWithConfig()
	if err != nil {
		return nil, nil, nil, err
	}

	chain, ok := cfg.Chain(flagTxIFTChain)
	if !ok {
		return nil, nil, nil, errors.Errorf("chain %q not declared in config", flagTxIFTChain)
	}
	if chain.Type() != config.ChainTypeEVM {
		return nil, nil, nil, errors.Errorf("chain %q is not an EVM chain", flagTxIFTChain)
	}

	keyHex, err := deployerKeyHex(cfg, flagTxIFTFrom)
	if err != nil {
		return nil, nil, nil, err
	}
	key, err := crypto.HexToECDSA(strings.TrimPrefix(keyHex, "0x"))
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "signer key")
	}

	backend, err := ethclient.DialContext(ctx, chain.EVM.RPC)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "dial %s", chain.EVM.RPC)
	}

	chainID, err := backend.ChainID(ctx)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "query chain id")
	}

	return backend, key, chainID, nil
}
