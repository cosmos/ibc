package evm

import (
	"encoding/binary"

	"github.com/ethereum/go-ethereum/crypto"
)

const (
	packetPathSeparator  byte = 0x01
	receiptPathSeparator byte = 0x02
	ackPathSeparator     byte = 0x03
)

func PathPacket(sourceClient string, sequence uint64) []byte {
	return commitmentPath(sourceClient, packetPathSeparator, sequence)
}

func PathReceipt(destClient string, sequence uint64) []byte {
	return commitmentPath(destClient, receiptPathSeparator, sequence)
}

func PathAck(destClient string, sequence uint64) []byte {
	return commitmentPath(destClient, ackPathSeparator, sequence)
}

func PathHash(path []byte) [32]byte {
	return crypto.Keccak256Hash(path)
}

func commitmentPath(clientID string, separator byte, sequence uint64) []byte {
	path := make([]byte, 0, len(clientID)+1+8)
	path = append(path, clientID...)
	path = append(path, separator)

	var sequenceBytes [8]byte
	binary.BigEndian.PutUint64(sequenceBytes[:], sequence)

	return append(path, sequenceBytes[:]...)
}
