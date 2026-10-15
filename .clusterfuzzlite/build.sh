#!/bin/bash -eu
# SPDX-License-Identifier: Apache-2.0

# Builds every native Go fuzz test in the cli module as a libFuzzer target
# named <package dir>_<function>, seeded from its testdata/fuzz corpus.

# The seed tool renames its zip into $OUT, so it must stage it there too.
seeds_tmp=$(mktemp -d "$OUT/seeds.XXXXXX")
trap 'rm -rf "$seeds_tmp"' EXIT

grep -roE --include='*_test.go' '^func Fuzz\w+\(\w+ \*testing\.F\)' . | sort | while IFS=: read -r file match; do
  dir=$(dirname "${file#./}")
  fn=${match#func }
  fn=${fn%%(*}
  name="${dir//\//_}_$fn"

  compile_native_go_fuzzer_v2 "$(go list "./$dir")" "$fn" "$name"
  # The helper exits 0 when it cannot find the function.
  [ -f "$OUT/$name" ] || { echo "fuzz target $name was not built" >&2; exit 1; }

  if [ -d "$dir/testdata/fuzz/$fn" ]; then
    TMPDIR=$seeds_tmp addStdLibCorpusToFuzzer -verbose -dir "$dir/testdata/fuzz/$fn" -fuzzer_name "$name"
  fi
done
