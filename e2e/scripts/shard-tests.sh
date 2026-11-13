#!/usr/bin/env bash

# SPDX-License-Identifier: Apache-2.0

# Prints a `go test -run` pattern for shard I/N: every Nth top-level test of the e2e package in
# `go test -list` order, starting at test I. Together the shards run each test once.

set -euo pipefail

if ! [[ "${1-}" =~ ^([1-9][0-9]*)/([1-9][0-9]*)$ ]]; then
	printf 'Usage: %s I/N\n' "$0" >&2
	exit 2
fi

cd "$(dirname "$0")/.."

tests="$(go test . -list . | awk -v i="${BASH_REMATCH[1]}" -v n="${BASH_REMATCH[2]}" '/^Test/ && c++ % n == i - 1' | paste -sd '|')"
if [ -z "$tests" ]; then
	printf 'shard %s selects no tests\n' "$1" >&2
	exit 1
fi
printf '^(%s)$\n' "$tests"
