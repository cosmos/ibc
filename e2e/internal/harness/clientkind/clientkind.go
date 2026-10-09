// SPDX-License-Identifier: Apache-2.0

// Package clientkind names the light client kinds as the relayer configures
// them, for every harness package that deploys, attaches or configures one.
// They are the IBC CLI's public config values, restated here the way any
// external caller would: the harness is a black box to the CLI.
package clientkind

// Kind names a light client implementation as the relayer configures it.
type Kind string

const (
	Attestation Kind = "attestation"
	BesuQBFT    Kind = "besu-qbft"
	Remote      Kind = "remote"
)
