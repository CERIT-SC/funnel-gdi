# Release policy and process

This document defines how funnel-gdi is versioned, released and supported
(the release policy) and the steps of every release (the release checklist).

## Release policy

### Versions

| Rule | |
|---|---|
| Format | `X.Y.Z.N`, without a `v` prefix, e.g. `0.12.2.1` |
| `X.Y.Z` | The upstream Funnel release the fork is based on (stated in the release notes of each version). The only exception is `0.11.2.1`, released before this rule, which is based on upstream 0.11.1. |
| `N` | The GDI revision on that upstream base, starting at `1`; it starts again at `1` after merging a newer upstream release |
| Tags | One git tag per release, created on `master`. Release tags are never moved or deleted. |
| Pre-releases | Not used. Test images are published under other tag names (manual run of the `Git Tag` workflow, e.g. `manual-test`). |

### When a release is made

Releases are made **on demand**:

| Trigger | Release |
|---|---|
| Changes on `master` are ready to be deployed | Next GDI revision `X.Y.Z.(N+1)` |
| A newer upstream release has been merged | `X'.Y'.Z'.1` on the new upstream base |
| A security vulnerability is fixed | As soon as possible, as the next revision of every supported release line (see [Support](#support)) |

Releases are made by maintainers of `CERIT-SC/funnel-gdi` with write access.
The release changes go to `master` through a reviewed pull request.

### Release criteria

A version is released only when all of these hold:

| Criterion | Checked by |
|---|---|
| CI is green on `master`: unit tests, TES compliance, Nextflow, documentation site build and links | GitHub Actions |
| `README.md` and `DEPLOYMENT.md` name the version, the Helm chart's `appVersion` matches | `scripts/check-docs-version.sh`, run again by the `Git Tag` workflow, which stops the release otherwise |
| The documentation has been reviewed against the changes | [Checklist](#release-checklist), step 3 |
| The platforms listed as "Tested" in `DEPLOYMENT.md` have been re-tested for this version | [Checklist](#release-checklist), step 4 |

### What a release contains

| Artifact | Where |
|---|---|
| Git tag `X.Y.Z.N` | https://github.com/CERIT-SC/funnel-gdi/tags |
| GitHub Release with the release notes of the version (generated from the merged pull requests, plus the upstream base and upgrade notes) and links to its documentation | https://github.com/CERIT-SC/funnel-gdi/releases |
| Container image, linux/amd64 and linux/arm64 | `ghcr.io/cerit-sc/funnel-gdi:X.Y.Z.N` |
| Helm chart (`appVersion: "X.Y.Z.N"`) | `deploy-guide/kubernetes` at the tag |
| Documentation of the version | `https://cerit-sc.github.io/funnel-gdi/X.Y.Z.N/` (and `latest/` for the newest release); the repository documents at the tag |

There are no prebuilt binaries; build them from the tag (`make build`).

### Support

| Release | Bug fixes | Security fixes |
|---|---|---|
| Latest release | Yes | Yes |
| Previous release, if it has a different upstream base (`X.Y.Z`) | No | Yes, for **3 months** after the latest release is published |
| Previous release on the same upstream base | No — upgrade to the latest release (same base, compatible, see below) | No |
| Older releases | No | No |

Fixes are released as a new revision of the supported line, e.g. a security
fix for `0.11.3.1` after `0.12.2.1` is out is released as `0.11.3.2`.
The currently supported versions are listed in `SECURITY.md`, which also
explains how to report a vulnerability.

### Compatibility

| Upgrade | What to expect |
|---|---|
| To a newer revision on the same upstream base (`X.Y.Z.N` → `X.Y.Z.N+1`) | Compatible: the config format and the Helm chart values keep working. Any exception is described in the release notes with upgrade notes. |
| To a new upstream base (`X.Y.Z` → `X'.Y'.Z'`) | May need changes to the config and the Helm values. Read the release notes of every version in between and the [Upgrade and rollback](DEPLOYMENT.md#upgrade-and-rollback) section of the deployment guide. |

Removed features and changed configuration keys are always listed in the
release notes.

## Release checklist

1. **Version in the docs.** In `README.md` and `DEPLOYMENT.md`, change
   `**Documentation version: X.Y.Z.N (unreleased)**` to
   `**Documentation version: X.Y.Z.N**`.
2. **Helm chart.** In `deploy-guide/kubernetes/Chart.yaml`, set
   `appVersion: "X.Y.Z.N"` and increase `version` if the chart changed.
3. **Review the documentation.** Read `README.md`, `DEPLOYMENT.md` and
   `GDI-FEATURES.md` against the changes in this release: deployment steps,
   configuration, system requirements, GDI features and links. Update the
   supported versions in `SECURITY.md` according to [Support](#support).
   On the website, check the hand-written pages (`website/content/gdi.md`,
   `website/content/download.md`) and that the pages marked
   `gdi_support: unsupported` in `website/content/docs/` match
   "Not supported" in `DEPLOYMENT.md`. Preview with `make website-versions`.
4. **Re-test the platforms.** Repeat the manual tests listed as "Tested" in
   `DEPLOYMENT.md` → "Supported and tested platforms" and update the versions
   and the dates there:
   - Kubernetes: build and push an image of the release commit (the release
     image does not exist until the tag is pushed), deploy it with Option 2,
     run the commands in "Testing the server", uninstall. Record
     `kubectl version` (server) and `helm version`.
   - Web dashboard: start a server (Option 1), submit the hello-world task
     from "Testing the server" and run
     `node scripts/dashboard-smoke-test.mjs http://localhost:8000 <task-id> build/dashboard-test`
     (setup in the script header). Record the browser version it prints.
     If the dashboard bundle changed, check its minimum Chrome version with
     `npx es-check` on `/static/js/main.*.js`.
5. **Check.** `scripts/check-docs-version.sh X.Y.Z.N` must pass.
6. **Merge** the release changes to `master`.
7. **Tag and push.**
   ```bash
   git tag X.Y.Z.N
   git push origin X.Y.Z.N
   ```
   The `Git Tag` workflow checks the documentation version again, publishes
   `ghcr.io/cerit-sc/funnel-gdi:X.Y.Z.N` and creates a GitHub Release with
   release notes generated from the merged pull requests and links to the
   documentation at this tag. The `Documentation site` workflow publishes
   `https://cerit-sc.github.io/funnel-gdi/X.Y.Z.N/` and updates `latest/`;
   check that both show the new version.
8. **Complete the release notes.** Edit the GitHub Release and add the
   upstream base (`Based on upstream calypr/funnel <release or commit>`) and,
   if the config format or Helm values changed, the upgrade notes.
9. **Start the next version.** Set both docs to
   `**Documentation version: X.Y.Z.N+1 (unreleased)**`.

For a security fix of the previous release line, create a branch from its
tag (e.g. `git switch -c release-0.11.3 0.11.3.1`), apply the fix, follow the
same checklist on that branch and tag `X.Y.Z.(N+1)` there.

## One-time repository settings

The documentation website is deployed by `.github/workflows/pages.yaml`.
It needs, once (repository admin):

- *Settings → Pages → Build and deployment → Source:* **GitHub Actions**.
- *Settings → Environments → github-pages → Deployment branches and tags:*
  allow the branch `master` and tags matching `*.*.*.*`.

Then run the `Documentation site` workflow manually once (or push to
`master`) to publish `dev/`.
