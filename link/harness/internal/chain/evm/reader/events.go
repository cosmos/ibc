package reader

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/cosmos/ibc/link/harness/fixtures"
)

const eventIFTReceived = "IFTReceived"

var mockIFTABI = fixtures.MockIFT.MustABI()

type iftReceivedLog struct {
	Seq      *big.Int
	Receiver common.Address
	Amount   *big.Int
}

func decodeIFTReceived(data []byte) (iftReceivedLog, error) {
	var ev iftReceivedLog
	if err := mockIFTABI.UnpackIntoInterface(&ev, eventIFTReceived, data); err != nil {
		return iftReceivedLog{}, fmt.Errorf("decode IFTReceived: %w", err)
	}
	return ev, nil
}

const eventIFTRefunded = "IFTRefunded"

type iftRefundedLog struct {
	Seq    *big.Int
	Sender common.Address
	Amount *big.Int
}

func decodeIFTRefunded(data []byte) (iftRefundedLog, error) {
	var ev iftRefundedLog
	if err := mockIFTABI.UnpackIntoInterface(&ev, eventIFTRefunded, data); err != nil {
		return iftRefundedLog{}, fmt.Errorf("decode IFTRefunded: %w", err)
	}
	return ev, nil
}

const eventGMPReceived = "GMPReceived"

var mockGMPABI = fixtures.MockGMP.MustABI()

type gmpReceivedLog struct {
	Seq    *big.Int
	Target common.Address
	// Success=false is the error-ack outcome, not a delivery failure.
	Success bool
}

func decodeGMPReceived(data []byte) (gmpReceivedLog, error) {
	var ev gmpReceivedLog
	if err := mockGMPABI.UnpackIntoInterface(&ev, eventGMPReceived, data); err != nil {
		return gmpReceivedLog{}, fmt.Errorf("decode GMPReceived: %w", err)
	}
	return ev, nil
}

var counterABI = fixtures.Counter.MustABI()

var counterIncrementCalldata = mustIncrementCalldata()

func mustIncrementCalldata() []byte {
	data, err := counterABI.Pack("increment")
	if err != nil {
		panic(fmt.Sprintf("onchain: pack Counter.increment(): %v", err))
	}
	return data
}
