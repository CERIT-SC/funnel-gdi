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
- CI reports the statement coverage of the unit and component tests per package (job summary and the `coverage` artifact with an HTML report) and fails when it drops below the minimum `COVERAGE_MIN` (75%). Run `make test-coverage` for the same report locally.
- CI runs the component tests against real databases, event streams and storage (Postgres, MongoDB, Elasticsearch, Badger, DynamoDB local, Datastore and Pub/Sub emulators, Kafka, S3-compatible storage, FTP); their coverage counts toward the total.
- Acceptance tests with coverage: the `Acceptance Tests` workflow builds Funnel with coverage instrumentation (`make build-cover`), runs the TES compliance suite, the Nextflow nf-canary workflow and the Kubernetes integration tests against it, reports the merged coverage per package and fails below `ACCEPTANCE_COVERAGE_MIN`.
- Kubernetes integration tests run the GDI Helm chart (`deploy-guide/kubernetes`) on a kind cluster on every push and pull request to `master` (before: the upstream chart, only on the unused `develop` branch).
- The TES compliance suite also runs with the Postgres database and with S3 storage, and once each with the Badger, MongoDB, Elasticsearch, DynamoDB and Datastore databases.
- Helm chart values `imagePullPolicy` (default `Always`) and `funnel.extraEnv` (extra environment variables of the server container).

### Fixed

- **Security** — PostgreSQL database: the authenticated user was dropped from the request context, so every task was stored with the owner `SYSTEM` and access was checked as the internal system user. `Server.TaskAccess: Owner` and `OwnerOrAdmin` therefore did not restrict anything: every user could read, list and cancel all tasks. Tasks created before this fix keep the owner `SYSTEM`: with `OwnerOrAdmin` only administrators see them, with `Owner` no user does (`UPDATE tasks SET owner = '<user>' WHERE owner = 'SYSTEM'` reassigns them).
- PostgreSQL database: the MINIMAL view (also the default when no view is given) returned the whole task, the `state` filter of `ListTasks` never matched, task metadata events were lost (and values with `"` failed), and system logs of a later attempt were appended to the first attempt's.
- Elasticsearch database: a task or its new state was not visible to `ListTasks` until the next index refresh (up to a second).
- FTP storage: a file could not be found on servers with MLST (e.g. current pure-ftpd), a directory with a single file was taken for that file on servers without MLST, and uploading to the root directory failed on some servers.
- DynamoDB database: reading a task without a view (the TES default, MINIMAL; also used by `CancelTask`) failed with HTTP 500 (nil pointer), and `ListTasks` ignored the state, tag and owner filters, so `Server.TaskAccess: Owner` listed all users' tasks.
- Lists in a configuration file (`--config`), e.g. `MongoDB.Addrs` or `EventWriters`, were appended to the default lists instead of replacing them, so the server with MongoDB connected to the default address `localhost` and writers from the defaults ran twice.
- Badger, MongoDB, Elasticsearch, Datastore and DynamoDB databases: `GetTask` and `ListTasks` without a view returned the whole task instead of the MINIMAL view (the TES default), and an unknown view was not rejected (as with BoltDB). `ListTasks` ignored the `name_prefix` filter with Badger, Elasticsearch, DynamoDB and Datastore; with MongoDB, characters such as `.` in the prefix were taken as a regular expression.
- BoltDB database: `ListTasks` returned the BASIC view instead of MINIMAL when filtering by tags, and accepted an unknown view.
- An executor's `stdin` file was truncated before the executor started (it was created instead of opened), so tasks with `stdin` always read empty input.
- A server with OIDC authentication (`Server.OidcAuth`, chart value `oidc.enabled`) crashed at start-up (nil OAuth2 configuration, an upstream regression).
- Reading or canceling another user's task (with `Server.TaskAccess: Owner` or `OwnerOrAdmin`) returned HTTP 500 instead of 403 Forbidden.
- Task IDs from requests are shortened (256 bytes) in error messages.
- The OIDC login echoed the identity provider's error description without a length limit; it is now shortened.
- The backend test targets (`make test-<backend>`) crashed before running any test: the test configuration lost all defaults when a `-funnel-config` file was given, and lists from the file (e.g. `MongoDB.Addrs`) were appended to the defaults instead of replacing them. The MongoDB metrics test failed with the current Prometheus client library. A storage backend could not be enabled by a test config file (`Disabled: false`), so the S3 and FTP storage tests were skipped.
- The FTP test server image is built from `tests/ftp-test-server/Dockerfile`; the published image is no longer available.
- S3 tests use versitygw (`make start-s3`) because MinIO no longer publishes its images; `tests/amazoncli-minio-s3.config.yml` and `tests/dynamo.config.yml` were updated to the current `AWSConfig` keys. The Kafka tests use the `apache/kafka` image (the old `spotify/kafka` image does not work with the current client), and the FTP test server publishes its whole passive port range. nf-canary is pinned to a commit in the Nextflow and Kubernetes integration tests, which no longer replace its parameter defaults. The backend tests use their own PostgreSQL database per test, as for the other databases.
