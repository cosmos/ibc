package evm

import (
	"crypto/sha256"
	"encoding/binary"

	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

const commitmentVersion byte = 0x02

func PacketCommitment(packet v2.Packet) [32]byte {
	destClientHash := sha256.Sum256([]byte(packet.DestClient))

	var timeoutBytes [8]byte
	binary.BigEndian.PutUint64(timeoutBytes[:], packet.TimeoutTimestamp)
	timeoutHash := sha256.Sum256(timeoutBytes[:])

	payloadCommitments := make([]byte, 0, sha256.Size*len(packet.Payloads))
	for _, payload := range packet.Payloads {
		commitment := payloadCommitment(payload)
		payloadCommitments = append(payloadCommitments, commitment[:]...)
	}
	payloadsHash := sha256.Sum256(payloadCommitments)

	data := make([]byte, 0, 1+3*sha256.Size)
	data = append(data, commitmentVersion)
	data = append(data, destClientHash[:]...)
	data = append(data, timeoutHash[:]...)
	data = append(data, payloadsHash[:]...)

	return sha256.Sum256(data)
}

func payloadCommitment(payload v2.Payload) [32]byte {
	sourcePortHash := sha256.Sum256([]byte(payload.SourcePort))
	destPortHash := sha256.Sum256([]byte(payload.DestPort))
	versionHash := sha256.Sum256([]byte(payload.Version))
	encodingHash := sha256.Sum256([]byte(payload.Encoding))
	valueHash := sha256.Sum256(payload.Value)

	data := make([]byte, 0, 5*sha256.Size)
	data = append(data, sourcePortHash[:]...)
	data = append(data, destPortHash[:]...)
	data = append(data, versionHash[:]...)
	data = append(data, encodingHash[:]...)
	data = append(data, valueHash[:]...)

	return sha256.Sum256(data)
}
