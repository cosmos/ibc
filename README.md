# Interchain Standards Development

This repository is for standards development of Interchain Standards. Initially it will be used to solicit and integrate feedback on IBC (Inter-blockchain Communication) Protocol specification development. We propose usage of an adaptation of the [TC 39](https://tc39.github.io/process-document/) process used by the ECMAScript steering committee.

## Cosmos Interchain Specification Proposal Process

#### Stage 0 - `Strawman`
- _**Purpose**_: Start the specification process
- _**Entrance Criteria**_: [Open an issue](https://github.com/cosmos/ics/issues/new) on this repository with a short outline of your proposal
- _**Acceptance Signifies**_: N/A
- _**Spec Quality**_: N/A
- _**Changes Expected Post-Acceptance**_: N/A
- _**Implementation Types Expected**_: N/A

#### Stage 1 - `Proposal`
- _**Purpose**_:
  * Make the case for the addition of this specification to the Cosmos ecosystem
  * Describe the shape of the a potential solution
  * Identify challenges to this proposal
- _**Entrance Criteria**_:
  * Prose outlining the problem or need and the general shape of a solution in a PR to a `./spec/{{ .Spec.Number }}-{{ .Spec.Name }}/proposal.md` file in this repo. This file should contain:
    1. Illustrative examples of usage
    1. High-level API
    1. Discussion of key algorithms, abstractions and semantics
    1. Identification of potential “cross-cutting” concerns and implementation challenges/complexity
  * Identified `Champion(s)` who will advance the proposal
- _**Acceptance Signifies**_:
  * The PR has received 2 :+1:s from members of the specification team.
- _**Spec Quality**_:
  * None, this is just a proposal
- _**Changes Expected Post-Acceptance**_:
  * Major, the entire shape of the solution may change. Proposal documents are not to be relied upon for implementation.
- _**Implementation Types Expected**_:
  * Tightly bounded demos, example repos showing reproduction steps for issues fixed by proposal

#### Stage 2 - `Draft`
- _**Purpose**_:
  * Precisely describe the syntax and semantics using formal spec language
- _**Entrance Criteria**_:
  * Everything from stage 1
  * Initial specification text in a PR to add a `./spec/{{ .Spec.Number }}-{{ .Spec.Name }}/spec.md` file
- _**Acceptance Signifies**_:
  * The specification team expects that this proposal will be finalized and eventually included in the standard
- _**Spec Quality**_:
  * Draft: all major semantics, syntax and API are covered, but TODOs, placeholders and editorial issues are expected
- _**Changes Expected Post-Acceptance**_:
  * Incremental changes expected after spec enters draft stage. Implementors should work with the spec champions as work continues on spec development.
- _**Implementation Types Expected**_:
  * Experimental

#### Stage 3 - `Candidate`
- _**Purpose**_:
  * Indicate that further refinement will require feedback from implementations and users
- _**Entrance Criteria**_:
  * Everything from stages 1,2
  * Complete specification text
- _**Acceptance Signifies**_:
  * The solution is complete and no further work is possible without implementation experience, significant usage and external feedback.
- _**Spec Quality**_:
  * Complete: all semantics, syntax and API are completed described
- _**Changes Expected Post-Acceptance**_:
  * Limited: only those deemed critical based on implementation experience
- _**Implementation Types Expected**_:
  * Spec compliant

#### Stage 4 - `Finished`
- _**Purpose**_:
  * Indicate that the addition is included in the formal ICS system
- _**Entrance Criteria**_:
  * Everything from stages 1,2,3
  * Acceptance tests are written and merged into the Cosmos-SDK
  * At least one spec compatible implementation exists
  * All files in the `./spec/{{ .Spec.Number }}-{{ .Spec.Name}}/` directory are up to date and merged into the `cosmos/ics` repo
- _**Acceptance Signifies**_:
  * The addition is now part of the ICS standards
- _**Spec Quality**_:
  * Final: All changes as a result of implementation experience are integrated
- _**Changes Expected Post-Acceptance**_:
  * None
- _**Implementation Types Expected**_:
  * Shipping/Production

### Calls for implementation and feedback

When an addition is accepted at the “candidate” (stage 3) maturity level, the committee is signifying that it believes design work is complete and further refinement will require implementation experience, significant usage and external feedback.
