<!-- SPDX-License-Identifier: Apache-2.0 -->

# ADR 0001: Model E2E environments as owned resource graphs

- Status: Proposed
- Date: 2026-08-05

## Context

Repository acceptance tests need real chains, protocol contracts, clients, connections, attestors, and Link processes. These resources have dependencies and different ownership: the harness may create and destroy a managed chain, but it must not take ownership of a chain supplied by the caller.

Without an explicit boundary, provisioning details leak into tests, cleanup becomes ambiguous, and connectivity can be mistaken for permission to control an external resource.

## Decision

Tests describe the desired environment as a typed resource graph before starting it. The harness validates and realizes that graph in dependency order.

The harness owns the lifecycle only of resources declared as managed. Attached resources remain caller-owned, even when the harness can communicate with them.

Generic chain access contains only behavior common across chain families. Family-specific operations remain family-specific, and control capabilities are exposed only when the selected resource declaration and adapter guarantee them.

Traffic workflows bind applications and routes above the environment and exercise Link as an external process. Endpoint timing governs direct endpoint operations; waits spanning a route belong to the route rather than either endpoint alone.

## Consequences

- Resource ownership and cleanup are explicit and testable.
- Tests may spell out complete graphs when their topology is unusual; this is intentional.
- Attached resources cannot accidentally gain lifecycle or mining control from connectivity alone.
- Supporting another chain family requires its own declarations, realization, and family-specific access rather than additions to a universal chain interface.
