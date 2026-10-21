<!-- SPDX-License-Identifier: Apache-2.0 -->

# Contributing to IBC

Thank you for your interest in contributing to IBC. IBC is an open standard for trust-minimized interoperability between independent ledgers, stewarded under the Linux Foundation Decentralized Trust (LFDT). Contributions come in many forms, and all of them follow the same public path.

Before participating, please read the LFDT Code of Conduct. If you use AI or LLM tooling in your contributions, see the AI sign-off rule under Developer Certificate of Origin below.

## Ways to contribute

A contribution does not have to be code:

- Report a bug as a GitHub issue using the bug template, with enough detail to reproduce.
- Propose a feature or enhancement as a GitHub issue using the feature template.
- Improve documentation, which follows the same pull request path as code and is the easiest first contribution.
- Submit code against an accepted issue.
- Review other contributors' pull requests, which is one of the most useful ways to build standing toward Maintainer.

## Where to start

Browse issues labeled good first issue and help wanted. Before you start work on an issue, comment on it or raise it in the community chat so a maintainer can assign it to you and you avoid duplicating effort. If you have an idea with no issue yet, open an issue using the feature template or raise it on a community call first. This step is expected, not optional, and it protects you from investing in work that overlaps or conflicts with existing plans.

## Communication

- GitHub Discussions for open questions and community Q&A.
- GitHub Issues for proposals, bugs, and scoped, accepted work, through the templates.
- GitHub pull requests for ADRs, code, docs, and specs.
- Community calls, open to all and recorded, with a public calendar, for synchronous design and roadmap.
- Community chat on the LFDT Discord for questions and coordination.
- The governance forum named in [MAINTAINERS.md](MAINTAINERS.md) for formal and asynchronous governance decisions.
- Security disclosures follow the private path in [SECURITY.md](SECURITY.md) and never go through public issues.

## Contribution workflow

Everyone, without exception, contributes changes through pull requests. This is the same path for maintainers and first-time contributors: the same review, the same CI, and no direct pushes to main.

1. Reach rough agreement on an issue or a community call that the problem is worth solving.
2. For anything that changes architecture, a public interface, or a specification, submit an Architecture Decision Record (ADR) as a markdown pull request and have it merged under lazy consensus before implementation. Trivial changes, bug fixes, and documentation corrections skip this step.
3. Make sure an accepted, scoped GitHub issue exists. The issue is the record of truth for status.
4. Create a topic branch named after the issue. Contributors fork the repository and add an upstream remote. Maintainers may branch directly in the repository.
5. Make your change, following the pull request guidelines below.
6. Open a pull request. Mark it a draft if it is not ready. Link the issue with a closing keyword such as fixes #1234.
7. Address review feedback, keep CI green, and a Maintainer will merge once approved.

## Lazy consensus

Proposals and ADRs proceed under lazy consensus. Anyone can comment, and an objection blocks only if it comes from a Maintainer, which is what standing means here. The objection window is 5 working days from when the proposal or ADR pull request is opened. If no Maintainer objects in that time, a Maintainer records the outcome and it proceeds. An objection should state its reason and, where possible, a change that would resolve it. Objections that cannot be resolved go to a TSC vote.

## Pull request guidelines

- Each pull request should address one specific issue or concern, with a scope that fits comfortably within a reviewer’s mental context. Avoid bundling unrelated fixes, features, or formatting, and split larger work into a sequence of smaller pull requests that reviewers can take in order.
- Every pull request is squash-merged, with a Conventional Commits title as the squash commit message. Keep your branch up to date with main before merge, by rebase or merge, whichever you prefer. A merge queue is not used at launch and would only be introduced if pull request volume makes it necessary.
- Use draft pull requests for work in progress.
- Be responsive. A pull request that sits with unaddressed comments until it needs a full rebase is slower for everyone.

## Commit messages

Write commit messages in Conventional Commits form, for example fix(core): correct packet timeout handling. The history is a record people rely on to understand why a change was made and it feeds release notes.

## Testing and CI

Include tests appropriate to the change, covering the expected path and error handling. Some changes, such as documentation, do not need tests. Where adding coverage needs a large test-suite refactor, or where end-to-end tests are tracked separately, that work can be split into its own pull request or issue, noted in the description. Passing public CI is a hard gate for merge, not a formality. If a status check fails for a reason you believe is unrelated to your change, leave a comment explaining why and a maintainer will look into it.

## Developer Certificate of Origin (DCO)

All commits must be signed off to satisfy the Developer Certificate of Origin. Sign off every commit using git commit -s. The sign-off must match your commit author identity. A bot checks the sign-off is present; it does not police identity. Configure your name and email once:

```
git config user.name "FIRST_NAME LAST_NAME"
git config user.email "you@example.com"
```

Only a human may sign off a commit. Where AI or LLM tooling assisted, disclose it with a Co-Authored-By or Assisted-By trailer in the commit message body, naming the tool, model, and version. The human contributor still signs off and remains responsible for the change. Example:

```
Co-Authored-By: <model> <version> <noreply@example.com>
Signed-off-by: Jane Contributor <jane@example.com>
```

Maintainers may decline AI-assisted contributions that do not meet these expectations, and the response can be to the person, not just the pull request. Where someone repeatedly opens low-effort or machine-generated pull requests that add no clear value, ignores this guidance, or treats the project as a venue for volume rather than genuine contribution, maintainers may close their open pull requests and decline to engage further. Persistent cases are handled as a conduct matter through the Code of Conduct responders, who may block the account from the project. This is a moderation action about effort and conduct, not about the use of AI, which remains welcome wherever a contributor stands behind what they submit.

## Changelog

The changelog is generated from Conventional Commit titles at release time, so a well-formed pull request title is what matters rather than a hand-written entry. CI checks the title format.

## Code review

All changes are code reviewed. For non-trivial changes, the reviewer should be a Maintainer familiar with the affected area, and they may pull in a second reviewer at their discretion. Merge is performed by a Maintainer, normally the one who approved. If a pull request has gone about five working days without a reviewer emerging, ask in the community chat rather than pinging individuals. Disagreements that the reviewing Maintainers cannot resolve escalate to the TSC, and only genuinely cross-cutting disputes reach the full committee.

## Copyright and license

Code is contributed under Apache 2.0, specifications under the Community Specification License 1.0, and documentation under CC BY 4.0, regardless of where in the repository the file lives. The SPDX header on each file records which applies.

## Trivial contributions

Small documentation and typo fixes from genuine contributors are welcome, since lowering the barrier for first-time contributors matters. This is balanced against low-effort or automated submissions: maintainers may batch, close, or decline trivial pull requests that appear mass-generated or add no clear value. The AI-assisted contribution rules above apply, so undisclosed or unreviewed generated output can be declined on that basis alone.

## Security

If you think you have found a security issue, do not open a public issue. Follow the coordinated disclosure process in [SECURITY.md](SECURITY.md).

## Initial Maintainers

The initial Maintainers form the initial TSC and are its voting members.

Giovanni Vitali, Cosmos Labs (Chair)  
Dennis Fang, Cosmos Labs  
Gjermund Garaba, Cosmos Labs  
Serdar Türkmenafşar, Cosmos Labs  
Dmitry Shlemin, Cosmos Labs  
Dylan Kilkenny, OpenZeppelin  
Jun Kimura, Datachain

## Governance

The project is governed by a Technical Steering Committee under the Technical Charter. See [GOVERNANCE.md](GOVERNANCE.md) for the roles and how decisions are made.
