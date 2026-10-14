#!/bin/bash -eu
# SPDX-License-Identifier: Apache-2.0

# Builds every native Go fuzz test in the cli module as a libFuzzer target
# named after its function.

cd "$SRC/ibc/cli"

grep -rlE --include='*_test.go' '^func Fuzz\w*\(f \*testing\.F\)' . | sort | while read -r file; do
  pkg=$(go list "./$(dirname "$file")")
  for fn in $(grep -oE '^func Fuzz\w*' "$file" | cut -d' ' -f2); do
    compile_native_go_fuzzer_v2 "$pkg" "$fn" "$fn"
  done
done
