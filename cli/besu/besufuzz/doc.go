// SPDX-License-Identifier: Apache-2.0

// Package besufuzz fuzzes package besu. It is separate because ClusterFuzzLite
// builds every test file beside a fuzz target into one package, and besu's
// external tests are package besu_test.
package besufuzz
