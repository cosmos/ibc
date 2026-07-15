# Remove the Nested Internal Tree from the E2E Harness

Status: ready-for-agent

## Problem Statement

The repository-internal E2E harness is already contained by the E2E module's Go `internal` boundary, but the harness contains a second `internal` package tree. That nested tree divides related EVM providers across separate hierarchies, hides package ownership behind a generic visibility mechanism, and gives small implementation helpers the appearance of harness-wide abstractions.

The second boundary does prevent the parent E2E module from importing those packages directly, but that restriction is no longer desired. Removing it mechanically would still produce a poor structure: packages such as process supervision and attached-chain connection are too shallow to justify independent boundaries, while Solidity IBC realization is too substantial to merge into a broad package. Each package must move to the domain that owns its behavior rather than simply moving one directory upward.

## Solution

Remove the harness's nested `internal` tree and relocate its contents according to domain ownership. EVM implementations and their shared managed-container and polling mechanics will live under the EVM chain domain. Link attestor launching and shared subprocess supervision will live in the IBC Link adapter. Solidity IBC protocol realization, including its generated binding and contract-generation assets, will live in a focused child of the Environment module.

Besu will become a peer of Anvil. Attached EVM support will merge into the existing EVM package because it only verifies and composes existing EVM primitives. The attestor launcher will merge into the IBC Link package, while preserving the distinction between the durable `environment.AttestorSpec` declaration and the transient launch inputs assembled during realization. Process supervision will become an unexported IBC Link implementation detail.

This is a structural refactor. Runtime behavior, resource ownership, public harness behavior, protocol semantics, and test assertions will remain unchanged except for names required by the two package merges.

## User Stories

1. As a harness contributor, I want the package tree to express domain ownership, so that I can locate behavior without interpreting redundant visibility boundaries.
2. As a harness contributor, I want the outer E2E `internal` boundary to be the only Go visibility boundary around the harness, so that the filesystem does not imply two independent encapsulation requirements.
3. As an E2E contributor, I want harness implementation packages to be technically importable from the E2E module, so that Go does not enforce a boundary the project no longer wants.
4. As a reviewer, I want every relocated package to have an explicit owner, so that the refactor does not create a flat collection of unrelated harness utilities.
5. As a chain-adapter contributor, I want Besu and Anvil represented as peer EVM providers, so that equivalent implementations are discoverable in one domain hierarchy.
6. As a chain-adapter contributor, I want attached EVM support colocated with the EVM primitives it composes, so that a shallow provider package does not obscure its limited responsibility.
7. As an Environment maintainer, I want attached-chain ownership semantics to remain unchanged, so that closing an Environment continues to close local access without stopping a borrowed chain.
8. As a managed-chain contributor, I want Anvil and Besu to share one scoped container-policy package, so that cleanup labels, safe names, and loopback port binding remain consistent.
9. As a security-conscious contributor, I want managed-container port binding policy preserved during relocation, so that provider ports do not accidentally become externally exposed.
10. As an EVM contributor, I want polling mechanics scoped to EVM behavior, so that a small retry loop does not become an unowned harness-wide utility.
11. As an EVM client user, I want ordinary client code to remain separate from managed-container implementation types, so that the main EVM package does not absorb Moby-specific responsibilities.
12. As an IBC Link contributor, I want daemon and attestor process launchers owned by the same adapter, so that executable configuration, readiness, and lifecycle behavior have one clear home.
13. As an Environment contributor, I want `environment.AttestorSpec` to remain the durable domain declaration, so that authored resource graphs contain identities and references rather than process-local details.
14. As an IBC Link adapter contributor, I want the attestor launch input to have a name that describes its transient purpose, so that it is not confused with the Environment's domain spec.
15. As an Environment contributor, I want runtime bindings and realized dependencies converted into attestor launch inputs at the realization boundary, so that the Environment retains responsibility for resolving the declared graph.
16. As an IBC Link contributor, I want subprocess-group supervision shared privately by the daemon and attestor launchers, so that process cleanup behavior is reused without exposing an accidental generic API.
17. As a protocol-realization contributor, I want Solidity IBC deployment and attachment mechanics kept cohesive, so that their transaction, verification, and generated-binding behavior does not spread through the Environment package.
18. As an Environment maintainer, I want Solidity IBC realization visibly owned by the Environment module, so that its role in privately realizing declared protocol resources remains clear.
19. As a contract-binding contributor, I want the AccessManager binding and Solidity generation project moved with their owning adapter, so that regeneration continues to operate on one cohesive unit.
20. As a contributor, I want generation scripts and repository targets to reference the new Solidity IBC location, so that bindings can still be rebuilt and checked for staleness.
21. As a maintainer, I want harness guidance to describe the new package ownership and polling location, so that future changes do not recreate the removed hierarchy.
22. As a reviewer, I want the nested `internal` directory removed once empty, so that no obsolete structure remains after the migration.
23. As a test author, I want the Environment's declaration, realization, ownership, and cleanup behavior preserved, so that package movement does not change the harness contract.
24. As a test runner, I want existing harness and E2E commands to keep working, so that the structural refactor does not require a new verification workflow.
25. As a maintainer, I want failures after the move treated as relocation regressions, so that this work does not silently expand into product or harness redesign.

