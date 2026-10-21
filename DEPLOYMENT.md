<title>Funnel Deployment Guide</title>

# Funnel Deployment Guide

**Documentation version: 0.12.2.1 (unreleased)** — applies to funnel-gdi 0.12.2.1. For another version, open this file at the matching git tag (e.g. [`0.11.3.1`](https://github.com/CERIT-SC/funnel-gdi/tree/0.11.3.1)); every release on the [Releases](https://github.com/CERIT-SC/funnel-gdi/releases) page links to its docs.

This guide is also published, per version, at https://cerit-sc.github.io/funnel-gdi/. Run every command in this guide from the repository root.

## Contents

1. [Choose a deployment option](#choose-a-deployment-option)
2. [System requirements](#system-requirements)
3. [Supported and tested platforms](#supported-and-tested-platforms)
4. [Option 1: Local binary](#option-1-local-binary)
5. [Option 2: Kubernetes](#option-2-kubernetes)
6. [Option 3: Docker container (no Kubernetes)](#option-3-docker-container-no-kubernetes)
7. [Testing the server](#testing-the-server)
8. [Upgrade and rollback](#upgrade-and-rollback)
9. [Production checklist](#production-checklist)
10. [Troubleshooting](#troubleshooting)
11. [Troubleshooting for users](#troubleshooting-for-users)
12. [Getting help](#getting-help)

## Choose a deployment option

| | [Option 1: Local binary](#option-1-local-binary) | [Option 2: Kubernetes](#option-2-kubernetes) | [Option 3: Docker container](#option-3-docker-container-no-kubernetes) |
|---|---|---|---|
| **Use for** | Development | Production and shared testing | A single host without Kubernetes |
| **Server runs as** | Process on your machine | Deployment (Helm chart [`deploy-guide/kubernetes`](deploy-guide/kubernetes)) | Docker container |
| **Tasks run as** | Docker containers on the same machine | A worker Job + one executor Job per task step | Docker containers on the host (shared Docker socket) |
| **Task database** | BoltDB in `./funnel-work-dir` | BoltDB on a shared PVC | BoltDB in `./funnel-work-dir` |
| **Authentication by default** | None | HTTP Basic auth | None |
| **Tested for this release** | No | **Yes** | No |

## System requirements

### Minimum resources

Values for Kubernetes are the chart defaults in [`values.yaml`](deploy-guide/kubernetes/values.yaml). They are enough for testing; raise them for production load. Options 1 and 3 set no limits and use the resources of the host.

| Component | CPU request / limit | Memory request / limit | Ephemeral storage request / limit | Set in |
|---|---|---|---|---|
| Server pod | 100m / 2 | 4Gi / 4Gi | 25Gi / 25Gi | `values.yaml`: `funnel.resources` |
| Worker pod (one per task) | from the task\* / 1 | from the task\* / 1G | from the task\* / — | `files/funnel-server-config.yml` |
| Executor pod (one per task step) | from the task\* / 1 | from the task\* / 5G | from the task\* / — | `values.yaml`: `executor.limits` |

\* Requests come from the TES task's `resources` (`cpu_cores`, `ram_gb`, `disk_gb`). Without them a worker requests 100m CPU, 16M memory and 100M ephemeral storage.

**Limit on task resources.** Kubernetes rejects a pod whose request is higher than its limit, and both the worker and the executor pod request what the task asks for. With the chart defaults a task can therefore ask for at most `cpu_cores: 1` and `ram_gb: 1` (the worker limits; `ram_gb` and `disk_gb` are rounded to the nearest whole gigabyte, halves to even: `ram_gb: 1.5` becomes a request of `2G`, `2.5` becomes `2G`, and a value below `0.5` becomes `0G`); a task asking for more ends in `SYSTEM_ERROR` because its Job cannot be created. To run bigger tasks, raise the worker limits in `files/funnel-server-config.yml` (`WorkerTemplate`) and `executor.limits` in your values.

### Storage

| What | Option 2 (Kubernetes) | Options 1 and 3 |
|---|---|---|
| Task database (BoltDB) | Shared PVC `pvc.name`, default size 2Gi (`pvc.size`) | `./funnel-work-dir` on the host |
| Working copies of task inputs and outputs | The same shared PVC in `pvcMode: shared` — size it for your data (see [Per-task storage modes](#per-task-storage-modes)). Deleted when the task ends. | `./funnel-work-dir` on the host |
| Task outputs (kept after the task) | S3-compatible storage (`s3.*` in your values). `file://` output URLs do not work: the worker only allows paths under `/opt/funnel` in its own pod, and those are deleted with the task or the pod. Without S3, tasks can only return their results in the executor `stdout`/`stderr` logs (FULL view, at most 10 KB each). | Local paths (`file://`) under a directory allowed by `LocalStorage.AllowedDirs` (default: the server's working directory) |
| StorageClass | Must support `ReadWriteMany` (e.g. `nfs-csi`) | — |
| Extra for `pvcMode: full` | S3 Mountpoint CSI driver, permission to create a ClusterRole | — |

### Network

| Port | Protocol | Used for |
|---|---|---|
| `8000` | HTTP | REST API (TES), web dashboard, `/healthz` |
| `9090` | gRPC | Worker → server communication, CLI |

### Software

| Tool | Option 1 | Option 2 | Option 3 | Minimum version |
|---|---|---|---|---|
| Go | required | — | — | 1.26 (`go.mod`) |
| make, git | required | git, to get the chart | required | any recent |
| Docker | required | only to build your own image | required | Engine with BuildKit (default since 23.0; the `Dockerfile` uses `RUN --mount`) |
| kubectl | — | required | — | within ±1 minor version of the cluster ([skew policy](https://kubernetes.io/releases/version-skew-policy/)) |
| Helm | — | required | — | 3.x or 4.x (chart `apiVersion: v2`) |
| Node.js + npm | only to rebuild the web dashboard (`cd webdash && npm ci`, then `make webdash`) | — | — | — (the built dashboard is committed in `webdash/web.go`) |

## Supported and tested platforms

**Tested manually for this release**

| What | Versions | Date |
|---|---|---|
| Kubernetes, Option 2 with `deploy-guide/kubernetes`, `pvcMode: shared` | Kubernetes v1.36.4 (RKE2, CERIT `kuba-cluster`), Helm v4.3.0, StorageClass `nfs-csi-temporary` (NFS, RWX), image linux/amd64 | 2026-10-07 |
| Web dashboard (Option 1 server, headless browser via Playwright: task list, task detail, service info, no console errors) | Chromium 153.0.8010.12 (the engine of Chrome 153) | 2026-10-07 |

**Tested automatically by CI** ([`.github/workflows`](.github/workflows))

| What | Scope | When |
|---|---|---|
| Unit tests | All packages (`go test ./...`), with a coverage report and minimum | Every push |
| Backend component tests | Databases Postgres, MongoDB, Elasticsearch, Badger, DynamoDB (local), Datastore (emulator); events Kafka, Pub/Sub (emulator); storage FTP, S3-compatible (`AmazonS3` with a custom endpoint and `GenericS3`, against versitygw and Zenko CloudServer) | Every push |
| TES API compliance | TES 1.0.0 and 1.1.0, local compute, with BoltDB or Postgres and local or S3 storage (`GenericS3` against versitygw) | Pushes and pull requests to `master` |
| Workflow engine | Nextflow (nf-canary), local compute | Pushes and pull requests to `master` |
| Kubernetes | Option 2 chart `deploy-guide/kubernetes` with `pvcMode: shared` on a single-node kind cluster: a TES task, a task with an S3 output (`GenericS3` against versitygw), Nextflow nf-canary | Pushes and pull requests to `master` |

**Built, but not tested**

| What | Details |
|---|---|
| Container image `ghcr.io/cerit-sc/funnel-gdi` | linux/amd64, linux/arm64 |
| `pvcMode: pvc` and `pvcMode: full` | Unit tests only |
| Storage: HTSGET, SDA, Amazon S3 itself (AWS), HTTP(S) | HTSGET has unit tests; SDA, AWS and HTTP(S) have no automated tests. S3-compatible storage is tested in CI (above). |
| Prometheus metrics at `/metrics` | Tested only with the MongoDB backend (component tests) |

**Web dashboard (browser support)**

| | |
|---|---|
| Supported | **Chrome 64 or newer** (and other Chromium-based browsers of the same engine version). The bundled dashboard uses ECMAScript 2018 syntax (checked with `es-check`); newer browser APIs are used only behind feature checks. |
| Tested | Chromium 153, see the table above |
| Other browsers | Expected to work if they support ECMAScript 2018 (the build targets the browserslist `>0.2%, not dead, not op_mini all`), but not tested |
| Known issue | The *Nodes* page sends requests to `/v1/nodes` in an endless loop (an upstream bug in `webdash/src/Pages.js`). The task pages are not affected. With the `local` compute backend the Nodes page lists no nodes anyway. |

**Not supported** — inherited from upstream and not supported by the GDI fork (some have automated tests, see above)

| Area | Not supported |
|---|---|
| Compute | HTCondor, Slurm, PBS/Torque, Grid Engine, AWS Batch, GCP Batch |
| Storage | Google Cloud Storage, Swift, FTP |
| Databases | MongoDB, PostgreSQL, Badger, DynamoDB, Datastore, Elasticsearch |
| Event writers | Kafka, Google Cloud Pub/Sub |
| Funnel's own scheduler | `Compute: manual` with `funnel node` (see *Docs → Compute → Deploying a cluster*) |
| Deployment scripts | `install.sh` (installs upstream `calypr/funnel`), `deployments/gce`, `deployments/gce-cos`, `deployments/systemd` |
| Operating systems | Windows |

## Option 1: Local binary

**Requirements:** Go 1.26+, a running Docker daemon.

**1. Build the binary** (into the repository root):
```bash
make build
```
funnel-gdi releases publish container images only. `install.sh` in the repository root comes from upstream and installs upstream `calypr/funnel` releases, not funnel-gdi.

**2. Start the server:**
```bash
./funnel server run --config config/default-config.yaml
```
The default config uses local compute (tasks run as Docker containers on your machine), BoltDB, HTTP on `:8000` and gRPC on `:9090`, with no authentication. To change ports, database path or auth, copy `config/default-config.yaml`, edit it and pass your copy with `--config`.

**3.** Continue with [Testing the server](#testing-the-server).

## Option 2: Kubernetes

**Requirements:** access to a namespace, `kubectl`, Helm 3.x or 4.x, a `ReadWriteMany` StorageClass and an image the cluster can pull.

With the default `pvcMode: shared` the chart needs only **namespaced** permissions: ServiceAccount, Role, RoleBinding, PVC, Deployment, Service, Secret and Ingress.

**1. Find your namespace and StorageClass**
```bash
kubectl get namespaces
kubectl get storageclass
```
Most shared clusters assign you a namespace (use it as `<namespace>` below); if you may create one, run `kubectl create namespace <namespace>`. Pick a StorageClass that supports `ReadWriteMany` (e.g. `nfs-csi`).

**2. Choose the image**

| Image | When to use |
|---|---|
| `ghcr.io/cerit-sc/funnel-gdi:<version>` — public, linux/amd64 and arm64 ([tags](https://github.com/CERIT-SC/funnel-gdi/pkgs/container/funnel-gdi)) | A released version. Use the one matching `appVersion` in [`Chart.yaml`](deploy-guide/kubernetes/Chart.yaml); images older than 0.12.2.1 do not understand this chart's config format, so until 0.12.2.1 is released, build your own image. |
| Your own build (command below) | Unreleased code, including this version before its release |

```bash
docker buildx build --platform linux/amd64 -t <registry>/<registry-user>/funnel-gdi:<tag> -f Dockerfile --push .
```

For a private registry, create a pull secret and set `imagePullSecret: regcred` in your values:
```bash
kubectl create secret docker-registry regcred -n <namespace> \
  --docker-server=<registry> --docker-username=<registry-user> --docker-password=<token>
```

**3. Prepare your values**
```bash
cp deploy-guide/kubernetes/values.yaml my-values.yaml
```
Edit your copy and replace every `<placeholder>`:

| Key | Required | What to set |
|---|---|---|
| `image` | Yes | Release image or your own build (step 2) |
| `pvc.storageClass` | Yes | Your `ReadWriteMany` StorageClass |
| `basicauth.password` | Yes | A strong password; it also secures worker → server RPC |
| `ingress.host`, `ingress.annotations` | Yes, or set `ingress.enabled: false` | Host name and cert-manager issuer; without Ingress use port-forward |
| `imagePullSecret` | For private images | Name of the pull secret |
| `pvcMode`, `storageClassName` | No | See [Per-task storage modes](#per-task-storage-modes) |
| `oidc.*` | No | OIDC login (needed for SDA inputs) |
| `s3.*` | For task outputs; yes for `pvcMode: full` | S3 storage for task inputs and outputs — the only place where task outputs are kept after the task (see [Storage](#storage)) |
| `htsget.serviceurl`, `sda.serviceurl` | No | GA4GH input backends; empty = disabled |

**4. Check and install**
```bash
helm template my-funnel deploy-guide/kubernetes -n <namespace> -f my-values.yaml > /dev/null
helm install my-funnel deploy-guide/kubernetes -n <namespace> -f my-values.yaml
```
The first command is a dry run that fails early on an unknown `pvcMode`, on `pvcMode: pvc` without a StorageClass and on `pvcMode: full` without `s3.bucket` / `s3.region`. It does **not** detect `<placeholder>` values left in your copy: check that none remain (`grep '<' my-values.yaml`), otherwise the chart installs with, e.g., the literal password `<choose-a-strong-password>`. The second installs the ServiceAccount, RBAC, PVC, Deployment, Service, Ingress and config Secrets.

**5. Connect**
```bash
kubectl get pods -n <namespace>
kubectl port-forward -n <namespace> svc/my-funnel 8000:8000
```
Once the `my-funnel-…` pod is `Running`, port-forward (or use your Ingress host) and continue with [Testing the server](#testing-the-server). The API requires Basic auth: add `-u admin:<password>` to the `curl` commands. A running task shows a worker pod (`<task-id>-…`) and an executor pod (`<task-id>-0-…`).

**6. Change values later**
```bash
helm upgrade my-funnel deploy-guide/kubernetes -n <namespace> -f my-values.yaml
kubectl rollout restart deployment/my-funnel -n <namespace>
```
The restart is needed when only the config Secrets changed (same image).

**7. Uninstall**
```bash
helm uninstall my-funnel -n <namespace>
kubectl delete jobs -n <namespace> -l 'app in (funnel-worker,funnel-executor)'
```
`helm uninstall` also deletes the shared PVC with the task database (unless `pvc.existing: true`). The second command removes finished task Jobs; until their pods are gone, the PVC stays `Terminating`.

### Per-task storage modes

`pvcMode` (Funnel's `Kubernetes.PVCMode`) decides how a task's files are shared between its worker and executor pods.

| `pvcMode` | Created per task | Needs |
|---|---|---|
| `shared` (default) | Nothing. All tasks use `pvc.name`, isolated by `subPath`; executors also get a cross-task `/shared` directory. | Namespaced RBAC only |
| `pvc` | A PVC from `storageClassName` (falls back to `pvc.storageClass`), deleted with the task | Namespaced RBAC only, a `ReadWriteMany` StorageClass |
| `full` | An S3-backed PV (Mountpoint CSI) + PVC — upstream behaviour | `s3.bucket` / `s3.region`, the S3 CSI driver, permission to create a ClusterRole for `persistentvolumes` |

Tasks without inputs, outputs or volumes get no PVC in `pvc` and `full` mode (an `emptyDir` is used instead).

### Config format

Funnel parses its config strictly (Protobuf); a wrong key or type stops the server at startup. The chart's configs live in `deploy-guide/kubernetes/files/`.

| Rule | Correct | Wrong |
|---|---|---|
| No unknown keys | Keys defined in `config/config.proto` | Typos, keys from older versions |
| Types must match | `RPCPort: "9090"` | `RPCPort: 9090` |
| Durations in seconds | `300s` | `5m` |
| Timeouts as messages | `Timeout: {duration: 30s}` | `Timeout: 30s` |
| RPC credentials | Under `RPCClient.Credential` | Directly under `RPCClient` |

## Option 3: Docker container (no Kubernetes)

**Requirements:** a running Docker daemon.

**1. Build the image.** It includes the Docker CLI (no daemon), so it can drive the host's Docker socket:
```bash
docker build -t funnel-gdi:dind -f Dockerfile.dind .
```

**2. Create the work directory** on the host:
```bash
mkdir -p funnel-work-dir
```

**3. Start the server** (on re-runs, first run `docker rm -f funnel-dind`):
```bash
docker run -d --name funnel-dind \
  -p 8000:8000 -p 9090:9090 \
  -w "$(pwd)" \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v "$(pwd)/funnel-work-dir:$(pwd)/funnel-work-dir" \
  -v "$(pwd)/config/default-config.yaml:/opt/funnel/config.yml:ro" \
  funnel-gdi:dind server run --config /opt/funnel/config.yml
```

| Flag | Why |
|---|---|
| `-p 8000:8000 -p 9090:9090` | Exposes the HTTP and gRPC ports on the host |
| `-w "$(pwd)"` | Same working directory as on the host, so relative paths in the config resolve the same way |
| `-v /var/run/docker.sock:...` | Shares the host's Docker daemon, so the server can start task containers on the host |
| `-v "$(pwd)/funnel-work-dir:..."` | Mounts the work directory at the identical path in the container (required for the shared socket) |
| `-v ".../default-config.yaml:...:ro"` | Mounts the config read-only. For your own config replace this line; use absolute paths matching the `Worker.WorkDir` mount. |

**4.** Continue with [Testing the server](#testing-the-server).

## Testing the server

The same for all options; only host and port change. On Kubernetes, add `-u admin:<password>` to every `curl`.

| Step | Command | Expected result |
|---|---|---|
| 1. Health check | `curl -i http://localhost:8000/healthz` | `HTTP/1.1 200 OK` |
| 2. Server info | `curl -s http://localhost:8000/v1/service-info` | JSON with the server name and `version` |
| 3. Submit a task | see the command below | `{"id": "<task-id>"}` |
| 4. Task result | `curl -s "http://localhost:8000/v1/tasks/<task-id>?view=FULL"` | `"state": "COMPLETE"`, executor `stdout` is `hello world` |
| 5. Web dashboard | Open `http://localhost:8000` in a browser | The task is listed |

```bash
curl -s -X POST http://localhost:8000/v1/tasks \
  -d '{"name":"Hello world","executors":[{"image":"alpine","command":["echo","hello world"]}]}'
```

## Upgrade and rollback

Read the release notes on the [Releases](https://github.com/CERIT-SC/funnel-gdi/releases) page between your version and the new one first — a new upstream base can change the config format.

| Action | Kubernetes | Local binary / Docker |
|---|---|---|
| Upgrade | `git checkout <version>`, then `helm upgrade my-funnel deploy-guide/kubernetes -n <namespace> -f my-values.yaml --set image=ghcr.io/cerit-sc/funnel-gdi:<version>` | Stop the server, `git checkout <version>`, rebuild (`make build` or `docker build`), start it with the same config and work directory |
| List versions | `helm history my-funnel -n <namespace>` | `git tag` |
| Roll back | `helm rollback my-funnel <revision> -n <namespace>` (chart, values and image together) | Same as upgrade, with the previous tag |
| Task database | Kept on the shared PVC | Kept in `./funnel-work-dir` |

Upgrade from a checkout of the target version, so that the chart matches the image.

### Database migrations

A new version may need to change how the task database stores its data. Funnel does this itself when the server starts; there is nothing to run by hand.

| Database | What happens on upgrade |
|---|---|
| BoltDB (default) | Nothing to migrate: missing buckets are created on start, and tasks are stored as Protobuf messages, which newer versions read |
| PostgreSQL | The server applies the SQL files in [`database/postgres/migrations/`](database/postgres/migrations) that the database does not have yet, in order of their number, each in its own transaction, and records them in the `schema_version` table. A failing migration is rolled back and the server does not start (the error names the file). |

**Back up the database before an upgrade.** A rollback does not undo migrations: a server refuses to start on a PostgreSQL database that a newer Funnel has migrated (`the database schema (version N) is newer than this Funnel supports`). Roll back by restoring the backup together with the old version.

| Database | Back up | Restore |
|---|---|---|
| BoltDB on Kubernetes | Stop the server (`kubectl scale deploy/my-funnel -n <namespace> --replicas=0`), copy `funnel.bolt.db` from the shared PVC with a helper pod (below), start the server again (`--replicas=1`) | The same, copying the file back |
| BoltDB, local binary / Docker | Stop the server, copy `./funnel-work-dir/funnel.db` | Stop the server, copy the file back |
| PostgreSQL | `pg_dump -Fc -h <host> -U <user> <database> > funnel.dump` | `pg_restore --clean -h <host> -U <user> -d <database> funnel.dump` |

Helper pod for the shared PVC (`pvc.name`, default `storage`):
```bash
kubectl run funnel-backup -n <namespace> --image=busybox:1.36 --restart=Never --overrides='
{"spec": {"containers": [{"name": "backup", "image": "busybox:1.36", "command": ["sleep", "600"],
  "volumeMounts": [{"name": "storage", "mountPath": "/data"}]}],
  "volumes": [{"name": "storage", "persistentVolumeClaim": {"claimName": "storage"}}]}}'
kubectl wait -n <namespace> --for=condition=Ready pod/funnel-backup
kubectl cp <namespace>/funnel-backup:/data/funnel.bolt.db ./funnel.bolt.db   # restore: swap the two paths
kubectl delete pod -n <namespace> funnel-backup
```

## Production checklist

| Area | What to do |
|---|---|
| Authentication | Set a strong `basicauth.password`. For user logins and SDA inputs, enable `oidc`. Options 1 and 3: the default config has none. In your copy of [`config/default-config.yaml`](config/default-config.yaml), add a user under `Server.BasicAuth` **and** the same user and password under `RPCClient.Credential`: the worker reads the task and reports its progress over RPC with these credentials, and without them every task stays `QUEUED`. `OidcAuth` can be added on top for user logins, but never replaces `BasicAuth`, because workers authenticate only with Basic credentials. |
| Exposure | The chart's Ingress requests a TLS certificate through cert-manager (`ingress.annotations`). Without an Ingress controller and cert-manager, set `ingress.enabled: false` and use `kubectl port-forward`. |
| Task visibility | The chart does not set `Server.TaskAccess`, so it is `All`: every authenticated user can list and read all tasks. With OIDC, set `TaskAccess: OwnerOrAdmin` (and `Admins`) under `Server` in `deploy-guide/kubernetes/files/funnel-server-config.yml`. This matters for SDA and HTSGET inputs: the user's bearer token is stored in the input URL and is visible in the task and its logs (see *Docs → Storage → SDA / Htsget* on the documentation website). |
| Shared directory | In `pvcMode: shared` every executor mounts the same writable `/shared` directory, so any task can read and change what other tasks (also of other users) leave there. Tasks must not put sensitive data there — e.g. decrypted SDA or HTSGET inputs. If users must not share data, use `pvcMode: pvc` or a separate deployment per project. |
| Task images | Executor pods run as user `1000` with `runAsNonRoot` and all capabilities dropped (`funnel.runAsUser`). Task images that need root do not work. Options 1 and 3 run executor containers with `docker run --read-only --tmpfs /tmp` (`Worker.Container.RunCommand`): a task can write only to its declared inputs, outputs and volumes and to `/tmp`, which is not shared between executors unless the task declares a volume at `/tmp`. |
| Database | BoltDB supports one server replica only. The chart runs `replicas: 1` with the `Recreate` strategy, so `helm upgrade` and `kubectl rollout restart` stop the old server pod before the new one opens the database: the API is unavailable until the new pod starts. Workers of running tasks retry their calls to the server (`RPCClient.MaxRetries: 3` in the worker config, back-off starting at 5 s, about 35 s in total), so a short restart does not interrupt them; keep upgrades shorter than that or run them when no tasks are running. |
| Backups | `helm uninstall` deletes the shared PVC. Keep data you need on a PVC you manage yourself (`pvc.existing: true`). |
| Image | Pin a released version, not a moving tag such as `latest`. |
| Audit log | Every API request, including rejected authentication, is logged with `msg=AUDIT`, user ID, method, result code and task ID — see them with `kubectl logs -n <namespace> deploy/my-funnel \| grep AUDIT`. Forward these logs to your log storage if you need to keep them. |

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `Forbidden` on `helm install` | Your account lacks namespaced RBAC rights, or `pvcMode: full` needs a ClusterRole | Set `rbac.create: false` and have an admin create the ServiceAccount, Role and RoleBinding; or use `pvcMode: shared` or `pvc` |
| `ImagePullBackOff: pull access denied` | Private image without a pull secret | Set `imagePullSecret` to an existing `kubernetes.io/dockerconfigjson` Secret ([Option 2](#option-2-kubernetes), step 2) |
| Server `CrashLoopBackOff` with `failed to unmarshal JSON with protojson` | Wrong config key or value type | See [Config format](#config-format) |
| Task stays `QUEUED` or ends in `SYSTEM_ERROR` | A worker Job or PVC could not be created | `kubectl get jobs,pods -n <namespace>`, `kubectl logs job/<task-id> -n <namespace>` and the server log `kubectl logs deploy/my-funnel -n <namespace>` |
| PVC stays `Pending` | The StorageClass is not a working `ReadWriteMany` class | Fix `pvc.storageClass` / `storageClassName` |
| PVC stays `Terminating` after uninstall | Finished task pods still reference it | `kubectl delete jobs -n <namespace> -l 'app in (funnel-worker,funnel-executor)'` |
| `401 Unauthorized` from the API | Basic auth is enabled (Kubernetes) | Add `-u admin:<password>` to `curl` |

The [Troubleshooting for users](#troubleshooting-for-users) section below covers problems of individual tasks.

## Troubleshooting for users

This section is for people who submit tasks to a running Funnel server. The examples use `curl` against `http://localhost:8000`; add `-u <user>:<password>` when the server uses Basic authentication. The web dashboard (the server's root URL in a browser) shows the same information. Error messages that start with a code (`FUNNEL-…`) are explained under *Docs → Error codes* on the documentation website.

### Task states

| State | Meaning | What to do |
|---|---|---|
| `QUEUED` | Accepted, waiting for a worker | Wait. If it stays queued for minutes, the server cannot start workers: ask the operator ([Troubleshooting](#troubleshooting)) |
| `INITIALIZING` | A worker is downloading inputs and pulling images | Wait; large inputs and images take time |
| `RUNNING` | The executors are running | Wait, or cancel with `curl -X POST http://localhost:8000/v1/tasks/<task-id>:cancel` |
| `COMPLETE` | All executors exited with code 0 and the outputs were uploaded | – |
| `EXECUTOR_ERROR` | An executor exited with a non-zero code: the task itself failed | Read the executor's `stderr` and `exit_code` (below) |
| `SYSTEM_ERROR` | Funnel could not run the task: an input could not be downloaded, an output could not be uploaded, the image could not be pulled, or the worker failed | Read the `system_logs` (below) |
| `CANCELING`, `CANCELED` | The task is being canceled / was canceled | – |

### Reading the logs of a task

```bash
curl -s "http://localhost:8000/v1/tasks/<task-id>?view=FULL"
```

- `logs[].logs[]` holds one entry per executor with its `exit_code`, `stdout` and `stderr`. Only the first 10 KB of each are kept, so write large logs to an output file.
- `logs[].system_logs` holds Funnel's own messages about the task: downloads, uploads, image pulls and errors.
- Without `?view=FULL` (the default `MINIMAL` view) only the ID and state are returned.

### Common problems

| Symptom | Cause | Fix |
|---|---|---|
| `400 Bad Request` with `Task.Executors[0].Image: required, but empty` (or another `Task.…` message) | The task document is invalid; every problem is listed | Fix the named field. Each executor needs `image` and `command`; paths (`path`, `workdir`, `stdin`, `stdout`, `stderr`, volumes) must be absolute |
| `400 Bad Request` with `… is not allowed as an input path` (or output, volume, working directory) | The path is reserved by the server, e.g. a system directory | Use a path under your own directory such as `/data` |
| `401 Unauthorized`: `Basic/Bearer authorization token missing` or `Basic-authentication failed` | No or wrong credentials | Send `-u <user>:<password>`, or `Authorization: Bearer <token>` when the server uses OIDC |
| `401 Unauthorized`: `Bearer authorization token not accepted` | The OIDC token expired or is for another service | Get a new token |
| `404 Not Found`: `task not found: taskID: …` | Wrong task ID | Check the ID with `curl "http://localhost:8000/v1/tasks?view=MINIMAL"` |
| `permission denied` | The task belongs to another user, and the server only shows tasks to their owners (`Server.TaskAccess`) | Use the account that created the task, or ask an administrator |
| `EXECUTOR_ERROR`, `exit_code` 127 or `stderr` says `not found` | The command does not exist in the image | Use an image that contains the program, or the full path to it |
| `EXECUTOR_ERROR` with `Permission denied` when writing a file | On Kubernetes, executors run as a non-root user and only the task's inputs, outputs, volumes and `/tmp` are writable | Write to an output path or a declared volume; use an image that works without root |
| `SYSTEM_ERROR`, `system_logs` mention the input URL | An input URL is unreachable, misspelled, or needs credentials the server does not have | Open the URL yourself; for S3 or other storage, ask the operator which storage the server can read |
| `SYSTEM_ERROR`, or the task stays `INITIALIZING`; the logs mention pulling the image | The image does not exist or is private | Check the image name and tag; private images need the operator to configure access |
| `COMPLETE`, but the output is missing | `file://` outputs on Kubernetes are deleted with the task | Use S3 (`s3://…`) output URLs ([Storage](#storage)) |

## Getting help

- **Questions and bugs:** open an issue at https://github.com/CERIT-SC/funnel-gdi/issues/new/choose and use the matching template.
- **Include:** the Funnel version (`curl http://localhost:8000/v1/service-info`, field `version`), the task ID, the state, and the relevant part of `?view=FULL` (see [Reading the logs of a task](#reading-the-logs-of-a-task)). **Remove passwords, tokens and signed URLs before you post**: input URLs and logs may contain them.
- **Security vulnerabilities:** do not open a public issue; follow [SECURITY.md](SECURITY.md).
- **Operators:** for problems with the installation itself, see [Troubleshooting](#troubleshooting).
