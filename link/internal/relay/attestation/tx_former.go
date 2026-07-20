package attestation

import (
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/chains/evm/contracts/ics26router"
)

var routerABI = mustRouterABI()

func mustRouterABI() *abi.ABI {
	parsed, err := ics26router.ContractMetaData.GetAbi()
	if err != nil {
		panic(errors.Wrap(err, "parsing ics26 router abi"))
	}

	return parsed
}

// height converts a plain attested height into the router's versioned height
// type. EVM chains have no notion of a revision/epoch, so RevisionNumber is
// always 0.
func height(h uint64) ics26router.IICS02ClientMsgsHeight {
	return ics26router.IICS02ClientMsgsHeight{RevisionNumber: 0, RevisionHeight: uint32(h)} //nolint:gosec // heights fit in uint32 for the chains link targets
}

// packUpdateClient packs a call to updateClient(clientId, updateMsg), where
// updateMsg is the abi-encoded AttestationProof wrapping a StateAttestation.
func packUpdateClient(clientID string, proof AttestationProof) ([]byte, error) {
	updateMsg, err := encodeAttestationProof(proof)
	if err != nil {
		return nil, errors.Wrap(err, "encoding client update proof")
	}

	packed, err := routerABI.Pack("updateClient", clientID, updateMsg)
	if err != nil {
		return nil, errors.Wrap(err, "packing updateClient call")
	}

	return packed, nil
}

// packRecvPacket packs a call to recvPacket for one packet, with proof the
// abi-encoded AttestationProof wrapping a PacketAttestation.
func packRecvPacket(packet ics26router.IICS26RouterMsgsPacket, proof []byte, proofHeight uint64) ([]byte, error) {
	packed, err := routerABI.Pack("recvPacket", ics26router.IICS26RouterMsgsMsgRecvPacket{
		Packet:          packet,
		ProofCommitment: proof,
		ProofHeight:     height(proofHeight),
	})
	if err != nil {
		return nil, errors.Wrap(err, "packing recvPacket call")
	}

	return packed, nil
}

// packAckPacket packs a call to ackPacket for one packet.
func packAckPacket(
	packet ics26router.IICS26RouterMsgsPacket,
	acknowledgement []byte,
	proof []byte,
	proofHeight uint64,
) ([]byte, error) {
	packed, err := routerABI.Pack("ackPacket", ics26router.IICS26RouterMsgsMsgAckPacket{
		Packet:          packet,
		Acknowledgement: acknowledgement,
		ProofAcked:      proof,
		ProofHeight:     height(proofHeight),
	})
	if err != nil {
		return nil, errors.Wrap(err, "packing ackPacket call")
	}

	return packed, nil
}

// packTimeoutPacket packs a call to timeoutPacket for one packet, with proof
// the abi-encoded AttestationProof wrapping a non-membership PacketAttestation.
func packTimeoutPacket(packet ics26router.IICS26RouterMsgsPacket, proof []byte, proofHeight uint64) ([]byte, error) {
	packed, err := routerABI.Pack("timeoutPacket", ics26router.IICS26RouterMsgsMsgTimeoutPacket{
		Packet:       packet,
		ProofTimeout: proof,
		ProofHeight:  height(proofHeight),
	})
	if err != nil {
		return nil, errors.Wrap(err, "packing timeoutPacket call")
	}

	return packed, nil
}

// packMulticall wraps a batch of already-packed calls (one updateClient plus
// one packet-operation call per packet) as a single multicall(bytes[]) call,
// the one transaction the batch is actually submitted as.
func packMulticall(calls [][]byte) ([]byte, error) {
	packed, err := routerABI.Pack("multicall", calls)
	if err != nil {
		return nil, errors.Wrap(err, "packing multicall")
	}

	return packed, nil
}
