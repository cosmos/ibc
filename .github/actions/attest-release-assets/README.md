<!-- SPDX-License-Identifier: Apache-2.0 -->

# attest-release-assets

Call this action in each build job, after the job puts its release files in `out/assets`. The [shared release actions](../README.md) page explains what it produces.

```yaml
jobs:
  build:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      id-token: write
      attestations: write
      artifact-metadata: write
    steps:
      # Build, then put the release files in out/assets.
      - uses: cosmos/ibc/.github/actions/attest-release-assets@<commit-sha> # main
        with:
          name: ibc-cli-v1.2.3-linux-amd64
          platform: linux-amd64
          sbom-file: cli/dist/ibc
```

- `name` is the SBOM's file name, without `.spdx.json`.
- `platform` names the job's upload, so give each build job a different one.
- `sbom-file` points at a binary. For a source tree with lockfiles, set `sbom-dir` instead.

The action fails if the SBOM lists no packages besides the scanned file or directory. That usually means the SBOM came from the wrong path. For a component without dependencies, set `min-sbom-packages: "0"`.

[`action.yml`](./action.yml) lists every input.
