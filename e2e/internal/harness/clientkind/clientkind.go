// SPDX-License-Identifier: Apache-2.0

// Package clientkind names the light client kinds as the relayer configures
// them, for every harness package that deploys, attaches or configures one.
// They are the IBC CLI's public config values, restated here the way any
// external caller would: the harness is a black box to the CLI.
package clientkind

import "github.com/cosmos/ibc/cli/besu"

// Kind names a light client implementation as the relayer configures it.
type Kind string

const (
	Attestation Kind = "attestation"
	BesuQBFT    Kind = "besu-qbft"
	BesuIBFT2   Kind = "besu-ibft2"
	Remote      Kind = "remote"
)

// Besu is the kind of the Besu client that verifies a chain running
// consensus, empty for an unknown consensus.
func Besu(consensus besu.Consensus) Kind {
	switch consensus {
	case besu.QBFT:
		return BesuQBFT
	case besu.IBFT2:
		return BesuIBFT2
	default:
		return ""
	}
}

// BesuConsensus is the consensus a Besu kind verifies, false for any other
// kind.
func (k Kind) BesuConsensus() (besu.Consensus, bool) {
	switch k {
	case BesuQBFT:
		return besu.QBFT, true
	case BesuIBFT2:
		return besu.IBFT2, true
	default:
		return "", false
	}
}
