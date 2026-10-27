# GDI fork features (not in upstream `calypr/funnel`)

This fork tracks `calypr/funnel` `develop` and carries the following additions
on top of it. Each one is self-contained so it can be offered upstream as a
separate PR. Last synced with upstream: `eff51273` (2026-09-02, v0.12.x).

| Feature | Files | Tests | Upstream PR readiness |
|---|---|---|---|
| **Namespaced-RBAC-friendly cleanup**: task/orphan cleanup only touches PersistentVolumes in `full` mode and ServiceAccounts only when Funnel creates them (`ServiceAccountTemplate` / `_WORKER_SA`) | `compute/kubernetes/backend.go` (`cleanResources`, `CleanOrphanedResources`) | `compute/kubernetes/pvc_mode_test.go` (`TestNamespacedRBAC_NoClusterOrSACalls`) | Ready; fixes `Forbidden` errors on namespaced deployments. |
| **Per-task storage modes** `Kubernetes.PVCMode` = `full` \| `pvc` \| `shared` (+ `StorageClassName`, `SharedPVCName`, `WorkerImage`) | `config/config.proto`, `config/kubernetes_pvc.go`, `compute/kubernetes/backend.go`, `compute/kubernetes/resources/{job,pvc}.go`, `config/kubernetes/{worker-job,executor-job,worker-pvc}.yaml`, `worker/{kubernetes,worker}.go`, `cmd/worker/run.go` | `config/kubernetes_pvc_test.go`, `compute/kubernetes/pvc_mode_test.go`, `worker/kubernetes_pvc_test.go` | Ready. `full` keeps upstream behaviour (default). |
| **HTSGET storage** (`htsget://{reads\|variants}/…`) | `storage/htsget.go`, `storage/htsget/`, `config/config.proto` (`HTSGETStorage`) | `storage/htsget_test.go` | Needs the token-in-URL question settled (see below). |
| **SDA storage** (`sda://dataset/path`) | `storage/sda.go`, `config/config.proto` (`SDAStorage`) | – | Needs tests + token-in-URL question settled. |
| **Crypt4GH decryption** + key management (`C4GH_*` env) | `storage/crypt4gh/` | `storage/crypt4gh/*_test.go` | Consider `neicnordic/crypt4gh` instead of the own implementation. |
| **Bearer token propagation** to `sda://` / `htsget://` inputs | `server/tes.go` (`ReplaceInputBearerToken`) | `server/tes_token_test.go` | Token is stored in the input URL (DB/API/UI) → redact or move to plugin credentials first. |
| **Audit log** (`msg=AUDIT`, `userID`, `method`, `code`, `taskID`) incl. rejected auth | `server/audit.go`, `server/auth.go` (`GetUserID`, `logRejected`), `server/server.go` | `server/audit_test.go` | Ready; maybe make it configurable. |
| **Panic recovery** gRPC interceptor | `server/server.go` | `server/server_test.go` (500 mapping) | Ready (1 line). |
| **Bearer token in CLI/`tes.Client`** (`FUNNEL_SERVER_TOKEN`) | `tes/client.go` | `tes/client_auth_test.go` | Ready. |
| **GenericS3 `isDir` fix** (`key+"/"` prefix; `x` is not a dir just because `x.bak` exists) | `storage/generic_s3.go` | – (needs MinIO) | Ready, add a unit test. |
| S3 test server versitygw (`make start-s3`) instead of MinIO, whose images are no longer published; `tests/amazoncli-minio-s3.config.yml` uses the current `AWSConfig` keys | `Makefile`, `.github/workflows/compliance.yaml`, `tests/*-s3.config.yml` | `make start-generic-s3 test-generic-s3` | Ready. |

GDI-only (not meant for upstream):

