<title>Funnel Deployment Guide</title>

# Funnel Deployment Guide

Three ways to run Funnel (GDI fork), from quickest to most production-like. Pick one:

- **[Option 1](#option-1-local-binary)** — local binary, for development.
- **[Option 2](#option-2-kubernetes)** — Kubernetes, using the GDI chart in [`deploy-guide/kubernetes`](deploy-guide/kubernetes).
- **[Option 3](#option-3-docker-container-no-kubernetes)** — single Docker container, no cluster needed.

Run every command below from the repository root.

## Option 1: Local binary

Requirements: Go 1.26+, Docker daemon running.

```bash
make build
```
Compiles the `funnel` binary into the repo root.

```bash
./funnel server run --config config/default-config.yaml
```
Starts the server with the default config: local compute (tasks run as plain Docker containers on your machine), BoltDB for the task database, HTTP API on `:8000`, RPC on `:9090`.

Want a different port, database path, or auth? Copy `config/default-config.yaml`, edit it, and pass `--config path/to/your-config.yaml` instead.

Jump to [Testing the server](#testing-the-server) to submit a task.

## Option 2: Kubernetes

Deploys the GDI chart in [`deploy-guide/kubernetes/`](deploy-guide/kubernetes). Requirements: cluster access, `kubectl`, `helm` v3, a container registry the cluster can pull from.

The server runs as a Deployment; every task runs as a worker Job, which starts one executor Job per task step. With the default `pvcMode: shared` the chart needs only **namespaced** permissions (ServiceAccount, Role, RoleBinding, PVC, Deployment, Service, Secret, Ingress) — see [Per-task storage modes](#per-task-storage-modes) for the alternatives.

```bash
kubectl get namespaces
```
Find your namespace — most shared clusters assign one and you can't create your own. Use it as `<namespace>` below. If you *can* create your own: `kubectl create namespace <namespace>`.

```bash
kubectl get storageclass
```
Pick a StorageClass that supports `ReadWriteMany` (e.g. `nfs-csi`) for `pvc.storageClass` — the server and the task pods mount the same PVC.

```bash
docker buildx build --platform linux/amd64 -t <registry>/<registry-user>/funnel-gdi:<tag> -f Dockerfile --push .
```
Builds and pushes the image used by the server and by every worker Job.

```bash
kubectl create secret docker-registry regcred -n <namespace> \
  --docker-server=<registry> --docker-username=<registry-user> --docker-password=<token>
```
Only for a private registry/repository: creates the pull secret. Set `imagePullSecret: regcred` in your values.

```bash
cp deploy-guide/kubernetes/values.yaml my-values.yaml
```
Edit your copy and replace every `<placeholder>`:
- `image` — the image you pushed above,
- `pvc.storageClass` — your `ReadWriteMany` StorageClass,
- `basicauth.password` — used for the API and for worker → server RPC,
- `ingress.host` and `ingress.annotations` (or `ingress.enabled: false` and use port-forward),
- optionally `imagePullSecret`, `pvcMode`, `oidc.*`, `s3.*`, and the GA4GH inputs `sda.serviceurl` / `htsget.serviceurl` (empty = disabled).

```bash
helm template my-funnel deploy-guide/kubernetes -n <namespace> -f my-values.yaml > /dev/null
```
Optional dry run: fails early with a clear message on invalid values (unknown `pvcMode`, missing StorageClass or S3 bucket).

```bash
helm install my-funnel deploy-guide/kubernetes -n <namespace> -f my-values.yaml
```
Installs everything: ServiceAccount, RBAC, PVC, Deployment, Service, Ingress, server/worker config Secrets.

```bash
kubectl get pods -n <namespace>
kubectl port-forward -n <namespace> svc/my-funnel 8000:8000
```
Once the `my-funnel-…` pod is `Running`, port-forward (or use your Ingress host) and jump to [Testing the server](#testing-the-server). The API requires Basic auth, so add `-u admin:<password>` to the `curl` commands there. While a task runs you'll see its worker pod (`<task-id>-…`) and executor pod (`<task-id>-0-…`).

```bash
helm upgrade my-funnel deploy-guide/kubernetes -n <namespace> -f my-values.yaml
kubectl rollout restart deployment/my-funnel -n <namespace>
```
Applies changed values. The restart is needed when only the config Secrets changed (same image tag).

```bash
helm uninstall my-funnel -n <namespace>
```
Uninstalls the release, including the shared PVC with the task database (unless `pvc.existing: true`).

### Per-task storage modes

`pvcMode` in the values (Funnel's `Kubernetes.PVCMode`) decides how a task's inputs/outputs are shared between its worker and executor pods:

| `pvcMode` | Created per task | Needs |
|---|---|---|
| `shared` (default) | nothing — all tasks use `pvc.name`, isolated by `subPath`; executors also get a cross-task `/shared` directory | namespaced RBAC only |
| `pvc` | a PVC from `storageClassName` (falls back to `pvc.storageClass`), deleted with the task | namespaced RBAC only, `ReadWriteMany` StorageClass |
| `full` | an S3-backed PV (Mountpoint CSI) + PVC — upstream behaviour | `s3.bucket`/`s3.region`, the S3 CSI driver, a ClusterRole for `persistentvolumes` (created by the chart, so your account must be allowed to create ClusterRoles) |

Tasks without inputs, outputs or volumes get no PVC in `pvc`/`full` mode (an `emptyDir` is used instead).

### Config format

Funnel parses its config strictly (Protobuf): unknown keys and wrongly-typed values (e.g. `RPCPort: 9090` instead of `"9090"`) stop the server at startup. Durations are written in seconds (`300s`, not `5m`), timeouts as `Timeout: {duration: 30s}`, and RPC credentials under `RPCClient.Credential`. The chart's configs live in `deploy-guide/kubernetes/files/`.

**Troubleshooting:**
- `Forbidden` on install → your account lacks namespaced RBAC rights (set `rbac.create: false` and have an admin create the ServiceAccount/Role/RoleBinding), or you chose `pvcMode: full` without permission to create ClusterRoles.
- `ImagePullBackOff: pull access denied` → set `imagePullSecret` to an existing `kubernetes.io/dockerconfigjson` Secret (see above).
- Server `CrashLoopBackOff` with `failed to unmarshal JSON with protojson` → a config key or value type is wrong (see *Config format*).
- Task stays `QUEUED` / `SYSTEM_ERROR` → `kubectl get jobs,pods -n <namespace>` and `kubectl logs job/<task-id> -n <namespace>`; the server log (`kubectl logs deploy/my-funnel`) shows why a worker Job or PVC couldn't be created.
- PVC stays `Pending` → `pvc.storageClass` / `storageClassName` isn't a working `ReadWriteMany` StorageClass.

## Option 3: Docker container (no Kubernetes)

Requirements: Docker daemon running.

```bash
docker build -t funnel-gdi:dind -f Dockerfile.dind .
```
Builds the server image. This variant includes the Docker CLI (no daemon) so it can drive the host's Docker socket.

```bash
mkdir -p funnel-work-dir
```
Creates the task working-directory folder on the host ahead of time.

```bash
docker run -d --name funnel-dind \
  -p 8000:8000 -p 9090:9090 \
  -w "$(pwd)" \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v "$(pwd)/funnel-work-dir:$(pwd)/funnel-work-dir" \
  -v "$(pwd)/config/default-config.yaml:/opt/funnel/config.yml:ro" \
  funnel-gdi:dind server run --config /opt/funnel/config.yml
```
Starts the server (on re-runs, remove the old container first: `docker rm -f funnel-dind`). What each part does:
- `-p 8000:8000 -p 9090:9090` — exposes the HTTP/API and RPC ports on the host.
- `-w "$(pwd)"` — sets the container's working directory to match the host path, so the config's relative paths resolve consistently on both sides.
- `-v /var/run/docker.sock:...` — shares the host's Docker daemon, so the container can launch task containers on the host.
- `-v "$(pwd)/funnel-work-dir:..."` — mounts the task working directory at the identical path on both host and container (required for the shared-socket setup to work).
- `-v "$(pwd)/config/default-config.yaml:...:ro"` — mounts the config file, read-only.

Custom config instead of default: replace the last `-v` line with your own file (must use absolute paths matching the `-v` mount for `Worker.WorkDir`).

## Testing the server

Works the same way for all three options — only the host/port changes.

```bash
curl -i http://localhost:8000/healthz
```
Health check, expect `200 OK`.

```bash
curl -s http://localhost:8000/v1/service-info
```
Returns server metadata (name, version, supported storage backends).

```bash
curl -s -X POST http://localhost:8000/v1/tasks \
  -d '{"name":"Hello world","executors":[{"image":"alpine","command":["echo","hello world"]}]}'
```
Creates a task, returns `{"id": "<task-id>"}`.

```bash
curl -s "http://localhost:8000/v1/tasks/<task-id>?view=FULL"
```
Checks task status/result — look for `"state": "COMPLETE"`.
