# IBC Environment Architecture

Status: accepted target; migration in progress. The temporary acceptance path is transitional implementation state, not part of the target architecture.

## Decision

The harness has one deep `environment` module. Callers declare the IBC environment they need, `environment.Start` privately realizes that declaration, and the returned `Environment` is the sole owner of every declared resource and local handle.

Tests keep their behavioral orchestration explicit. There is no application registry, test-application resource layer, generic scenario framework, or second `Spec`/`Runtime`/`Start`/`Close` stack.

```text
environment.Spec + environment.Runtime
                    │
                    ▼
             environment.Start
            private realization
                    │
                    ▼
          resolved Environment
                    │
          ┌─────────┴─────────┐
          ▼                   ▼
 explicit test code    Environment.Close
```

## Declared resource graph

The target `environment.Spec` contains these resource families:

```go
type Spec struct {
	Chains       []ChainSpec
	IBCInstances []IBCInstanceSpec
	Connections  []ConnectionSpec
	Attestors    []AttestorSpec
	Relayers     []RelayerSpec
}
```

The graph has the following domain relationships:

```text
Chain A ── hosts ── IBC Instance A
                         │
                  IBC Connection
                  ┌──────┴──────┐
          IBC Client A     IBC Client B
                  └──────┬──────┘
                         │
Chain B ── hosts ── IBC Instance B

Attestor ── serves ── IBC Client
Relayer  ── serves ── IBC Connection
```

- A Chain hosts zero or more IBC Instances.
- An IBC Connection is the reciprocal pair of IBC Clients that point at one another. The Clients have stable authored identities because other resources may reference them.
- An Attestor is declared independently and references the Client it serves; it is not Connection configuration.
- A Relayer is declared independently and references the Connections it serves. Multiple Relayers may serve one Connection.
- A directed Route, when a Relayer needs one, is configuration of that concrete Relayer. It is not an IBC resource and never substitutes for a Connection.

Graph-addressable resources use distinct typed IDs. Declaration order has no lifecycle meaning; typed references determine dependencies.

## Declaration

`Spec` is durable desired state. It contains identities, references, and configuration, but no RPC clients, process handles, generated paths, or cleanup functions.

`Runtime` supplies process-local endpoint and authority bindings named by the Spec.

Concrete declarations express acquisition semantics when they genuinely differ. Managed and attached Chains, or new and existing protocol state, are separate variants. Creation and reuse are strict: a new declaration creates state, while an existing declaration requires an explicit locator. There is no implicit discovery or adoption.

The design does not introduce a generic resource union, builder framework, universal create/attach mode, or generic capability registry.

## Realization

`environment.Start` is the external seam for realization. Its implementation may be divided into private modules and internal adapter seams, but callers see one operation:

```go
func Start(context.Context, Spec, Runtime) (*Environment, error)
```

Behind that interface, realization:

1. snapshots and validates the declaration and runtime bindings;
2. derives dependency order from typed references;
3. selects the concrete adapters required by each declaration;
4. creates or attaches resources and verifies their identity;
5. funds the private authorities required for managed-Chain protocol mutations;
6. waits until every declared resource is ready;
7. records diagnostics and acquired effects as work succeeds; and
8. rolls back owned ephemeral effects in reverse order if startup fails.

`Start` returns a complete Environment or an error; it never returns a partial Environment or a public realization plan. Deterministic validation should happen before mutation where practical, but this is an implementation property rather than another caller-visible phase.

## Resolved Environment

`Environment` is the resolved counterpart of `Spec`. Stable authored IDs locate typed Chains, IBC Instances, Clients, Connections, Attestors, and Relayers.

Resolved resources expose stable facts and verified capabilities required by tests. Provider internals, process names, generated configuration, and cleanup hooks remain hidden.

Resolved Chains expose their RPC endpoints and may bind an IBC Link driver directly. Driver configuration uses environment-variable references whose values are supplied to the child process. The Environment's lifecycle lease remains borrowed until a one-shot command completes or a daemon is reaped; `Environment.Close` invalidates new binding use and waits for those active borrows, subject to its context.

Errors and child-process responses pass through the harness boundary unchanged.

Capabilities are derived from the concrete declaration, available authority, and adapter verification. Callers do not author capability booleans. Attachment grants connectivity, not ownership or mutation authority.

