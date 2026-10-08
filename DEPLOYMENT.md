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
| Unit tests | All packages (`go test ./...`) | Every push |
| TES API compliance | TES 1.0.0 and 1.1.0, with BoltDB, local compute, local storage | Every push |
| Workflow engine | Nextflow | Pushes to `master` |

**Built, but not tested**

| What | Details |
|---|---|
| Container image `ghcr.io/cerit-sc/funnel-gdi` | linux/amd64, linux/arm64 |
| `pvcMode: pvc` and `pvcMode: full` | Unit tests only |
| Storage: HTSGET, SDA, Amazon S3 (`AmazonS3`), other S3 providers (`GenericS3`), HTTP(S) | HTSGET has unit tests; SDA, S3 and HTTP(S) have no automated tests |
| Prometheus metrics at `/metrics` | Not tested |

**Web dashboard (browser support)**

| | |
|---|---|
| Supported | **Chrome 64 or newer** (and other Chromium-based browsers of the same engine version). The bundled dashboard uses ECMAScript 2018 syntax (checked with `es-check`); newer browser APIs are used only behind feature checks. |
| Tested | Chromium 153, see the table above |
| Other browsers | Expected to work if they support ECMAScript 2018 (the build targets the browserslist `>0.2%, not dead, not op_mini all`), but not tested |
| Known issue | The *Nodes* page sends requests to `/v1/nodes` in an endless loop (an upstream bug in `webdash/src/Pages.js`). The task pages are not affected. With the `local` compute backend the Nodes page lists no nodes anyway. |

**Not supported** — inherited from upstream, never tested in the GDI fork

| Area | Not supported |
|---|---|
| Compute | HTCondor, Slurm, PBS/Torque, Grid Engine, AWS Batch, GCP Batch |
| Storage | Google Cloud Storage, Swift, FTP |
| Databases | MongoDB, PostgreSQL, Badger, DynamoDB, Datastore, Elasticsearch |
| Event writers | Kafka, Google Cloud Pub/Sub |
| Funnel's own scheduler | `Compute: manual` with `funnel node` (see *Docs → Compute → Deploying a cluster*) |
| Operating systems | Windows |

## Option 1: Local binary

**Requirements:** Go 1.26+, a running Docker daemon.

**1. Build the binary** (into the repository root):
```bash
make build
```

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
