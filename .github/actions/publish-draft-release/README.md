<!-- SPDX-License-Identifier: Apache-2.0 -->

# publish-draft-release

Call this action once per release, in a job that needs every build job. It must run for a tag push, because it checks the files against the pushed tag. The [shared release actions](../README.md) page explains what it produces.

```yaml
jobs:
  release:
    needs: [build]
    runs-on: ubuntu-latest
    permissions:
      contents: write
      id-token: write
    steps:
      - uses: cosmos/ibc/.github/actions/publish-draft-release@<commit-sha> # main
        with:
          name: ibc-cli-v1.2.3
          title: IBC CLI v1.2.3
          prerelease: "false"
```

- `name` is the file name of the attestation bundle, without `.intoto.jsonl`.
- `title` is the release title, used when the draft is created.
- `prerelease` sets **Set as a pre-release** on the draft.

The action fails rather than ship a wrong or unsigned file. That happens when two build jobs produce a file with the same name, or when an existing draft holds a file this run does not produce.

## Gate an image on verified files

A release that also publishes an image should push it only after the files verify. Call the action first with `verify-only`, in a job the image jobs need. It verifies the files without signing or publishing anything, so the job only needs `contents: read`.

```yaml
      - uses: cosmos/ibc/.github/actions/publish-draft-release@<commit-sha> # main
        with:
          name: ibc-cli-v1.2.3
          title: IBC CLI v1.2.3
          verify-only: "true"
```

[`action.yml`](./action.yml) lists every input.