- `AGENTS.md` – commands, verification chain and invariants for coding agents; `make lint` runs golangci-lint v2 with `.golangci.yml` and the `go.mod` Go version, as CI does.
- Coverage tooling: `scripts/coverage/` (merges Go coverage profiles, leaves out generated code and `tests/` helpers, per-package report, `-min` gate; tests in `scripts/coverage/main_test.go`), `make test-coverage`; `.github/workflows/tests.yaml` runs the backend component tests (`COVER_DIR` on the `test-<backend>` Makefile targets), publishes the merged coverage report (job summary, artifact `coverage`) and enforces `COVERAGE_MIN`. Acceptance tests: `.github/workflows/acceptance.yaml` builds a coverage-instrumented binary (`make build-cover`, `build.yaml` input `cover`), runs `compliance.yaml`, `nextflow.yaml`, `integration-tests.yaml` (the GDI chart on kind, S3 via versitygw) and `scenarios.yaml` (`tests/acceptance/scenarios.sh`: CLI, terminal dashboard, scheduler mode with a node, Slurm, PBS, Grid Engine and HTCondor with fake commands, basic authentication, standalone worker, S3/FTP/HTTP storage) against it, merges the coverage (artifact `acceptance-coverage`) and enforces `ACCEPTANCE_COVERAGE_MIN`.
- `Dockerfile.dind` – image with Docker CLI for the local/manual worker with the Docker executor.
- `.github/workflows/git-tag.yaml` – publishes `ghcr.io/<repo>:<tag>` on git tags / manual dispatch; `workflow_dispatch` on CI workflows.
- `deploy-guide/kubernetes/` + `DEPLOYMENT.md` – Helm chart with `pvcMode` shared (default) / pvc / full; namespaced RBAC only except for `full`; `imagePullPolicy` and `funnel.extraEnv` values (used by the CI Kubernetes test).
- Docs: `website/content/docs/storage/{htsget,sda}.md`, PVC modes section in `website/content/docs/compute/kubernetes.md`.
- Documentation website (https://cerit-sc.github.io/funnel-gdi/, one copy per release + `latest/` + `dev/`): `.github/workflows/pages.yaml`, `website/scripts/` (`sync-gdi-docs.py` generates the "GDI fork" pages from the root documents, `build-versions.sh`, `check-links.py`), `website/content/gdi.md`, `website/content/download.md`, `website/layouts/partials/{version-bar,page-banner}.html`, `website/layouts/shortcodes/gdi-version.html`, `website/go.mod` (pinned Hugo modules), GDI header/home page in `website/layouts/`, `website/config.yaml` (`baseURL`, menu, `params`), `gdi` / `gdi_support` front matter on upstream pages.
- Postgres schema migrations: `database/postgres/migrations/*.sql` (human-readable, applied in order and recorded in `schema_version`), `database/postgres/migrate.go`; tests in `database/postgres/migrate_test.go`.
- Error codes: package `errcodes` (`errcodes.Status` / `errcodes.Message`, messages in `errcodes/messages.en.yaml`), reference page `website/content/docs/error-codes.md`; tests in `errcodes/errcodes_test.go` (catalog validation, page in sync with the catalog, every code used in the code is defined).
- End-user support: "Troubleshooting for users" and "Getting help" in `DEPLOYMENT.md` (linked from `website/content/gdi.md` and `website/content/docs/tasks.md`), issue templates `.github/ISSUE_TEMPLATE/{bug_report,question,config}.yml`.
- Release and documentation process: `RELEASING.md` (release policy and checklist), `scripts/dashboard-smoke-test.mjs` (browser test of the web dashboard), `scripts/check-docs-version.sh` (run by `git-tag.yaml` for `X.Y.Z.N` tags, which also creates the GitHub Release), `.github/workflows/docs.yaml` (website build + link check on PRs to `master`), `.github/pull_request_template.md`. `CHANGELOG.md` stays the upstream changelog.

## Keeping in sync with upstream

- Config is Protobuf (`config/config.proto`), parsed strictly. GDI fields use
  numbers `HTSGETStorage = 34`, `SDAStorage = 35` (in `Config`) and
  `PVCMode = 21` … `WorkerImage = 24` (in `Kubernetes`). Check for collisions
  on every upstream merge.
- Regenerate after proto/template changes:
  - `protoc -I . --go_out=paths=source_relative:. config/config.proto` (protoc-gen-go v1.36.12);
    `config/config.swagger.json` is generated by `buf generate` and is not updated for the GDI fields.
  - `go-bindata -pkg config -o config/internal/bundle.go $(find config/ -name '*.txt' -o -name '*.yaml') && gofmt -w -s config/internal/bundle.go`