## Implementation Decisions

- The harness module remains repository-internal through the E2E module's existing Go `internal` boundary. The nested harness `internal` tree will be removed, and no replacement visibility mechanism will be introduced.
- Package placement follows domain ownership rather than current importer visibility. The E2E module may technically import the relocated packages, while the intended caller surface remains an architectural convention documented by the harness.
- Besu will move under `chain/evm/besu` as a peer of Anvil. Its provider behavior, interfaces, managed-chain ownership, funding capability, diagnostics, and tests will remain unchanged.
- The current external EVM package will disappear as a package boundary. Its chain-ID verification and composition behavior will merge into `chain/evm` as attached-chain support, using names that distinguish the attached-chain adapter from general EVM primitives.
- The attached-chain surface will use the concepts `AttachedSpec`, `AttachedChain`, and `ConnectAttached`. These names express acquisition semantics without preserving an otherwise shallow provider namespace.
- Shared Anvil and Besu container policy will move to `chain/evm/container`. It will continue to own managed-container labels, safe name construction, and loopback port binding, while Moby container types remain outside the main EVM package.
- EVM transaction and provider readiness polling will move to `chain/evm/poll`. It will remain a focused child package because all current consumers poll EVM behavior and provider subpackages need to reuse it.
- The Link attestor launcher will merge into `ibclink`. It will continue to create protected configuration and key material, run the public `ibc attestor run` command, probe the public readiness endpoint, retain diagnostic logs, and stop the owned process group.
- `environment.AttestorSpec` remains the durable domain declaration containing the Attestor identity and references to the Client and Authority it uses. It will not acquire binary paths, generated directories, private keys, endpoints, handles, or cleanup functions.
- The concrete adapter input will be named `ibclink.AttestorLaunch`. It is a transient value assembled during realization from the authored Attestor declaration, runtime authority binding, observed Chain identity, resolved IBC Link binary, and Environment-owned workspace.
- Environment realization remains responsible for resolving the Attestor's dependency graph and assembling `ibclink.AttestorLaunch`. The IBC Link adapter remains responsible for interpreting those launch inputs as files, command arguments, readiness checks, diagnostics, and process lifecycle.
- The generic process package will disappear. Its process-group reaping, graceful signaling, escalation, and wait behavior will become an unexported IBC Link implementation detail shared by daemon and attestor launchers, with its focused tests retained.
- Solidity IBC realization will move to `environment/solidityibc` as a focused child package. It will retain deployment, attachment, client preparation, on-chain verification, transaction submission, and mining behavior without expanding the Environment's public API.
- The generated AccessManager binding and the Solidity contract-generation project will remain isolated within the Solidity IBC adapter and move with it. Repository generation scripts and stale-binding checks will be updated to the new ownership location.
- Imports, package declarations, aliases, test package references, repository targets, generation scripts, and harness contributor guidance will be updated consistently. The migration is incomplete while any source or automation reference still points to the removed nested tree.
- Only the attached EVM and process-supervision package boundaries will disappear. Besu, container policy, EVM polling, and Solidity IBC realization retain focused package boundaries at their new domain-owned locations.
- The refactor will preserve behavior. It will not introduce compatibility aliases for old internal import paths, redesign interfaces, alter resource lifetimes, change readiness or timing policy, or modify assertions except where symbol renaming mechanically requires updates.

## Testing Decisions

