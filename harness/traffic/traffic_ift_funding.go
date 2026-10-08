// SPDX-License-Identifier: Apache-2.0

package traffic

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/cosmos/ibc/gen/go/solidity-abi/iftbatchtransfershim"
)

// ercTransferChunk caps the recipients per batchTransfer transaction so one
// funding transaction stays well inside a block's gas limit.
const ercTransferChunk = 64

// ERCTransferRequest is one same-chain ERC-20 payment of the route's source
// IFT from the batch shim's balance.
type ERCTransferRequest struct {
	Address common.Address
	Amount  *big.Int
}

// Coins returns n whole tokens (n * 10^18 base units) of an 18-decimal token.
func Coins(n int64) *big.Int {
	return new(big.Int).Mul(big.NewInt(n), big.NewInt(weiPerEther))
}

// TransferBatch pays each request in the route's source IFT, on the source
// Chain, out of the batch shim's IFT balance (the shim is minted the initial
// supply at deployment). The route's sender submits one batchTransfer
// transaction per 64 recipients, in order, and TransferBatch returns their
// receipts. Every request is validated before any transaction is sent; a
// failed chunk stops the batch, leaving earlier chunks paid.
func (i *IFT) TransferBatch(ctx context.Context, requests []ERCTransferRequest) ([]*types.Receipt, error) {
	recipients := make([]iftbatchtransfershim.IFTBatchTransferShimRecipient, len(requests))
	for k, request := range requests {
		amount, err := validAmount(request.Amount)
		if err != nil {
			return nil, fmt.Errorf("traffic: ERC transfer %d: %w", k, err)
		}
		if request.Address == (common.Address{}) {
			return nil, fmt.Errorf("traffic: ERC transfer %d has a zero address", k)
		}
		recipients[k] = iftbatchtransfershim.IFTBatchTransferShimRecipient{
			Account: request.Address,
			Amount:  amount,
		}
	}

	receipts := make([]*types.Receipt, 0, (len(recipients)+ercTransferChunk-1)/ercTransferChunk)
	for start := 0; start < len(recipients); start += ercTransferChunk {
		end := min(start+ercTransferChunk, len(recipients))
		data, err := calldata(func(opts *bind.TransactOpts) (*types.Transaction, error) {
			return iftBatchShimTransactor.BatchTransfer(opts, i.sourceIFT, recipients[start:end])
		})
		if err != nil {
			return receipts, fmt.Errorf("traffic: pack IFT batchTransfer on route %q: %w", i.routeID, err)
		}
		receipt, err := i.source.evm.BroadcastTx(ctx, i.sender, &i.batcher, data, nil)
		if err != nil {
			return receipts, fmt.Errorf(
				"traffic: ERC transfer batch %d:%d on route %q: %w",
				start, end, i.routeID, err,
			)
		}
		receipts = append(receipts, receipt)
	}
	return receipts, nil
}
