# Flatten the E2E Acceptance Tests

Status: ready-for-agent

## Problem Statement

The repository-level end-to-end acceptance tests are divided across `setup`, `ibclink`, `external`, and `negative` packages even though they exercise one black-box IBC Link acceptance surface. These package boundaries make the suite harder to browse, force developers to understand an artificial taxonomy before finding a scenario, and encode classifications such as “happy” and “negative” that do not describe the behavior under test.

The split also leaks into the test runner. The default command selects several packages while excluding the negative package, and CI has to supply a different package list to obtain full coverage. Short tests are scattered across narrowly named files, while test names such as `Fault`, `SelectedSuite`, and `StatusIsBetterSignal` emphasize categories or implementation context instead of observable behavior.

## Solution

Move all repository-level acceptance scenarios into one root external test package. Organize that package around protocol domains where scenarios form a coherent group, and around a specific operational behavior where combining tests would create an overly broad file.

Consolidate IFT scenarios, GMP scenarios, configuration scenarios, and test-application deployment scenarios into domain-oriented files. Keep attached-chain ownership, Relayer lifecycle, pending-packet reporting, cross-route behavior, node recovery, and invalid manual relay behavior in separate behavior-oriented files. The resulting root package will contain ten test files covering those concerns.

Rename tests whose current names encode the removed package taxonomy or obscure their assertion. Names must describe externally observable behavior, while established and useful protocol distinctions such as IFT, GMP, automatic relay, and manual relay remain visible. Related scenarios remain separate top-level Go tests so they retain independent setup, capability checks, filtering, and failure reporting.

The default end-to-end command will run the complete root acceptance package, including the scenarios formerly classified as negative. Focused execution will use Go test flags and test names instead of package selection.

## User Stories

1. As a contributor, I want all black-box acceptance scenarios in one root package, so that I can understand the product acceptance surface without traversing an artificial package hierarchy.
2. As a contributor, I want support libraries to remain separate from acceptance scenarios, so that reusable harness code and executable product behavior keep distinct responsibilities.
3. As a contributor, I want IFT scenarios grouped by domain, so that automatic relay, manual relay, and timeout/refund behavior are discoverable together.
4. As a contributor, I want GMP scenarios grouped by domain, so that automatic relay, manual relay, and error-acknowledgement behavior are discoverable together.
5. As a contributor, I want configuration validation scenarios and their private helpers colocated, so that the helpers' purpose and ownership remain obvious.
6. As a contributor, I want test-application deployment verification grouped under its own domain, so that deployment acceptance behavior is not confused with configuration or relay behavior.
7. As a contributor, I want attached-chain ownership behavior to remain a distinct concern, so that the Environment's responsibility for borrowed resources is explicit.
8. As a contributor, I want Relayer restart behavior grouped as lifecycle behavior, so that recovery across process restarts is easy to find.
9. As a contributor, I want pending-packet reporting to remain a distinct behavior, so that status semantics under paused destination mining are not hidden in a generic Relayer file.
10. As a contributor, I want cross-route sequence handling to remain a distinct behavior, so that collision prevention across routes is easy to identify.
11. As a contributor, I want destination-node restart recovery to remain a distinct behavior, so that infrastructure fault recovery is visible without a vague “negative” classification.
12. As a contributor, I want invalid manual relay requests to remain a distinct behavior, so that request rejection is named by its contract rather than by a broad failure category.
13. As a test runner, I want every acceptance scenario included in the default end-to-end command, so that local and CI coverage do not diverge through different package lists.
14. As a contributor, I want focused runs to select stable test names, so that I can iterate on one scenario after package-based selection is removed.
15. As a contributor, I want each scenario to remain a separate top-level test, so that setup, capability requirements, filtering, and failures stay independent.
16. As a reviewer, I want test names to state observable behavior, so that test output explains the contract being verified without requiring me to inspect the implementation.
17. As a maintainer, I want obsolete package-selection documentation removed, so that examples describe commands that continue to work after the package flattening.
18. As a maintainer, I want the former suite directories removed once empty, so that the filesystem reflects the new single-package model without dead structure.
19. As a CI maintainer, I want the workflow to use the same end-to-end entry point as developers, so that failures reproduce locally with the documented command.
20. As a contributor, I want lane selection to continue working unchanged, so that Anvil, interval-mining Anvil, and Besu remain available without coupling lanes to file organization.
21. As a contributor, I want capability-based skips to continue operating per test, so that scenarios requiring mining or node-lifecycle controls remain valid across interchangeable lane selections.
22. As a maintainer, I want unit and harness test commands to remain independent of the root acceptance package, so that fast support-code checks do not unexpectedly start chains.

## Implementation Decisions

