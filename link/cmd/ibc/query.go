// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ift"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/cosmos/ibc/link/internal/config"
)

var (
	cmdQuery = &cobra.Command{
		Use:     "query",
		Short:   "Query commands",
		Aliases: []string{"q"},
	}

	cmdQueryIFT = &cobra.Command{
		Use:   useIFT,
		Short: "IFT query subcommands",
	}

	cmdQueryIFTBalance = &cobra.Command{
		Use:   "balance",
		Short: "Query an address's IFT token balance",
		Example: "  ibc query ift balance --chain 1 --ift 0xIFTTokenAddress... \\\n" +
			"    --address 0xAccount...",
		RunE: queryIFTBalance,
	}
)

var (
	flagQueryIFTChain   string
	flagQueryIFTAddress string
	flagQueryIFTAccount string
)

func queryIFTBalance(cmd *cobra.Command, _ []string) error {
	cfg, err := setupHomeWithConfig()
	if err != nil {
		return err
	}

	chain, ok := cfg.Chain(flagQueryIFTChain)
	if !ok {
		return errors.Errorf("chain %q not declared in config", flagQueryIFTChain)
	}
	if chain.Type() != config.ChainTypeEVM {
		return errors.Errorf("chain %q is not an EVM chain", flagQueryIFTChain)
	}

	account, err := resolveEVMAddress(cfg, "address", flagQueryIFTAccount)
	if err != nil {
		return err
	}

	backend, err := ethclient.DialContext(cmd.Context(), chain.EVM.RPC)
	if err != nil {
		return errors.Wrapf(err, "dial %s", chain.EVM.RPC)
	}

	contract, err := ift.NewContract(common.HexToAddress(flagQueryIFTAddress), backend)
	if err != nil {
		return err
	}

	opts := &bind.CallOpts{Context: cmd.Context()}

	balance, err := contract.BalanceOf(opts, common.HexToAddress(account))
	if err != nil {
		return errors.Wrap(err, "balanceOf")
	}
	symbol, err := contract.Symbol(opts)
	if err != nil {
		return errors.Wrap(err, "symbol")
	}

	return config.PrintJSON(map[string]string{
		"address": account,
		"symbol":  symbol,
		"balance": balance.String(),
	})
}
