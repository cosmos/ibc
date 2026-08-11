<!-- SPDX-License-Identifier: Apache-2.0 -->

# ADR 0002: Select E2E providers from test requirements

- Status: Proposed
- Date: 2026-08-05

## Context

Provider-named test lanes mix three concerns: what a test requires, which providers are available, and whether a run prioritizes speed or production similarity. Tests then branch on global lane names, capable tests are skipped unnecessarily, and it is difficult to see which provider actually covers each test.

We want fast local feedback, complete pull-request coverage, and production-like scheduled coverage without encoding provider policy in individual tests. This decision builds on [ADR 0001](./0001-model-e2e-environments-as-owned-resource-graphs.md).

## Decision

Each test declares requirements for each chain family it uses. Requirements describe needed behavior; a test names an exact provider only when that provider's behavior is itself under test.

An execution mode applies provider policy:

- Fast selects the fastest compatible providers and may skip tests that cannot run.
- Complete requires every test to resolve, choosing the fastest compatible providers.
- Production requires every test to resolve, preferring production-like compatible providers.

Each test runs once per mode. Requirements filter acceptable providers; the mode chooses among them. A requirement may force a less production-like provider when that provider is the only one offering the required control.

Selection produces the concrete environment graph consumed by the harness. That resolved graph is the source of truth for both execution and a deterministic coverage matrix. Additional production-like providers are added through provider policy or curated CI runs, not through test branches or a universal fidelity ranking.

## Consequences

- Tests no longer know about lanes or select providers procedurally.
- Local and pull-request runs remain fast without weakening the requirement that complete modes cover every test.
- Scheduled runs exercise production-like providers wherever test requirements allow.
- Adding a provider changes selection policy and coverage configuration; portable tests do not change.
