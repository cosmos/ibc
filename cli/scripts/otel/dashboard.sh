#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

action=${1:-}
overwrite=${2:-}
if [[ "$action" != export && "$action" != import ]] ||
    [[ -n "$overwrite" && ( "$action" != import || "$overwrite" != --overwrite ) ]] ||
    (( $# > 2 )); then
    printf 'Usage: bash dashboard.sh export | import [--overwrite]\n' >&2
    exit 1
fi

file=${DASHBOARD_FILE:-$(dirname "$0")/dashboard-loadtest.json}
namespace=${GRAFANA_NAMESPACE:-default}
url=${GRAFANA_URL:-http://127.0.0.1:3001}
curl_args=(--silent --show-error --connect-timeout 5 --max-time 30)

jq -e '.apiVersion == "dashboard.grafana.app/v2" and .kind == "Dashboard"
    and (.spec | type == "object")
    and (.metadata.name | type == "string" and test("^[a-zA-Z0-9_-]+$"))' "$file" > /dev/null
name=$(jq -r '.metadata.name' "$file")
endpoint="${url%/}/apis/dashboard.grafana.app/v2/namespaces/$namespace/dashboards"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

status=$(curl "${curl_args[@]}" --output "$tmp/live.json" --write-out '%{http_code}' "$endpoint/$name")
if [[ "$status" != 200 && "$status" != 404 ]]; then
    printf 'Grafana dashboard lookup failed (HTTP %s).\n' "$status" >&2
    jq -r '.message // .' "$tmp/live.json" >&2
    exit 1
fi

if [[ "$action" == export ]]; then
    if [[ "$status" != 200 ]]; then
        printf 'Dashboard %s does not exist in Grafana.\n' "$name" >&2
        exit 1
    fi
    jq -e '{apiVersion, kind, metadata: {name: .metadata.name}, spec}
        | if .apiVersion == "dashboard.grafana.app/v2" and .kind == "Dashboard"
            and (.spec | type == "object") then . else error("Invalid dashboard response") end' \
        "$tmp/live.json" > "$tmp/snapshot.json"
    mv "$tmp/snapshot.json" "$file"
    printf 'Exported %s to %s.\n' "$name" "$file"
    exit 0
fi

if [[ "$status" == 200 && "$overwrite" != --overwrite ]]; then
    printf 'Dashboard %s already exists; refusing to overwrite local edits.\n' "$name" >&2
    printf 'Use dashboard-import OVERWRITE=1 to replace it explicitly.\n' >&2
    exit 1
fi

jq --arg namespace "$namespace" \
    '{apiVersion, kind, metadata: {name: .metadata.name, namespace: $namespace}, spec}' \
    "$file" > "$tmp/snapshot.json"
method=POST
if [[ "$status" == 200 ]]; then
    # Preserve target-instance metadata, including the version used for optimistic concurrency.
    jq --slurpfile live "$tmp/live.json" '.metadata = $live[0].metadata' \
        "$tmp/snapshot.json" > "$tmp/update.json"
    mv "$tmp/update.json" "$tmp/snapshot.json"
    method=PUT
    endpoint="$endpoint/$name"
fi

curl "${curl_args[@]}" --fail-with-body --request "$method" \
    --header 'Content-Type: application/json' --data-binary "@$tmp/snapshot.json" \
    "$endpoint" > /dev/null
printf 'Imported %s into %s.\n' "$name" "$url"