Managed Chains expose provider-independent funding as an explicit capability: `EnsureEOABalance` idempotently establishes and verifies a minimum native-token balance for an externally owned account. Zero and code-bearing addresses are rejected before mutation. The concrete adapter privately chooses the mechanism, such as a development RPC on Anvil or a transfer from an Environment-owned treasury on Besu. Attached Chains do not expose funding; callers that bring credentials for them must provision those accounts out of band.

`Environment` owns the local lifetime of every declared resource and handle. Operation handles resolve the adapter's current client when used, so a managed node restart does not stale an earlier binding. `Environment.Close` invalidates every operation handle before cleanup, releases owned ephemeral effects in reverse dependency order, closes local access to borrowed resources, preserves externally durable state, and reports cleanup failures. No other aggregate lifetime is nested beside it.

## Test code and focused operation modules

Tests explicitly sequence setup, actions, fault injection, observation, and assertions. This ordering is part of the behavior under test and should remain visible.

Focused modules may hide vertical mechanics such as ABI encoding, RPC calls, transaction submission, event decoding, polling, or process control. They should not hide horizontal orchestration across resources behind an aggregate `Run`, callback session, or outcome that silently performs the whole test.

`e2etest` is a testing adapter only. It selects reusable Specs and runtime bindings, applies test policy, calls `environment.Start`, and registers `Environment.Close` with test cleanup. It also contains the temporary application, relayer, and observation mechanics used only by these acceptance tests. It does not add another environment-shaped declaration or own another aggregate resource lifecycle.

## Test setup and the temporary acceptance path

Applications used to produce observable traffic are ordinary test setup, not Environment resources. The e2e-only `e2etest` package hides their vertical transaction, observation, and process mechanics, while tests keep deployment, orchestration, and lifecycle explicit. Temporary routes and applications must not shape `environment.Spec` or be relabeled as IBC resources.

These tests create distinct application and relayer signers. Managed Chains fund their public addresses through the resolved funding capability; attached Chains require out-of-band funding. The temporary process receives signer aliases and protected local key files, while raw keys and provider-default accounts stay out of declarations, configuration, and test code.

This lane remains until actual IBC Instances, Connections, and a truthful Relayer replace its traffic coverage. Its eventual deletion must not require changing the Environment interface.

## Assessment and test selection

`environment.Requirements` names workflow-visible capabilities on stable authored resource IDs. Requirements are separate from `Spec`: they describe what a test needs from a selected Environment, not properties the declaration may grant to its resources.

`environment.Assess(Spec, Runtime, Requirements)` is side-effect-free. It validates the complete selected declaration, runtime bindings, and requirement targets without filesystem, process, Docker, RPC, or network work. An invalid selection returns an error; capabilities the selection cannot guarantee produce a non-feasible `Assessment`.

`e2etest.RequireCapabilities` owns testing policy at the test seam. It fails an invalid selection and skips a non-feasible interchangeable selection before acquisition. `environment.Start` remains solely responsible for realization and acquisition: it does not accept Requirements, decide whether to skip a test, or turn startup failures into unsupported selections.

## Deliberately excluded

- application or scenario fields, registries, resource families, or lifecycle aggregates in `environment.Spec`;
- a callback session or second aggregate runtime;
- capability booleans authored in Specs;
- implicit resource discovery, adoption, or ensure semantics;
- global provider-default test identities or faucet accounts;
- treating Relayer Routes as IBC Connections; and
- compatibility aliases for the superseded harness shape.

## Current implementation gap

The repository realizes Chains, IBC Instances, Connections, and Attestors. It does not yet declare or realize the truthful protocol Relayer described by this design.

Implementation should proceed by defining the concrete `RelayerSpec`, resolved Relayer interface, readiness/status semantics, and process adapter together from the product Relayer port. Once real protocol traffic covers the same behavior, the temporary acceptance machinery in `e2etest` can be deleted without changing the Environment interface.

## Open design work

- Define concrete `RelayerSpec` variants and their dependency references from the product Relayer port.
- Define Relayer readiness, status, manual relay, lifecycle, signer, and finality semantics.
- Decide when the temporary acceptance lane has enough truthful replacement coverage to be deleted.
- Shape any further test-application bindings and attached-Chain effect reporting from concrete tests.
- Define diagnostic artifact schema and retention only when consumers require them.
- Model finality separately from timing when a real workflow requires it.
- Add process-placement controls only when a concrete standalone versus co-located case requires them.
