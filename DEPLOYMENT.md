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

The chart only needs **namespaced** permissions (ServiceAccount, Role, RoleBinding, PVC, Deployment, Service, Secret, Ingress): all tasks share one `ReadWriteMany` PVC, isolated by `subPath` (`Kubernetes.PVCMode: shared`), so no per-task PersistentVolume and no ClusterRole is required.

```bash
kubectl get namespaces
```
Find your namespace — most shared clusters assign one and you can't create your own. Use it as `<namespace>` below. If you *can* create your own: `kubectl create namespace <namespace>`.

```bash
kubectl get storageclass
```
Pick a StorageClass that supports `ReadWriteMany` (e.g. `nfs-csi`) for `pvc.storageClass` — the server and every task pod mount the same PVC.

Build and push the image:
```bash
docker buildx build --platform linux/amd64 -t <registry>/<registry-user>/funnel-gdi:<tag> -f Dockerfile --push .
```

```bash
cp deploy-guide/kubernetes/values.yaml my-values.yaml
```
Edit your copy and replace every `<placeholder>`: `image`, `pvc.storageClass`, `basicauth.password`, `ingress.host` (or set `ingress.enabled: false`), optionally `imagePullSecret`, `oidc.*`, `s3.*`, and the GA4GH inputs `sda.serviceurl` / `htsget.serviceurl` (empty = disabled).

```bash
helm install my-funnel deploy-guide/kubernetes -n <namespace> -f my-values.yaml
```
Installs everything: ServiceAccount, RBAC, PVC, Deployment, Service, Ingress, server/worker config Secrets.

```bash
kubectl get pods -n <namespace>
kubectl port-forward -n <namespace> svc/my-funnel 8000:8000
```
Once the pod is `Running`, port-forward (or use the Ingress host) and jump to [Testing the server](#testing-the-server).

```bash
helm uninstall my-funnel -n <namespace>
```
Uninstalls the release, including the shared PVC (unless `pvc.existing: true`).

**Per-task storage modes** (`Kubernetes.PVCMode` in `files/funnel-server-config.yml` and `files/funnel-worker-config.yml`, keep both in sync):
- `shared` (this chart's default) — one pre-existing PVC (`SharedPVCName`) for all tasks.
- `pvc` — a dedicated PVC per task, dynamically provisioned from `StorageClassName`; the Role already allows PVC create/delete.
- `full` — upstream behaviour: a PV (S3 Mountpoint CSI, needs `GenericS3` `Bucket`/`Region`) + PVC per task; requires a ClusterRole for `persistentvolumes`.

**Config format:** Funnel parses its config strictly (Protobuf) — unknown keys and wrongly-typed values (e.g. `RPCPort: 9090` instead of `"9090"`) stop the server at startup. Durations are written in seconds (`300s`, not `5m`), timeouts as `Timeout: {duration: 30s}`, and RPC credentials under `RPCClient.Credential`.

**Troubleshooting:**
- `Forbidden` on install → your account lacks namespaced RBAC rights; set `rbac.create: false` and have an admin create the ServiceAccount/Role/RoleBinding.
- `ImagePullBackOff: pull access denied` → set `imagePullSecret` to an existing `kubernetes.io/dockerconfigjson` Secret.
- Server `CrashLoopBackOff` with `failed to unmarshal JSON with protojson` → a config key or value type is wrong (see *Config format* above).
- Task submission fails → `pvc.storageClass` isn't a working `ReadWriteMany` StorageClass.

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
Starts the server. What each part does:
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
