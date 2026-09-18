// SPDX-License-Identifier: Apache-2.0

package besu

import "errors"

// ErrInvalidHeader indicates a header cannot be decoded for payload construction.
var ErrInvalidHeader = errors.New("invalid besu header")
