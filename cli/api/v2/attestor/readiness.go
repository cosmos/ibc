// SPDX-License-Identifier: Apache-2.0

package attestor

// ProcessReadinessEvent identifies the attestor startup event.
const ProcessReadinessEvent = "ready"

// ProcessReadiness announces the attestor's bound HTTP address.
type ProcessReadiness struct {
	Event string `json:"event"`
	HTTP  string `json:"http"`
}
