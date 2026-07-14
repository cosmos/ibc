#!/usr/bin/env bash
set -euo pipefail

dry_run=0

while [ "$#" -gt 0 ]; do
	case "$1" in
		--dry-run) dry_run=1 ;;
		-h|--help)
			printf 'Usage: %s [--dry-run]\n\n' "$0"
			printf 'Stops leaked e2e relayer daemons and removes e2e Docker resources by label or ibc-link-e2e- name prefix.\n'
			printf 'Daemons are matched by the harness config marker (ibc-link.config.yaml in argv), so a\n'
			printf 'developer'"'"'s own unrelated "ibc relayer run" is left alone.\n'
			exit 0
			;;
		*)
			printf 'clean-e2e: unknown argument %s\n' "$1" >&2
			exit 2
			;;
	esac
	shift
done

note() { printf 'clean-e2e: %s\n' "$*"; }

sweep() {
	local label="$1" pattern="$2" pids
	pids="$(pgrep -f "$pattern" || true)"
	if [ -z "$pids" ]; then
		note "no $label found"
		return
	fi
	note "stop $label:"
	# shellcheck disable=SC2086
	ps -o pid=,command= -p $pids || true
	if [ "$dry_run" -eq 1 ]; then
		return
	fi
	# shellcheck disable=SC2086
	kill $pids 2>/dev/null || true
	sleep 1
	# SIGKILL the original capture only — do not re-pgrep.
	# shellcheck disable=SC2086
	kill -9 $pids 2>/dev/null || true
}

# Match either SUT binary (real `ibc` or the e2e `ibc-stub`) running `relayer run`, scoped to the
# harness's compiled config (ibc-link.config.yaml, always in --config of a harness-spawned daemon) so a
# developer's own unrelated `ibc relayer run` is never signaled. Binary is anchored at a path separator
# or start of the cmdline so an unrelated `…ibc` suffix can't false-match.
sweep "e2e relayer daemons" '(^|/)ibc(-stub)? relayer run .*ibc-link\.config\.yaml'

docker_sweep() {
	if ! command -v docker >/dev/null; then
		note "docker not found; skip Docker resources"
		return
	fi
	if ! docker info >/dev/null 2>&1; then
		note "docker not reachable; skip Docker resources"
		return
	fi

	local containers networks
	containers="$(
		{
			docker ps -aq --filter label=ibc-link-e2e=true
			docker ps -aq --filter name='^/ibc-link-e2e-'
		} 2>/dev/null | sort -u
	)"
	if [ -z "$containers" ]; then
		note "no e2e Docker containers found"
	else
		note "remove e2e Docker containers:"
		for id in $containers; do
			docker ps -a --format '  {{.ID}} {{.Names}} {{.Status}}' --filter "id=$id" || true
		done
		if [ "$dry_run" -eq 0 ]; then
			# shellcheck disable=SC2086
			docker rm -f $containers >/dev/null 2>&1 || true
		fi
	fi

	networks="$(
		{
			docker network ls -q --filter label=ibc-link-e2e=true
			docker network ls -q --filter name='^ibc-link-e2e-'
		} 2>/dev/null | sort -u
	)"
	if [ -z "$networks" ]; then
		note "no e2e Docker networks found"
	else
		note "remove e2e Docker networks:"
		for id in $networks; do
			docker network ls --format '  {{.ID}} {{.Name}}' --filter "id=$id" || true
		done
		if [ "$dry_run" -eq 0 ]; then
			# shellcheck disable=SC2086
			docker network rm $networks >/dev/null 2>&1 || true
		fi
	fi
}

docker_sweep

note "done"
