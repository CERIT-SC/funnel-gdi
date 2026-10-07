---
title: Download
gdi: true
menu:
  main:
    weight: -1000
---

# Download funnel-gdi

funnel-gdi is released as a container image. There are no prebuilt binaries;
build the binary from source if you need one. Before deploying, read the
[Deployment guide](/gdi/deployment/) and its
[system requirements](/gdi/deployment/#system-requirements).

> [!NOTE]
>
> Funnel runs tasks as containers, so the server needs access to Docker
> (Options 1 and 3 of the deployment guide) or to a Kubernetes cluster (Option 2).

## Container image

| | |
|---|---|
| Image | `ghcr.io/cerit-sc/funnel-gdi:<version>` |
| Platforms | linux/amd64, linux/arm64 |
| Versions | [package page](https://github.com/CERIT-SC/funnel-gdi/pkgs/container/funnel-gdi), [Releases](https://github.com/CERIT-SC/funnel-gdi/releases), [Changelog](/gdi/changelog/) |

This documentation describes version **{{< gdi-version >}}**. Use the image
with the same version as the documentation you follow.

```sh
docker run -p 8000:8000 ghcr.io/cerit-sc/funnel-gdi:<version> server run

# With config
docker run -p 8000:8000 -v ./config.yaml:/config.yaml \
  ghcr.io/cerit-sc/funnel-gdi:<version> server run --config /config.yaml
```

This image cannot start task containers by itself. To run tasks, deploy it on
Kubernetes ([Option 2](/gdi/deployment/#option-2-kubernetes)) or use the
Docker-in-Docker variant ([Option 3](/gdi/deployment/#option-3-docker-container-no-kubernetes)).

## Build from source

Requires Go 1.26 or newer, `make` and `git`.

```sh
git clone https://github.com/CERIT-SC/funnel-gdi.git
cd funnel-gdi
git checkout <version>
make build
./funnel version
```

See [Option 1: Local binary](/gdi/deployment/#option-1-local-binary) for how
to run it.

## Upstream Funnel

Binaries, the install script and the `quay.io/ohsu-comp-bio/funnel` images
of upstream Funnel do **not** contain the GDI features. They are available
from the upstream [Releases](https://github.com/calypr/funnel/releases) page.