- The highest behavioral seam is the existing `environment.Start` path exercised by harness tests. It validates that declarations and runtime bindings still realize Chains, IBC Instances, Connections, and Attestors through the relocated adapters and that Environment cleanup preserves owned-versus-borrowed resource semantics.
- The existing harness test target is the primary regression command. It compiles every harness package and runs focused unit tests plus Docker-backed integration tests when Docker is available, providing coverage across the moved package boundaries without adding a new test seam.
- Good tests assert stable behavior rather than directory structure. Provider startup, chain identity verification, funding, readiness, process cleanup, protocol deployment, attachment, and Environment ownership remain the contracts; tests should not be rewritten merely to encode new import paths.
- Besu and Anvil retain their provider-focused tests. These tests are prior art for configuration generation, managed-node startup, readiness, diagnostics, and lifecycle behavior under interchangeable EVM declarations.
- Attached EVM behavior remains covered at the Environment seam, including chain-ID verification and the rule that Environment closure does not stop a borrowed Chain. A new package-level abstraction test is unnecessary after the shallow external package is removed.
- Shared container-policy tests move with the container package and continue to verify safe managed-resource names and loopback-only dynamic port binding.
- EVM polling remains exercised through existing client and provider behavior. Tests should prefer transaction and readiness outcomes over tests coupled to retry-loop implementation details.
- Attestor launcher tests move into the IBC Link package and continue to verify protected workspace creation, public-endpoint readiness, early-exit diagnostics, invalid-input rejection, and process cleanup.
- Process-supervision tests remain focused on observable lifecycle guarantees: cancellation and forced termination must still reap the subprocess group, and graceful-stop escalation must retain its current semantics. The helper's new unexported status does not justify weakening these tests.
- Solidity IBC tests move with the adapter and continue to cover deployment and attachment, configuration validation before side effects, broadcast failure reporting, cancellation while awaiting mining, and generated AccessManager interaction.
- The E2E module's existing pure-Go unit target must continue to pass, proving that selection and helper packages compile against the relocated harness surface without starting Chains.
- The root E2E package must be compiled against the relocated harness imports without requiring a full acceptance run. The service-free compile seam is `go -C e2e test . -run '^$' -count=1`. Full acceptance behavior is unchanged and remains available through the existing E2E target when its ordinary binary and Chain prerequisites are present.
- The existing E2E lint target must pass for both the E2E and harness modules. This catches stale imports, package naming problems, generated-code placement issues, and violations of the harness's current lint policy.
- Contract generation and stale-binding verification must use the relocated Solidity IBC project and binding output. A clean regeneration must not produce an unexplained diff.
- Static verification must confirm that no Go import, shell script, Make target, or harness guidance references the removed nested `internal` tree and that the directory no longer exists.
- No new testing framework or seam will be introduced. The prior art is the current focused package tests beneath the single deep Environment realization seam.

## Out of Scope

- Changing Link, Attestor, EVM provider, Solidity IBC, or protocol behavior.
- Redesigning `environment.Spec`, `environment.Runtime`, `environment.Start`, the resolved `Environment`, or their resource ownership model.
- Changing the domain relationship in which an Attestor serves one authored IBC Client and observes the counterparty IBC Instance derived through that Client's Connection.
- Combining the durable `environment.AttestorSpec` declaration with the transient `ibclink.AttestorLaunch` adapter input.
- Making the relocated implementation packages part of a promised public API merely because the E2E module can technically import them.
- Adding lint rules or another mechanism to recreate the visibility restriction previously supplied by the nested Go `internal` directory.
- Moving the harness out of the E2E module's outer `internal` boundary or merging the harness and E2E Go modules.
- Redesigning package APIs beyond the names required when attached EVM support and process supervision lose their package boundaries.
- Changing polling intervals, readiness budgets, process stop grace periods, Docker images, container labels, port exposure policy, or diagnostic retention.
- Regenerating unrelated bindings, changing Solidity contracts, or modifying generated code beyond relocation and path updates.
- Adding acceptance scenarios or changing existing test expectations.
- Publishing an issue or changing issue-tracker state; this specification is intentionally a local Markdown artifact.

## Further Notes

The outer and inner Go `internal` directories enforced different boundaries. The outer boundary keeps the harness repository-internal to the E2E module's import namespace. The removed inner boundary additionally prevented the parent E2E module from importing harness implementation packages. This specification intentionally removes only the second restriction.

The package map avoids both extremes: it does not preserve every current package mechanically, and it does not flatten every implementation into the harness root. Focused vertical mechanics remain separate where they carry substantial behavior or isolate dependencies, while shallow namespaces disappear when their owner package already provides the correct conceptual home.
