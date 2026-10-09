#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# Resolves the advisories in .govulncheck-ignore to GitHub Advisory (GHSA) identifiers, so
# that actions/dependency-review-action can be handed the same accepted list govulncheck
# reads. Prints them comma-separated, the format the action's allow-ghsas input wants.
#
#   ./scripts/govulncheck-ignore-ghsas.sh
#   ./scripts/govulncheck-ignore-ghsas.sh --ignore-file <file>
#
# The two databases key the same advisory differently -- GO-ID in the Go database, GHSA in
# GitHub's -- so they have to be mapped through the Go database's alias list. An entry with
# no GHSA alias exists only in the Go database, which means the action cannot report it and
# there is nothing to allow; that is reported on stderr and skipped.
#
# A lookup that fails is an error rather than a skip: silently dropping an entry would fail
# the dependency review on an advisory this repository has already accepted.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

IGNORE_FILE="${REPO_ROOT}/.govulncheck-ignore"
GO_VULN_DB="https://vuln.go.dev"

die() {
  echo "govulncheck-ignore-ghsas: $*" >&2
  exit 2
}

# The header comment is the help text. Printing it up to the first non-comment line, rather
# than to a hardcoded line number, keeps the two from drifting apart as the comment is
# edited -- a stale range silently spills the script's own code into `--help`.
usage() {
  sed -n '3,${/^#/!q; s|^# \{0,1\}||; p;}' "${BASH_SOURCE[0]}"
}

# `shift 2` past the end of the arguments fails under `set -e`, which would exit 1 with no
# message at all, so a flag's value is checked before it is consumed.
need_value() {
  [[ $# -ge 2 && -n "$2" ]] || die "$1 requires a value"
}

while (($# > 0)); do
  case "$1" in
    --ignore-file) need_value "$@"; IGNORE_FILE="$2"; shift 2 ;;
    -h | --help) usage; exit 0 ;;
    *) die "unknown argument '$1'" ;;
  esac
done

[[ -f "${IGNORE_FILE}" ]] || die "ignore file '${IGNORE_FILE}' does not exist"
command -v jq >/dev/null || die "jq is required"

# Everything after a '#' is a comment, so entries can carry their reason.
ids="$(sed 's/#.*//' "${IGNORE_FILE}" | tr -s '[:space:]' '\n' | sed '/^$/d')"

ghsas=""

for id in ${ids}; do
  # Checked before it reaches a URL: a typo should be a clear error, not a odd request.
  [[ "${id}" =~ ^GO-[0-9]{4}-[0-9]+$ ]] \
    || die "'${id}' in ${IGNORE_FILE} is not a Go advisory ID"

  json="$(curl -fsS --retry 3 --retry-delay 2 "${GO_VULN_DB}/ID/${id}.json")" \
    || die "could not read ${id} from ${GO_VULN_DB}"

  mapped="$(printf '%s' "${json}" | jq -r '(.aliases // [])[] | select(startswith("GHSA-"))')"

  if [[ -z "${mapped}" ]]; then
    echo "${id}: no GHSA alias, so GitHub's advisory database cannot report it" >&2
    continue
  fi

  for ghsa in ${mapped}; do
    ghsas="${ghsas:+${ghsas}, }${ghsa}"
  done
done

printf '%s\n' "${ghsas}"
