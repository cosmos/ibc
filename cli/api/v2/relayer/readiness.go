// SPDX-License-Identifier: Apache-2.0

package relayer

// ProcessReadinessEvent identifies the relayer startup event.
const ProcessReadinessEvent = "ready"

// ProcessReadiness announces the relayer's connected chains and bound HTTP address.
type ProcessReadiness struct {
	Event           string   `json:"event"`
	ChainsConnected []string `json:"chainsConnected"`
	HTTP            string   `json:"http"`
}
