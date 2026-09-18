<title>Funnel Deployment Guide</title>

# Funnel Deployment Guide

Three ways to run Funnel (GDI fork), from quickest to most production-like. Pick one:

- **[Option 1](#option-1-local-binary)** — local binary, for development.
- **[Option 2](#option-2-kubernetes)** — Kubernetes, for a real deployment on any cluster. Main focus of this guide.
- **[Option 3](#option-3-docker-container-no-kubernetes)** — single Docker container, no cluster needed.

Run every command below from the repository root (the Kubernetes template lives at [``](kubernetes) in this repo, so everything here works straight from a checkout — no external chart to clone).

## Option 1: Local binary

Requirements: Go 1.24+, Docker daemon running.

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

This walks through deploying the chart in [``](kubernetes) to a real cluster, from an empty namespace to a running task. Requirements: cluster access, `kubectl`, `helm` v3, a container registry the cluster can pull from.

### 1. Build and push the image

```bash
docker buildx build --platform linux/amd64 -t <registry>/<registry-user>/funnel-gdi:<tag> -f Dockerfile --push .
```
Builds the server/worker image from this repo and pushes it so the cluster can pull it. Both the server Deployment and every per-task Job use this same image. `<registry>` is wherever your cluster pulls images from (a cloud registry, a self-hosted one like Harbor, Docker Hub, etc.) and `<registry-user>` is your username/namespace there — log in first with `docker login <registry>` if it's private.

`--platform linux/amd64` matters if you're building on an Apple Silicon Mac (or anything non-amd64) — the cluster's nodes are almost certainly `linux/amd64`, and without this flag you'll push an image the nodes can't run at all: the pod fails with `ImagePullBackOff: no match for platform in manifest`. `--push` uploads straight from the cross-platform build instead of a separate `docker push`, which is more reliable here than building and pushing as two steps.

### 2. Find (or create) your namespace

On many shared/managed clusters you're already assigned one namespace and can't create new ones — check what you have instead of assuming:
```bash
kubectl get namespaces
```
If that comes back empty or `Forbidden`, check your cluster's management UI if it has one (e.g. Rancher's "Projects/Namespaces" tab), or ask whoever gave you cluster access. Everything below uses `<namespace>` for whatever name you find — substitute it every time.

Only if you're on a cluster where you *do* manage your own namespaces:
```bash
kubectl create namespace <namespace>
```

### 3. Give the cluster pull access to your image

Skip this if your image is in a public repository. If it's private (e.g. your own namespace/project on the registry from step 1), the cluster needs credentials to pull it — this is the fix for an `ImagePullBackOff` / `pull access denied` error on the pod:
```bash
kubectl create secret docker-registry regcred \
  --docker-server=<registry> \
  --docker-username=<registry-user> \
  --docker-password=<your-registry-password-or-token> \
  -n <namespace>
```
Then set `imagePullSecret: regcred` in `my-values.yaml` (step below) so both the server Deployment and every per-task Job reference it.

### 4. Fill in your values

```bash
cp deploy-guide/kubernetes/values.yaml my-values.yaml
```
[``](kubernetes/values.yaml) is the chart's default values file — every field that must be cluster-specific is a `<placeholder>` with a comment explaining it. At minimum, edit in your copy:

| Field | What it's for |
|---|---|
| `image` | The image you just pushed |
| `imagePullSecret` | Name of the Secret from step 3 (`regcred`) — leave `""` if your image is public |
| `pvc.storageClass` | A `ReadWriteMany`-capable StorageClass (e.g. `nfs-csi`) — both the server and every task worker mount this PVC at once |
| `basicauth.password` / `rpcclient.password` | Admin login + internal server↔worker auth — must match each other |
| `ingress.host`, `ingress.annotations.cert-manager.io/cluster-issuer` | Where the API/dashboard will be reachable |

Leave `oidc.enabled: false` and `s3.disabled: true` for a first try — Basic Auth alone is enough to log in and submit tasks; you can turn OIDC and S3 storage on later without reinstalling from scratch.

### 5. Install

```bash
helm install my-funnel ./deploy-guide/kubernetes -n <namespace> -f my-values.yaml
```
This single command creates everything: the `funnel-sa` ServiceAccount + Role + RoleBinding, the PVC, the server Deployment/Service/Ingress, and the Secrets holding the server/worker configs. No separate `kubectl create serviceaccount`/`rolebinding` step needed — the chart owns that now.

Testing on a namespace without Ingress set up yet? Skip it and use port-forwarding instead:
```bash
helm install my-funnel ./deploy-guide/kubernetes -n <namespace> -f my-values.yaml --set ingress.enabled=false
kubectl port-forward -n <namespace> svc/my-funnel 8000:8000
```

### 6. Check it came up

```bash
kubectl get pods -n <namespace>
kubectl logs -n <namespace> deploy-guide/my-funnel
```
Look for one `my-funnel-...` pod in `Running` state. Then jump to [Testing the server](#testing-the-server) — same `curl` commands as the other options, just against your Ingress host (or `localhost:8000` if you port-forwarded).

- Pod stuck in `ImagePullBackOff: pull access denied`? Revisit step 3 — either the image wasn't actually pushed, or `imagePullSecret` in `my-values.yaml` doesn't match a real Secret in `<namespace>`.
- Pod stuck in `ImagePullBackOff: no match for platform in manifest`? You built the image without `--platform linux/amd64` (see step 1) — rebuild and push, then `kubectl rollout restart deployment/my-funnel -n <namespace>`.
- Task creation fails with `Forbidden`? Check [``](kubernetes/templates/role.yaml) — it's almost always a permissions mismatch between `funnel.serviceAccount` and whatever the RBAC objects actually granted.

### Uninstall / clean up

```bash
helm uninstall my-funnel -n <namespace>
```
Removes everything the chart created. The PVC's contents (and BoltDB task history) go with it unless you set `pvc.existing: true` against a PVC you manage yourself.

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
