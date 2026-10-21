# Changelog (GDI fork)

All notable changes to the GDI fork of Funnel (`CERIT-SC/funnel-gdi`) are
documented here. `CHANGELOG.md` is the upstream changelog of
[calypr/funnel](https://github.com/calypr/funnel) and is not maintained by
this fork; the current list of GDI additions is in
[GDI-FEATURES.md](GDI-FEATURES.md).

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versions use the scheme `X.Y.Z.N`: `X.Y.Z` is the upstream release the fork
is based on, `N` is the GDI revision on top of it.

## [0.12.2.1] - Unreleased

### Added

- CI reports the statement coverage of the unit and component tests per package (job summary and the `coverage` artifact with an HTML report) and fails when it drops below the minimum `COVERAGE_MIN`. Run `make test-coverage` for the same report locally.
- CI runs the component tests against real databases, event streams and storage (Postgres, MongoDB, Elasticsearch, Badger, DynamoDB local, Datastore and Pub/Sub emulators, Kafka, S3-compatible storage, FTP); their coverage counts toward the total.
- Acceptance tests with coverage: the `Acceptance Tests` workflow builds Funnel with coverage instrumentation (`make build-cover`), runs the TES compliance suite and the Nextflow nf-canary workflow against it, reports the merged coverage per package and fails below `ACCEPTANCE_COVERAGE_MIN`.

### Fixed

- The backend test targets (`make test-<backend>`) crashed before running any test: the test configuration lost all defaults when a `-funnel-config` file was given, and lists from the file (e.g. `MongoDB.Addrs`) were appended to the defaults instead of replacing them. The MongoDB metrics test failed with the current Prometheus client library.
