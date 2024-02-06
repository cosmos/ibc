# Contribution Guidelines

Thanks for your interest in contributing to IBC! Contributions are always welcome. 

Contributing to this repo can mean many things such as participating in discussion or proposing changes. To ensure a smooth workflow for all contributors, the general procedure for contributing has been established:

- Feel free to [open](https://github.com/cosmos/ibc/issues/new) an issue to raise a question, explain a concern, or discuss a possible future feature, protocol change, or standard.
  - Make sure first that the issue does not already [exist](https://github.com/cosmos/ibc/issues).
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
  - The changes must be consistent with the other IBC standards, especially the [core IBC standards](../README.md#core). 
  - The modified standard must be consistent with the description from [ICS 1](../spec/ics-001-ics-standard/README.md).
- If you are only making "surface level" reviews, submit any notes as `Comments` without adding a review.

### PR Targeting

Ensure that you base and target your PR on the `master` branch.

### Development Procedure

- The latest state of development is on `master`.
- Create a development branch either on `github.com/cosmos/ibc` or your fork (using `git remote add fork`).
  - To ensure a clear ownership of branches on the ibc repo, branches must be named with the convention `{moniker}/{issue#}-branch-name`
- Before submitting a pull request, begin `git rebase` on top of `master`. 
  **Since standards cannot be compiled, make sure that the changes in your PR remains consistent with the new commits on the `master` branch**.

### Pull Merge Procedure

- Ensure all github requirements pass.
- Squash and merge pull request.
