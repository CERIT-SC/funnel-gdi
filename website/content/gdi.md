---
title: GDI fork
gdi: true
menu:
  main:
    name: Overview
    parent: GDI fork
    weight: 1
---

# Funnel GDI

This site documents **funnel-gdi {{< gdi-version >}}**, the
[GDI](https://gdi.onemilliongenomes.eu/) fork of
[Funnel](https://github.com/calypr/funnel) maintained by
[CERIT-SC](https://github.com/CERIT-SC). Funnel is a server and worker for the
[GA4GH Task Execution Service (TES)](https://github.com/ga4gh/task-execution-schemas)
API: it runs batch tasks as containers, downloads their inputs, uploads their
outputs and tracks their state and logs.

## Start here

| I want to… | Read |
|---|---|
| Deploy funnel-gdi | [Deployment guide](/gdi/deployment/) |
| Check hardware and software requirements | [System requirements](/gdi/deployment/#system-requirements) |
| See which platforms are supported and tested | [Supported and tested platforms](/gdi/deployment/#supported-and-tested-platforms) |
| See what changed in a version | [Releases](https://github.com/CERIT-SC/funnel-gdi/releases) (release notes) |
| Know what the fork adds to upstream Funnel | [GDI features](/gdi/features/) |
| Know how versions are released and supported | [Release policy](/gdi/release-policy/) |
| Report a security issue | [Security policy](/gdi/security/) |
| Learn the task API and the backends | [Docs](/docs/) (upstream reference) |

## What the fork adds

- Per-task storage modes on Kubernetes (`shared`, `pvc`, `full`) and a Helm
  chart that needs only namespaced permissions.
- HTSGET and SDA storage for task inputs, with Crypt4GH decryption and
  propagation of the user's bearer token.
- Audit logging of every API request, panic recovery, accurate HTTP status codes.
- Release images `ghcr.io/cerit-sc/funnel-gdi:<version>`.

The complete list with source files and tests is in [GDI features](/gdi/features/).

## About this documentation

| Requirement | How this documentation meets it |
|---|---|
| **Linked to the software version** | Every page shows the funnel-gdi version it describes (bar at the top). Each release has its own copy of this site at `https://cerit-sc.github.io/funnel-gdi/<version>/`, `latest/` is the newest release and `dev/` the development branch; the version switcher moves between them. On the GDI pages, links to the source code point to the same version (the upstream reference pages under Docs link to the development branch `master` or to upstream Funnel), and a running server reports its version at `GET /v1/service-info`. |
| **Regularly updated** | The GDI pages are generated from the documents in the repository (`DEPLOYMENT.md`, `RELEASING.md`, …) every time the site is built, so the site never differs from the repository. The site is rebuilt and published on every change of `master` and on every release. Pull requests follow a documentation checklist, and every release includes a documentation review and a re-test of the platforms ([Release policy](/gdi/release-policy/#release-checklist)). |
| **Supported and tested platforms** | [Supported and tested platforms](/gdi/deployment/#supported-and-tested-platforms): manually tested platforms with versions and date (Kubernetes, the web dashboard in Chromium), the supported browsers (Chrome 64 or newer), CI-tested components, components that are built but not tested, and unsupported backends. Pages about unsupported backends are marked as such. |
| **Deployment guidelines and system requirements** | [Deployment guide](/gdi/deployment/): three deployment options, [minimum resources, storage, network and software requirements](/gdi/deployment/#system-requirements), upgrade and rollback, production checklist and troubleshooting. |
| **Release policy** | [Release policy](/gdi/release-policy/): version numbering, when releases are made, release criteria, what each release contains, which versions are supported and for how long, compatibility between versions, and the release checklist. |

Pages under [Docs](/docs/) come from the upstream Funnel documentation and are
marked as upstream reference. Where they differ from the GDI pages, the GDI
pages apply.
