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

- Error codes: API errors can carry a stable code (`FUNNEL-<AREA>-<NNN>: <message>`), with the messages kept in a catalog file (`errcodes/messages.en.yaml`) ready for translation. Reference page *Docs → Error codes* on the website. Authentication and access errors use the codes `FUNNEL-AUTH-001`…`008`, task request errors `FUNNEL-TASK-001`…`006`, and every task validation problem its own `FUNNEL-VAL-001`…`020`.
- Postgres database: the schema is created and updated by versioned SQL migrations (`database/postgres/migrations/`), applied in order on server start and recorded in a `schema_version` table. Existing databases are adopted without changes. A server refuses to start on a database migrated by a newer Funnel.
- Deployment guide: "Database migrations" (what happens to BoltDB and PostgreSQL on upgrade, how to back up and restore before an upgrade or rollback). `install.sh` and the `deployments/` scripts are marked as upstream and not supported.
- "Troubleshooting for users" in the deployment guide: what each task state means, how to read a task's logs, and common problems with their causes and fixes. "Getting help" explains where to ask (GitHub issues with templates for bugs and questions) and what to include.
- CI reports the statement coverage of the unit and component tests per package (job summary and the `coverage` artifact with an HTML report) and fails when it drops below the minimum `COVERAGE_MIN`. Run `make test-coverage` for the same report locally.
- CI runs the component tests against real databases, event streams and storage (Postgres, MongoDB, Elasticsearch, Badger, DynamoDB local, Datastore and Pub/Sub emulators, Kafka, S3-compatible storage, FTP); their coverage counts toward the total.
- Acceptance tests with coverage: the `Acceptance Tests` workflow builds Funnel with coverage instrumentation (`make build-cover`), runs the TES compliance suite, the Nextflow nf-canary workflow and the Kubernetes integration tests against it, reports the merged coverage per package and fails below `ACCEPTANCE_COVERAGE_MIN`.
- Kubernetes integration tests run the GDI Helm chart (`deploy-guide/kubernetes`) on a kind cluster on every push and pull request to `master` (before: the upstream chart, only on the unused `develop` branch).
- The TES compliance suite also runs with the Postgres database and with S3 storage.
- Helm chart values `imagePullPolicy` (default `Always`) and `funnel.extraEnv` (extra environment variables of the server container).

### Fixed

- A server with OIDC authentication (`Server.OidcAuth`, chart value `oidc.enabled`) crashed at start-up (nil OAuth2 configuration, an upstream regression).
- Reading or canceling another user's task (with `Server.TaskAccess: Owner` or `OwnerOrAdmin`) returned HTTP 500 instead of 403 Forbidden.
- Task IDs from requests are shortened (256 bytes) in error messages.
- The OIDC login echoed the identity provider's error description without a length limit; it is now shortened.
- The backend test targets (`make test-<backend>`) crashed before running any test: the test configuration lost all defaults when a `-funnel-config` file was given, and lists from the file (e.g. `MongoDB.Addrs`) were appended to the defaults instead of replacing them. The MongoDB metrics test failed with the current Prometheus client library. A storage backend could not be enabled by a test config file (`Disabled: false`), so the S3 and FTP storage tests were skipped.
- The FTP test server image is built from `tests/ftp-test-server/Dockerfile`; the published image is no longer available.
- S3 tests use versitygw (`make start-s3`) because MinIO no longer publishes its images; `tests/amazoncli-minio-s3.config.yml` was updated to the current `AmazonS3.AWSConfig` keys.
