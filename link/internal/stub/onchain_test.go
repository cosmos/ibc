package stub

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestMockIFTSentFromReceipt(t *testing.T) {
	addr := common.HexToAddress("0x100")
	txHash := common.HexToHash("0xabc")
	data, err := mockIFTABI.Events["IFTSent"].Inputs.NonIndexed().Pack(
		big.NewInt(7), "route-a", "0xreceiver", big.NewInt(42), big.NewInt(99),
	)
	if err != nil {
		t.Fatal(err)
	}
	receipt := &types.Receipt{Logs: []*types.Log{
		{Address: common.HexToAddress("0x200"), Topics: []common.Hash{mockIFTABI.Events["IFTSent"].ID}, Data: data},
		{Address: addr, Topics: []common.Hash{mockIFTABI.Events["IFTSent"].ID}, Data: data, TxHash: txHash},
	}}

	got, found, err := newTestAppIFT(addr, nil).SentFromReceipt(receipt)
	if err != nil || !found {
		t.Fatalf("found = %v, err = %v", found, err)
	}
	if got.Seq.Uint64() != 7 || got.RouteID != "route-a" || got.Receiver != "0xreceiver" ||
		got.Amount.Uint64() != 42 || got.TimeoutTimestamp.Uint64() != 99 || got.TxHash != txHash {
		t.Fatalf("event = %+v", got)
	}
}

func TestMockGMPSentFromReceipt(t *testing.T) {
	addr := common.HexToAddress("0x100")
	txHash := common.HexToHash("0xdef")
	data, err := mockGMPABI.Events["GMPSent"].Inputs.NonIndexed().Pack(
		big.NewInt(8), "route-b", "0xtarget", []byte{1, 2, 3},
	)
	if err != nil {
		t.Fatal(err)
	}
	receipt := &types.Receipt{Logs: []*types.Log{
		{Address: addr, Topics: []common.Hash{mockGMPABI.Events["GMPSent"].ID}, Data: data, TxHash: txHash},
	}}

	got, found, err := newTestAppGMP(addr, nil).SentFromReceipt(receipt)
	if err != nil || !found {
		t.Fatalf("found = %v, err = %v", found, err)
	}
	if got.Seq.Uint64() != 8 || got.RouteID != "route-b" || got.Target != "0xtarget" ||
		string(got.Payload) != string([]byte{1, 2, 3}) || got.TxHash != txHash {
		t.Fatalf("event = %+v", got)
	}
}