- The repository-level acceptance surface will become one external Go test package at the root of the E2E module. The external package form preserves black-box usage and does not introduce production Go code at the module root.
- The four existing acceptance packages—setup, IBC Link flow, external chain, and negative flow—will cease to be package boundaries. Their empty directories will be removed after migration.
- The reusable selection/start package and all internal harness, observation, synthetic-traffic, and test-application packages will retain their current boundaries. Their unit tests are not part of this move.
- The root package will use ten concern boundaries: IFT, GMP, configuration, test-application deployment, attached-chain ownership, Relayer lifecycle, pending-packet status, cross-route handling, node recovery, and invalid manual relay requests.
- IFT automatic relay, manual relay, and timeout/refund scenarios will share one domain file. Their test functions will remain independent.
- GMP automatic relay, manual relay, and error-acknowledgement scenarios will share one domain file. Their test functions will remain independent.
- Configuration validation and its test-only helper functions will share one domain file. Test-application on-chain deployment verification will remain a separate domain.
- Operational scenarios will remain behavior-oriented rather than being folded into a generic Relayer file. This prevents unrelated lifecycle, status, route, node, and invalid-request tests from accumulating behind a broad filename.
- All moved files will use the same root package declaration. Existing imports of internal E2E packages remain valid because the root package is inside the same module's internal visibility boundary.
- Package-level declarations will be merged into one namespace. The current declarations do not collide, so the migration does not require new abstractions or helper packages.
- Test functions will be renamed only when their current names contain obsolete classifications, redundant environment-selection details, or unclear assertions. Renamed tests will use observable formulations such as configuration validation, attached resources remaining caller-owned, Relayer recovery after node restart, and rejection of an unknown source transaction.
- Established protocol vocabulary and meaningful execution modes will remain in test names. IFT, GMP, automatic relay, manual relay, timeout, refund, pending packets, and error acknowledgements are part of the domain language rather than organizational categories.
- The package-selection variable will be removed from the E2E command interface. The end-to-end target will always test the root package, and ordinary Go test flags will remain the supported customization mechanism.
- The former negative scenarios will join the default run. They specify required recovery, refund, acknowledgement, and validation behavior and are therefore ordinary acceptance criteria.
- CI will invoke the same default end-to-end target without supplying a package override.
- Contributor guidance and the E2E overview will describe one acceptance package, behavior-oriented coverage, complete default execution, lane selection, and focused runs by test name.
- Lane configuration and explicit capability assessment will not change. Tests that require mining control or node lifecycle control will continue to declare those requirements before acquiring an Environment.

## Testing Decisions

- The primary verification seam is the existing repository end-to-end target. It is the highest practical seam because it builds the ordinary IBC Link binary, starts real selected chain environments, drives public CLI and transport contracts, and observes application and packet outcomes.
- A complete verification run must execute the single root acceptance package and include every migrated scenario, including node recovery, timeout/refund, error acknowledgement, and invalid relay requests.
- Focused verification will use the existing test-flags input with Go's `-run` filtering. Package paths will no longer select setup, flow, external, or negative categories.
- Good acceptance tests assert externally observable behavior through the Environment, public IBC Link command transport, route-bound test-application bindings, Relayer status, and chain capabilities. They do not invoke Link handlers in process or import Link internal packages.
- Each scenario will continue to start a fresh Environment. This preserves resource isolation and prevents consolidation into a shared file from introducing shared runtime state.
- Related scenarios will remain separate top-level tests rather than subtests under broad domain umbrellas. This preserves precise filtering, standalone capability checks, and direct failure names.
- Existing per-test capability assessment remains the seam for lane-dependent behavior. A valid but insufficient lane may skip before acquisition, while invalid selection or startup failure must still fail.
- The existing unit-test target remains the verification seam for E2E selection and helper packages and must not begin running the root acceptance package.
- The existing harness-test target remains the verification seam for harness unit and integration behavior. No harness tests need to move or be rewritten for this structural change.
- Static verification must confirm that contributor documentation, automation, and workflow configuration contain no remaining package-selection examples or references to the removed suite directories.
- The prior art is the current linear acceptance-test style: explicit Environment startup, explicit signer and route setup, public driver or Relayer interaction, and assertions through route-bound application bindings.
- The migration should not change scenario behavior. Any failure after the move should be treated as a packaging, naming, command-wiring, or accidental test-content regression rather than as an opportunity to redesign the harness.

## Out of Scope

- Redesigning the Environment, Suite, runtime binding, chain, Relayer, synthetic traffic, observation, or test-application APIs.
- Moving or consolidating unit tests in the reusable E2E support packages or the nested harness module.
- Changing the Anvil, interval-mining Anvil, or Besu lane definitions.
- Introducing build tags, environment-variable gates, or runtime skips to recreate the former negative-suite opt-in behavior.
- Converting independent scenarios into table-driven tests or subtests solely because their source files are consolidated.
- Changing product behavior, protocol semantics, timeout values, capability requirements, or test assertions except where a mechanical move requires a package-name or symbol-name update.
- Adding new acceptance scenarios.
- Publishing an issue or changing issue-tracker state; this specification is intentionally a local Markdown artifact.

## Further Notes

The word “negative” should disappear as an organizational concept, but the underlying scenarios remain important acceptance criteria. A timeout refund or recovery after a node restart describes required system behavior just as directly as successful automatic relay.

The target file set is intentionally mixed: broad protocol domains are consolidated where the files are short and cohesive, while operational behaviors retain narrower names when a single component label would hide several unrelated contracts. This keeps the root package compact without replacing the old directory taxonomy with an oversized catch-all file.
