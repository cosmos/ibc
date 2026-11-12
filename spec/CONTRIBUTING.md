<!-- SPDX-License-Identifier: Community-Spec-1.0 -->

# Contribution Guidelines

Thanks for your interest in contributing to IBC! Contributions are always welcome. 

The specifications are developed under the Community Specification process. By contributing you agree to the [Contributor License Agreement](00-contributor-license-agreement.md), and your contribution is made under the [Community Specification License 1.0](01-community-specification-license-v1.md) and the [Contribution Policy](06-contributing.md). Every commit must be signed off with `git commit -s`; see the repository's [CONTRIBUTING.md](../CONTRIBUTING.md#developer-certificate-of-origin-dco).

Contributing to this repo can mean many things such as participating in discussion or proposing changes. To ensure a smooth workflow for all contributors, the general procedure for contributing has been established:

- Feel free to [open](https://github.com/lfdt-ibc/ibc/issues/new) an issue to raise a question, explain a concern, or discuss a possible future feature, protocol change, or standard.
  - Make sure first that the issue does not already [exist](https://github.com/lfdt-ibc/ibc/issues).
  - Participate in thoughtful discussion on the issue.

- If you would like to contribute in fixing / closing an issue:
  - If the issue is a proposal, ensure that the proposal has been accepted.
  - Ensure that nobody else has already begun working on this issue. If they have, make sure to contact them to collaborate.
  - If nobody has been assigned for the issue and you would like to work on it, make a comment on the issue to inform the community of your intentions to begin work.
  - Follow standard Github best practices: fork the repo, branch from the HEAD of `master`, make some commits, and submit a PR to `master`. 
    For more details, see the [Pull Requests](#pull-requests) section below. 
  - Be sure to submit the PR early in `Draft` mode, even if it's incomplete as this indicates to the community you're working on something and allows them to provide comments at an early stage.
  - When the PR is complete it can be marked `Ready for Review`.

- If you would like to propose a new standard for inclusion in the IBC standards, please take a look at [PROCESS.md](./PROCESS.md) for a detailed description of the standardisation process.
  - To start a new standardisation document, copy the [template](../spec/ics-template.md) and open a PR.

If you have any questions, you can usually find some IBC team members on the [Cosmos Discord](https://discord.gg/cosmosnetwork).

## Pull Requests

To accommodate the review process we suggest that PRs are categorically broken up.
Each PR should address only a single issue and **a single standard**. 
The PR name should be prefixed by the standard number, 
e.g., `ICS4: Some improvements` should contain only changes to [ICS 4](../spec/core/ics-004-channel-and-packet-semantics/README.md).
If fixing an issue requires changes to multiple standards, create multiple PRs and mention the inter-dependencies.

### Process for reviewing PRs

All PRs require an approval from at least two members of the [standardisation committee](./STANDARDS_COMMITTEE.md) before merge. 
The PRs submitted by one of the members of the standardisation committee require an approval from only one other member before merge. 
When reviewing PRs please use the following review explanations:

- `Approval` through the GH UI means that you understand all the changes proposed in the PR. In addition:
  - You must also think through anything which ought to be included but is not.
  - You must think through any potential security issues or incentive-compatibility flaws introduced by the changes.
  - The changes must be consistent with the other IBC standards, especially the [core IBC standards](../README.md#core-transport). 
  - The modified standard must be consistent with the description from [ICS 1](../spec/ics-001-ics-standard/README.md).
- If you are only making "surface level" reviews, submit any notes as `Comments` without adding a review.

### PR Targeting

Ensure that you base and target your PR on the `master` branch.

### Development Procedure

- The latest state of development is on `master`.
- Create a development branch either on `github.com/lfdt-ibc/ibc` or your fork (using `git remote add fork`).
  - To ensure a clear ownership of branches on the ibc repo, branches must be named with the convention `{moniker}/{issue#}-branch-name`
- Before submitting a pull request, begin `git rebase` on top of `master`. 
  **Since standards cannot be compiled, make sure that the changes in your PR remains consistent with the new commits on the `master` branch**.

### Pull Merge Procedure

- Ensure all github requirements pass.
- Squash and merge pull request.

## Copyright

This document is licensed under the Community Specification License 1.0, and source code included in it under the Apache License 2.0 unless otherwise marked. In the case of any conflict or confusion within this document between the Community Specification License and the designated source code license, the terms of the Community Specification License shall apply.

The repository uses three licenses, as set out in [Copyright and license](https://github.com/lfdt-ibc/ibc/blob/main/CONTRIBUTING.md#copyright-and-license):

- **Specifications:** [Community Specification License 1.0](https://github.com/lfdt-ibc/ibc/blob/main/spec/01-community-specification-license-v1.md) ([canonical text](https://github.com/CommunitySpecification/1.0))
- **Code:** [Apache License 2.0](https://github.com/lfdt-ibc/ibc/blob/main/LICENSE) ([canonical text](https://www.apache.org/licenses/LICENSE-2.0))
- **Documentation:** [Creative Commons Attribution 4.0 International](https://github.com/lfdt-ibc/ibc/blob/main/docs/LICENSE) ([canonical text](https://creativecommons.org/licenses/by/4.0/legalcode))

THESE MATERIALS ARE PROVIDED “AS IS.” The Contributors and Licensees expressly disclaim any warranties (express, implied, or otherwise), including implied warranties of merchantability, non-infringement, fitness for a particular purpose, or title, related to the materials.  The entire risk as to implementing or otherwise using the materials is assumed by the implementer and user. IN NO EVENT WILL THE CONTRIBUTORS OR LICENSEES BE LIABLE TO ANY OTHER PARTY FOR LOST PROFITS OR ANY FORM OF INDIRECT, SPECIAL, INCIDENTAL, OR CONSEQUENTIAL DAMAGES OF ANY CHARACTER FROM ANY CAUSES OF ACTION OF ANY KIND WITH RESPECT TO THIS DELIVERABLE OR ITS GOVERNING AGREEMENT, WHETHER BASED ON BREACH OF CONTRACT, TORT (INCLUDING NEGLIGENCE), OR OTHERWISE, AND WHETHER OR NOT THE OTHER MEMBER HAS BEEN ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
