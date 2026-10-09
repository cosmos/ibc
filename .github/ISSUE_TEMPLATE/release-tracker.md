---
# SPDX-License-Identifier: Apache-2.0
name: Release tracker
about: Create an issue to track release progress
---

<!-- < < < < < < < < < < < < < < < < < < < < < < < < < < < < < < < < < ☺
v                            ✰  Thanks for opening an issue! ✰
v    Before smashing the submit button please review the template.
v    Word of caution: poorly thought-out proposals may be rejected
v                     without deliberation
☺ > > > > > > > > > > > > > > > > > > > > > > > > > > > > > > > > >  -->

## Milestones

<!-- Links to alpha, beta, RC or final milestones -->

## IBC spec compatibility

<!-- Version of the IBC spec that this release is compatible with -->

## QA

### Backwards compatibility

<!-- List of tests that need to be performed with previous
versions to guarantee that no regression is introduced -->

- [ ] [E2E tests](https://github.com/cosmos/ibc/actions/workflows/e2e.yml) pass for the release commit.
- [ ] Database migrations (`ibc migrate up`) apply cleanly on top of the previous release.
- [ ] Existing configs from the previous release load without manual changes.

### Other testing

## Migration

<!-- Link to migration document -->

## Checklist

<!-- Remove any items that are not applicable. -->

- [ ] Branch off main to create release branch in the form of `release/vx.y.z` and add branch protection rules.
- [ ] Push the release tag and confirm the [IBC CLI Build Artifacts](https://github.com/cosmos/ibc/actions/workflows/ibc-cli-build.yml) workflow publishes binaries and the container image.

## Post-release checklist

- [ ] Update [`CHANGELOG.md`](https://github.com/cosmos/ibc/blob/main/CHANGELOG.md)
- [ ] Update the docs (`docs/`) with any version-specific changes, and permalinks with links of the released tag.

---

#### For Admin Use

- [ ] Not duplicate issue
- [ ] Appropriate labels applied
- [ ] Appropriate contributors tagged/assigned
