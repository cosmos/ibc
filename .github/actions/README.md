<!-- SPDX-License-Identifier: Apache-2.0 -->

# Shared release actions

These composite actions hold the release steps that cosmos/ibc, [cosmos/ibc-go](https://github.com/cosmos/ibc-go) and [cosmos/ibc-contracts](https://github.com/cosmos/ibc-contracts) have in common: attesting build outputs, signing, and attaching the signed assets to a draft release. Each repository keeps its own release workflow with its own tag patterns, tag validation and build, and calls these actions for everything after the build.

| Action | What it does | Job permissions it needs |
| --- | --- | --- |
| [`attest-release-assets`](./attest-release-assets/action.yml) | Generates an SPDX SBOM, attests the build provenance and SBOM of the files in `<out-dir>/assets`, and uploads them with the attestation bundles as the `release-assets-<platform>` artifact | `contents: read`, `id-token: write`, `attestations: write`, `artifact-metadata: write` |
| [`publish-draft-release`](./publish-draft-release/action.yml) | Collects every `release-assets-*` artifact of the run, verifies each asset against its bundles, signs `checksums.txt` with cosign, and creates or updates the draft release for the pushed tag with a "Verifying the release" section | `contents: write`, `id-token: write` |
| [`attest-image`](./attest-image/action.yml) | Attests the provenance of a pushed image by digest, optionally generates and attests its SBOM, and optionally signs it (recursively for an index) with cosign | `packages: write`, `id-token: write`, `attestations: write`, `artifact-metadata: write`, and a registry login earlier in the job |

## Why composite actions

Sigstore certificates and GitHub attestations name the workflow that ran the job. With composite actions that is still each repository's own `release.yml` (or `docker.yml`), so the verification commands in each repository's docs and generated release notes stay as they are: `--signer-workflow cosmos/ibc-go/.github/workflows/release.yml` keeps working.

A reusable workflow would instead put `cosmos/ibc/.github/workflows/<file>@<ref>` in every certificate, so every repository's verification instructions would have to name a workflow in this repository and the ref it was called at. It would also take the build out of the caller's hands, which does not fit three repositories that build very different things.

## Using the actions from another repository

Reference the actions by full commit SHA, like every other action in these repositories:

```yaml
uses: cosmos/ibc/.github/actions/publish-draft-release@<commit-sha> # main
```

Do not tag the actions with `v*` or `cli-v*` tags in this repository: pushing either runs the IBC CLI release here. A change to an action reaches ibc-go and ibc-contracts only when they bump the SHA, so bump it in both in the same way you would bump any pinned action.

The `cosmos` organization's Actions policy must allow `cosmos/ibc` actions in those repositories.

Within this repository the release workflow uses the actions by local path (`./.github/actions/...`) from the tagged commit, so a change here is exercised by the next CLI release without a SHA bump. Jobs that do not otherwise check out the repository do a sparse checkout of `.github/actions`.

### Release workflow

A build job puts its release files in `out/assets` and calls `attest-release-assets`. Give each job of a matrix a distinct `platform`. Then a single job, after every build job, calls `publish-draft-release`. It must run for a tag, either a tag push or a manual run started from the tag, because the notes it writes verify against `GITHUB_REF` and `GITHUB_WORKFLOW_REF`.

For example, the ibc-go source archive:

```yaml
  build:
    needs: [validate-tag]
    runs-on: ubuntu-latest
    permissions:
      contents: read
      id-token: write
      attestations: write
      artifact-metadata: write
    env:
      NAME: ${{ needs.validate-tag.outputs.name }}
      PATHS: ${{ needs.validate-tag.outputs.paths }}
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7
        with:
          persist-credentials: false
      - name: Collect assets
        run: |
          mkdir -p out/assets src
          # shellcheck disable=SC2086
          git archive --format=tar --prefix="${NAME}/" "$GITHUB_SHA" -- $PATHS \
            | gzip -n > "out/assets/${NAME}.tar.gz"
          tar -xzf "out/assets/${NAME}.tar.gz" -C src
      - uses: cosmos/ibc/.github/actions/attest-release-assets@<commit-sha> # main
        with:
          name: ${{ env.NAME }}
          sbom-dir: src/${{ env.NAME }}

  release:
    needs: [validate-tag, build]
    runs-on: ubuntu-latest
    permissions:
      contents: write
      id-token: write
    steps:
      - uses: cosmos/ibc/.github/actions/publish-draft-release@<commit-sha> # main
        with:
          name: ${{ needs.validate-tag.outputs.name }}
          title: ${{ needs.validate-tag.outputs.title }}
          prerelease: ${{ needs.validate-tag.outputs.prerelease }}
```

For a matrix, such as the ibc-contracts proof-api archives, pass the platform in both inputs:

```yaml
      - uses: cosmos/ibc/.github/actions/attest-release-assets@<commit-sha> # main
        with:
          name: ${{ env.NAME }}${{ matrix.asset-suffix }}
          platform: ${{ matrix.platform }}
          sbom-dir: src
```

`attest-release-assets` fails when the SBOM lists no packages besides the scanned file or directory, which catches an SBOM generated from the wrong path. Raise `min-sbom-packages` if you know the expected minimum, or set it to `0` for a component without dependencies.

`publish-draft-release` fails if two build jobs produce a file with the same name, rather than letting one overwrite the other.

### Images

Pass the digest that the push itself reported, not one looked up from a tag afterwards. For a multi-platform image, attest each platform image in its build job and sign the index in the job that creates it:

```yaml
      # Per platform, after docker/build-push-action with provenance: false
      - uses: cosmos/ibc/.github/actions/attest-image@<commit-sha> # main
        with:
          image: ghcr.io/cosmos/proof-api
          digest: ${{ steps.build.outputs.digest }}

      # For the index
      - name: Create manifest list and push
        id: index
        run: |
          docker buildx imagetools create --metadata-file "${RUNNER_TEMP}/index.json" ...
          echo "digest=$(jq -r '."containerimage.descriptor".digest' "${RUNNER_TEMP}/index.json")" >> "$GITHUB_OUTPUT"
      - uses: cosmos/ibc/.github/actions/attest-image@<commit-sha> # main
        with:
          image: ghcr.io/cosmos/proof-api
          digest: ${{ steps.index.outputs.digest }}
          sbom: "false"
          sign: "true"
          recursive: "true"
```

For a single-platform image, call it once with `sign: "true"` and the default `sbom: "true"`.

## Changing the actions

Keep the inputs backwards compatible where you can, since ibc-go and ibc-contracts pin a SHA and pick up a change only when they bump it. If an input has to change, update both repositories in the same bump.

To try a change, run this repository's [release workflow](../workflows/release.yml) from your branch and choose an existing release tag as the `tag` input: the run builds that tag's source with your branch's workflow and actions. For example, `gh workflow run release.yml --ref my-branch -f tag=cli-v0.1.0`. That dry run (`dry-run: "true"` on `publish-draft-release`) attests, verifies and signs as a release would, but creates no release and pushes no image, and uploads what would be released as the `release-dry-run` artifact.

To also exercise the draft release and the image signing, use a fork: push a pre-release tag such as `v0.0.0-test.1` there, check the draft release and its verification commands, then delete the draft, the tag and any images it pushed.
