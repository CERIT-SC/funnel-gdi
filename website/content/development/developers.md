---
gdi: true
title: Funnel Developers

menu:
  main:
    parent: Development
    weight: 30
---

# Developers

Notes for people who want to build funnel-gdi from source and change the code.
To run a build, see the [Deployment guide](/gdi/deployment/); for what the
fork adds to upstream Funnel and how to keep it in sync, see
[GDI features](/gdi/features/).

## Building the source

1. Install [Go 1.26 or newer][go] (the version in `go.mod`), `make` and `git`.
2. Clone the repository and build:

```shell
git clone https://github.com/CERIT-SC/funnel-gdi.git
cd funnel-gdi
git checkout master        # development branch; or a release tag X.Y.Z.N
make build                 # builds ./funnel
./funnel version
```

`make install` builds and installs the binary into `$(go env GOPATH)/bin`
instead.

## Developer tools

| Tool | Used for |
|---|---|
| [Go][go] 1.26+ | The server, worker, storage and compute backends |
| [Task Execution Schemas][tes] | The task API (`tes/`, regenerated with `make proto`) |
| [Protobuf][protobuf], [gRPC][grpc], [gRPC Gateway][gateway], [Buf][buf] | RPC and HTTP APIs (`make proto-depends`, `make proto`) and the config format (`config/config.proto`) |
| [Node.js][node] and [npm][npm] | The web dashboard (React, `webdash/`) |
| [Docker][docker] | Running task containers (local compute) and building images |
| [Hugo][hugo] (extended) and Python 3 | This documentation website (`website/`) |

## Common tasks

| Task | Command |
|---|---|
| Run all unit tests | `make test` (or `go test ./...`) |
| Run some tests | `go test ./worker/...`, `go test ./worker -run Cancel` |
| Run the tests against a database | `make start-mongodb test-mongodb` (and similar targets in the `Makefile`) |
| Rebuild the web dashboard | `cd webdash && npm ci && cd ..`, then `make webdash` (updates `webdash/web.go`) |
| Regenerate the TES API code | `make proto-depends`, then `make proto` |
| Preview the website | `make website-versions` (builds `build/pages/` and checks its links) |

After changing `config/config.proto` or the files in `config/`, regenerate the
Go code and the bundled defaults as described in
[GDI features → Keeping in sync with upstream](/gdi/features/#keeping-in-sync-with-upstream).

## Mocking

The [testify][testify] and [mockery][mockery] tools are used to generate and use
mock interfaces in test code; `make gen-mocks` regenerates the mocks of the
scheduler interfaces (`compute/scheduler`).

## Making a release

Releases of funnel-gdi follow the [Release policy](/gdi/release-policy/) and
its release checklist: `X.Y.Z.N` tags on `master`, a container image on
`ghcr.io/cerit-sc/funnel-gdi` and no prebuilt binaries. The upstream release
process (`make release`, goreleaser, Homebrew) is not used in the fork.

[go]: https://go.dev/
[protobuf]: https://protobuf.dev/
[grpc]: https://grpc.io/
[gateway]: https://github.com/grpc-ecosystem/grpc-gateway
[buf]: https://buf.build/
[docker]: https://www.docker.com/
[node]: https://nodejs.org
[npm]: https://www.npmjs.com/
[hugo]: https://gohugo.io/
[tes]: https://github.com/ga4gh/task-execution-schemas
[testify]: https://github.com/stretchr/testify
[mockery]: https://github.com/vektra/mockery
