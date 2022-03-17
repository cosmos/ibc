## IBC Standardisation Process

IBC standardisation will follow an adaptation of the [TC 39](https://tc39.github.io/process-document/) process used by the ECMAScript steering committee.

#### Stage 1 - `Strawman`

- _**Purpose**_: Start the specification process
- _**Entrance Criteria**_: [Open an issue](https://github.com/cosmos/ibc/issues/new) on this repository with a short outline of your proposal and a specification name.
- _**Acceptance Requirements**_: No acceptance required to move to the next stage. Keep the issue around to track the specification status, and close it when the final specification is merged or the proposal abandoned.
- _**Spec Quality**_: Outline only. Link to any prior documentation, discussion, or reference materials.
- _**Changes Expected Post-Acceptance**_: N/A
- _**Implementation Types Expected**_: None required, but link to any existing

#### Stage 2 - `Draft`

- _**Purpose**_:
  * Make the case for the addition of this specification to the IBC ecosystem
  * Describe the shape of a potential solution
  * Identify challenges to this proposal
- _**Entrance Criteria**_:
  * Prose outlining the problem or need and the general shape of a solution in a PR to a `./spec/{area}/ics-{{ .Spec.Number }}-{{ .Spec.Name }}/README.md` file in this repository.
    This file should contain:
    - List of expected projects & users within the Cosmos ecosystem who might make use of the specification along with any particular requirements they have
    - Discussion of key algorithms, abstractions, and semantics
    - High-level application interface outline, where applicable
    - Identification of potential design trade-offs and implementation challenges/complexity
    
    For a more detailed description of standard requirements, see [ICS 1](../spec/ics-001-ics-standard).
  
    For more details on submitting a PR, take a look at the [Pull Requests](./CONTRIBUTING.md#pull-requests) section in the contribution guidelines.
  * Identified `author(s)` who will advance the proposal in the header of the standard file
  * Any additional reference documentation or media in the `./spec/ics-{{ .Spec.Number }}-{{ .Spec.Name }}` directory
  * The specification team expects that this proposal will be finalised and eventually included in the IBC standard set.
- _**Spec Quality**_:
  * Follows the structure laid out in ICS 1 and provides a reasonable overview of the proposed addition.
- _**Acceptance Requirements**_:
  * The PR has received two approvals from members of the [core specification committee](./STANDARDS_COMMITTEE.md), at which point it can be merged into the IBC repository.
- _**Changes Expected Post-Acceptance**_:
  * Changes to details but not to the key concepts are expected after a standard enters draft stage. Implementers should work with the spec authors as work continues on spec development.
- _**Implementation Types Expected**_:
  * Tightly bounded demos, example repositories showing reproduction steps for issues fixed by proposal

#### Stage 3 - `Candidate`

- _**Purpose**_:
  * Indicate that further refinement will require feedback from implementations and users
- _**Entrance Criteria**_:
  * Everything from stages 1 & 2
  * Complete specification text
  * At least one specification-compatible implementation exists
  * All relevant ecosystem stakeholders have been given a chance to review and provide feedback on the standard
  * The solution is complete and no further work is possible without implementation experience, significant usage and external feedback.
- _**Spec Quality**_:
  * Complete: all semantics, syntax and API are completed as described
- _**Acceptance Requirements**_:
  * The PR changing the stage to "candidate" has been approved by two members of the core specification team.
- _**Changes Expected Post-Acceptance**_:
  * Limited: only those deemed critical based on implementation experiences.
- _**Implementation Types Expected**_:
  * Specification-compliant

#### Stage 4 - `Finalised`

- _**Purpose**_:
  * Indicate that the addition is included in the formal ICS standard set
- _**Entrance Criteria**_:
  * Everything from stages 1,2,3
  * At least two specification-compatible implementations exist, and they have been tested against each other
  * All relevant ecosystem stakeholders approve the specification (any holdout can block this stage)
  * Acceptance tests are written and merged into the relevant repositories
  * All files in the `./spec/ics-{{ .Spec.Number }}-{{ .Spec.Name}}/` directory are up to date and merged into the `cosmos/ics` repository
- _**Acceptance Requirements**_:
  * The PR changing the stage to "finalised" has been approved by representatives from all relevant ecosystem stakeholders, and all members of the core specification team.
- _**Spec Quality**_:
  * Final: All changes as a result of implementation experience are integrated
- _**Changes Expected Post-Acceptance**_:
  * None
- _**Implementation Types Expected**_:
  * Shipping/Production

### Calls for implementation and feedback

When an addition is accepted at the “candidate” (stage 3) maturity level, the committee is signifying that it believes design work is complete and further refinement will require implementation experience, significant usage and external feedback.
