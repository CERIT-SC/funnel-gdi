[![Tests][tests-badge]][tests]
[![Compliance Tests Status][compliance-tests-badge]][compliance-tests]
[![License: MIT][license-badge]][license]
[![GitHub Release][release-badge]][release]
[![Docker][docker-badge]][docker]

[tests-badge]: https://github.com/CERIT-SC/funnel-gdi/actions/workflows/tests.yaml/badge.svg?branch=master-gdi
[tests]: https://github.com/CERIT-SC/funnel-gdi/actions/workflows/tests.yaml

[compliance-tests]: https://github.com/CERIT-SC/funnel-gdi/actions/workflows/compliance.yaml
[compliance-tests-badge]: https://img.shields.io/github/actions/workflow/status/CERIT-SC/funnel-gdi/compliance.yaml?branch=master-gdi&label=TES%20Compliance

[license-badge]: https://img.shields.io/badge/License-MIT-yellow.svg
[license]: https://opensource.org/licenses/MIT

[release-badge]: https://img.shields.io/github/v/tag/CERIT-SC/funnel-gdi?label=release
[release]: https://github.com/CERIT-SC/funnel-gdi/releases

[docker-badge]: https://img.shields.io/badge/Image-ghcr.io-blue?logo=docker
[docker]: https://github.com/CERIT-SC/funnel-gdi/pkgs/container/funnel-gdi

# Funnel (GDI fork)

**Documentation version: 0.12.2.1 (unreleased)** — this documentation describes funnel-gdi
0.12.2.1. For another version, open the repository at the matching git tag
(e.g. `https://github.com/CERIT-SC/funnel-gdi/tree/0.11.3.1`) or follow the
docs links in [CHANGELOG-GDI.md](CHANGELOG-GDI.md) and on the
[Releases](https://github.com/CERIT-SC/funnel-gdi/releases) page.
A running server reports its version at `GET /v1/service-info`.

**Documentation website:** https://cerit-sc.github.io/funnel-gdi/ — one copy per
release (`/<version>/`), `/latest/` and `/dev/` (`master-gdi`), built from the
documents in this repository.

This is the [GDI](https://gdi.onemilliongenomes.eu/) fork of
[calypr/funnel](https://github.com/calypr/funnel), maintained by
[CERIT-SC](https://github.com/CERIT-SC).

Funnel is a toolkit for distributed, batch task execution, including a server, worker, and a set of compute, storage, and database backends. Given a task description, Funnel will find a worker to execute the task, download inputs, run a series of (Docker) containers, upload outputs, capture logs, and track the whole process.

Funnel is an implementation of the [GA4GH Task Execution Schemas](https://github.com/ga4gh/task-execution-schemas), an effort to standardize the APIs used for task execution across many platforms.

Funnel provides an API server, multiple storage backends (local FS, S3, Google Bucket, etc.), multiple compute backends (local, HTCondor, Google Cloud, etc.), and a web dashboard.

Upstream project and its reference documentation: https://calypr.org/tools/funnel/

## GDI fork additions

Per-task storage modes on Kubernetes (`shared` / `pvc` / `full`) with a Helm chart that needs only namespaced permissions, HTSGET and SDA storage with Crypt4GH decryption, bearer token propagation, audit logging, panic recovery and more. The full list, with the files and tests of each feature, is in [GDI-FEATURES.md](GDI-FEATURES.md). Images are published to `ghcr.io/cerit-sc/funnel-gdi:<version>` for every release; the available versions are on the [package page](https://github.com/CERIT-SC/funnel-gdi/pkgs/container/funnel-gdi).

## Documentation

| Document | Contents |
|---|---|
| [DEPLOYMENT.md](DEPLOYMENT.md) | How to deploy (local binary, Kubernetes, Docker), system requirements, supported and tested platforms, upgrade and rollback, production checklist |
| [CHANGELOG-GDI.md](CHANGELOG-GDI.md) | Changes per GDI version, with links to the docs of each release |
| [GDI-FEATURES.md](GDI-FEATURES.md) | What the fork adds on top of upstream and how to keep it in sync |
| [RELEASING.md](RELEASING.md) | Release policy (versioning, when releases are made, support, compatibility) and the release checklist |
| [SECURITY.md](SECURITY.md) | Supported versions and reporting security issues |
| [Documentation website](https://cerit-sc.github.io/funnel-gdi/) (source in `website/`) | The documents above as web pages, per version, plus the upstream reference docs for generic Funnel features (task API, storage, databases, compute backends), marked as upstream. `CHANGELOG.md` and `CONTRIBUTING.md` are upstream files. |
