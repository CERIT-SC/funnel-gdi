---
gdi: true
title: Error codes
menu:
  main:
    weight: 15
---

# Error codes

Every error the Funnel API returns to a client, and every error or warning
Funnel writes into the system logs of a task (`logs[].system_logs`), starts
with a code:

```
FUNNEL-<AREA>-<NNN>: <message>
```

The code identifies the error independently of the message text, so scripts
can match it and the text can be translated. The HTTP status code (or gRPC
status code) of the response is unchanged and follows the TES specification.
When you ask for help ([Getting help](/gdi/deployment/#getting-help)),
include the code.

| Area | Errors about |
|---|---|
| `GEN` | Funnel itself, and errors without a more specific code (by their gRPC status code) |
| `AUTH` | Authentication (Basic, OIDC), the `/login` endpoints and access to other users' tasks |
| `TASK` | Creating, reading and canceling tasks |
| `VAL` | Validation of a submitted task; all problems of a task are listed together under `FUNNEL-TASK-006` |
| `RUN` | Running a task: system logs of the worker, the scheduler and the compute backends |

## Languages

The messages are kept outside the code, in
[`errcodes/messages.en.yaml`](https://github.com/CERIT-SC/funnel-gdi/blob/master/errcodes/messages.en.yaml).
Only English is built in. The language is selected in the config:

```yaml
ErrorMessages:
  # "en" (or empty) is English.
  Language: cs
  # Directory with messages.<Language>.yaml.
  Dir: /etc/funnel/messages
```

A translation is a file `messages.<Language>.yaml` in `Dir` that maps codes to
translated messages, e.g.:

```yaml
FUNNEL-TASK-001: "úloha nenalezena: taskID: %s"
```

- Codes the file leaves out keep their English message, so a translation can
  start with the messages users see most.
- Placeholders (`%s`, `%d`, `%q`, …) must be the same, in the same order, as
  in the English message.
- The server and the workers read the file at start-up and do not start if it
  has a code that Funnel does not know or other placeholders. Workers use the
  `ErrorMessages` of their own config, which on Kubernetes is the server's
  config.
- The code itself never changes with the language.

## Codes

| Code | HTTP status | Message | Meaning and what to do |
|---|---|---|---|
| `FUNNEL-GEN-000` | 500 | internal error (unknown code …) | Funnel used an error code that is not in its catalog. This is a bug: please [report it](/gdi/deployment/#getting-help). |
| `FUNNEL-GEN-001` | 400 | invalid request: … | The request is invalid in a way without a more specific code, e.g. malformed JSON or an unknown field. The rest of the message says what is wrong. |
| `FUNNEL-GEN-002` | 404 | not found: … | The URL or the object does not exist, e.g. a path that is not part of the API. Check the URL against the [TES API](https://ga4gh.github.io/task-execution-schemas/). |
| `FUNNEL-GEN-003` | 401 | authentication required: … | The request needs authentication; see `FUNNEL-AUTH-002`. |
| `FUNNEL-GEN-004` | 403 | permission denied: … | The request is not allowed for your account. |
| `FUNNEL-GEN-005` | 409 | conflict: … | The request conflicts with the current state of the object. Read it again and retry. |
| `FUNNEL-GEN-006` | 499 or 504 | request canceled or timed out: … | The request was canceled by the client or took too long. Retry; if it keeps timing out, the operator should check the server and the database. |
| `FUNNEL-GEN-007` | 500 | not supported: … | The server does not support this call or HTTP method, e.g. `POST` on a URL that only takes `GET`. |
| `FUNNEL-GEN-008` | 500 | internal error: … | An unexpected error in the server, its database or its compute backend. Retry; if it persists, [report it](/gdi/deployment/#getting-help) with the message. |
| `FUNNEL-AUTH-001` | 400 | Missing metadata in the context | The request reached the server without gRPC metadata. Use the HTTP API or a gRPC client that sends metadata. |
| `FUNNEL-AUTH-002` | 401 | Basic/Bearer authorization token missing | The server requires authentication. Send `-u <user>:<password>` (Basic) or `Authorization: Bearer <token>` (OIDC). A Bearer token is also rejected with this code when the server has no OIDC configured. |
| `FUNNEL-AUTH-003` | 401 | Basic-authentication failed | Wrong user name or password. |
| `FUNNEL-AUTH-004` | 401 | Bearer authorization token not accepted | The OIDC token is invalid, expired, or lacks the required audience or scope. Get a new token. |
| `FUNNEL-AUTH-005` | 400 | Unexpected value in the 'state' query-parameter | The OIDC login (`/login`) was called with a `state` that this server did not issue, e.g. a login started in another browser. Start the login again. |
| `FUNNEL-AUTH-006` | 500 | OIDC authentication flow failed [*error*]: *description* | The identity provider rejected the login; *error* and *description* come from it (shortened). |
| `FUNNEL-AUTH-007` | 500 | Failed to receive a JWT for the authorization code: … | The server could not exchange the login code for a token at the identity provider. Ask the operator to check the OIDC client configuration. |
| `FUNNEL-AUTH-008` | 403 | permission denied: taskID: … | The task belongs to another user and the server shows tasks only to their owners (`Server.TaskAccess`). Use the account that created the task, or ask an administrator. |
| `FUNNEL-AUTH-009` | 405 | Only GET method is supported | `/login` and `/login/token` take only `GET`. |
| `FUNNEL-AUTH-010` | 401 | User authentication is required (Basic authentication with username and password) | `/login` on a server with Basic authentication asks the browser for the user name and password. |
| `FUNNEL-AUTH-011` | 404 | Missing 'jwt' cookie. Please log in first. | `/login/token` was called without the cookie set by the OIDC login. Log in at `/login` first. |
| `FUNNEL-TASK-001` | 404 | task not found: taskID: … | No task has this ID (long IDs are shortened in the message). Check the ID with `GET /v1/tasks?view=MINIMAL`. |
| `FUNNEL-TASK-002` | 400 | Task input from SDA requires a Bearer token … | An `sda://` input needs the user's OIDC token: log in with OIDC, or append the token to the URL (`sda://dataset-id/file/path#bearer-token`). |
| `FUNNEL-TASK-003` | 400 | invalid resources: … | The task asks for more CPU, memory or disk than the server allows on Kubernetes. Ask for less, or ask the operator for the limits. |
| `FUNNEL-TASK-004` | 500 | error creating task: … | The task could not be stored in the database. Try again; if it persists, the operator should check the server log and the database. |
| `FUNNEL-TASK-005` | from the plugin | rejected by the authorization plugin: … | The server's authorization plugin rejected the request; the message comes from the plugin. |
| `FUNNEL-TASK-006` | 400 | invalid task message: followed by one line per problem | The task document is invalid. Each following line starts with a `FUNNEL-VAL` code; fix all of them. |
| `FUNNEL-VAL-001` | 400 (in `FUNNEL-TASK-006`) | Task.Executors: at least one executor is required | Add an executor with `image` and `command`. |
| `FUNNEL-VAL-002` | 400 (in `FUNNEL-TASK-006`) | Task.Executors[*i*].Image: required, but empty | Set the container image of executor *i*. |
| `FUNNEL-VAL-003` | 400 (in `FUNNEL-TASK-006`) | Task.Executors[*i*].Command: required, but empty | Set the command of executor *i*. |
| `FUNNEL-VAL-004` | 400 (in `FUNNEL-TASK-006`) | Task.Executors[*i*].Workdir: must be an absolute path | Use a path starting with `/`. |
| `FUNNEL-VAL-005` | 400 (in `FUNNEL-TASK-006`) | Task.Executors[*i*].Workdir: "…" is not allowed as a working directory | The path is reserved by the server (e.g. a system directory); use your own directory such as `/data`. |
| `FUNNEL-VAL-006` | 400 (in `FUNNEL-TASK-006`) | Task.Executors[*i*].Stdin: must be an absolute path | Use a path starting with `/`. |
| `FUNNEL-VAL-007` | 400 (in `FUNNEL-TASK-006`) | Task.Executors[*i*].Stdout: must be an absolute path | Use a path starting with `/`. |
| `FUNNEL-VAL-008` | 400 (in `FUNNEL-TASK-006`) | Task.Executors[*i*].Stderr: must be an absolute path | Use a path starting with `/`. |
| `FUNNEL-VAL-009` | 400 (in `FUNNEL-TASK-006`) | Task.Inputs[*i*].Content: Url is non-empty | An input has either inline `content` or a `url`, not both. |
| `FUNNEL-VAL-010` | 400 (in `FUNNEL-TASK-006`) | Task.Inputs[*i*].Url: required, but empty | Set the `url` of input *i* (or its `content`). |
| `FUNNEL-VAL-011` | 400 (in `FUNNEL-TASK-006`) | Task.Inputs[*i*].Path: required, but empty | Set the path where input *i* appears in the container. |
| `FUNNEL-VAL-012` | 400 (in `FUNNEL-TASK-006`) | Task.Inputs[*i*].Path: must be an absolute path | Use a path starting with `/`. |
| `FUNNEL-VAL-013` | 400 (in `FUNNEL-TASK-006`) | Task.Inputs[*i*].Path: "…" is not allowed as an input path | The path is reserved by the server; use your own directory such as `/data`. |
| `FUNNEL-VAL-014` | 400 (in `FUNNEL-TASK-006`) | Task.Outputs[*i*].Url: required, but empty | Set where output *i* is uploaded to. |
| `FUNNEL-VAL-015` | 400 (in `FUNNEL-TASK-006`) | Task.Outputs[*i*].Path: required, but empty | Set the container path of output *i*. |
| `FUNNEL-VAL-016` | 400 (in `FUNNEL-TASK-006`) | Task.Outputs[*i*].Path: must be an absolute path | Use a path starting with `/`. |
| `FUNNEL-VAL-017` | 400 (in `FUNNEL-TASK-006`) | Task.Outputs[*i*].Path: "…" is not allowed as an output path | The path is reserved by the server; use your own directory such as `/data`. |
| `FUNNEL-VAL-018` | 400 (in `FUNNEL-TASK-006`) | Task.Volumes[*i*]: must be an absolute path | Use a path starting with `/`. |
| `FUNNEL-VAL-019` | 400 (in `FUNNEL-TASK-006`) | Task.Volumes[*i*]: "…" is not allowed as a volume path | The path is reserved by the server; use your own directory such as `/data`. |
| `FUNNEL-VAL-020` | 400 (in `FUNNEL-TASK-006`) | Task.Tags[""]=…: empty key | Tag names must not be empty. |

### Task system logs

These codes appear in the system logs of a task (`GET /v1/tasks/<id>?view=FULL`,
`logs[].system_logs`), not as an HTTP error. Details such as the URL or the
underlying error are in the fields of the log entry.

| Code | Level | Message | Meaning and what to do |
|---|---|---|---|
| `FUNNEL-RUN-001` | error | System error | The task failed because of Funnel or its environment, not the task's commands. The `error` field says why. |
| `FUNNEL-RUN-002` | error | Exec error | An executor exited with a non-zero exit code. Check its `stderr` and `exit_code`. |
| `FUNNEL-RUN-003` | error | Failed to upload outputs after executor error | After an executor failed, its outputs could not be uploaded either. |
| `FUNNEL-RUN-004` | error | Couldn't prepare log files | The worker could not create the files for the executor's stdin, stdout or stderr. Check the paths in the task and the worker's disk. |
| `FUNNEL-RUN-005` | error | failed to get task. ID unknown | The worker could not read its task. The operator should check the worker's config and its access to the database or server. |
| `FUNNEL-RUN-006` | error | failed to sync docker client API version | The worker could not talk to the container engine. The operator should check Docker on the node. |
| `FUNNEL-RUN-007` | error | failed to pull docker image using command | The image could not be pulled. Check the image name and that the registry is reachable and allows the pull. |
| `FUNNEL-RUN-008` | error | failed to run docker container | The container could not start, e.g. because of a wrong command or image. |
| `FUNNEL-RUN-009` | error | failed to stop docker container | The container could not be stopped after the task ended or was canceled. |
| `FUNNEL-RUN-010` | warning | download source directory is empty | An input directory (`type: DIRECTORY`) has no files. |
| `FUNNEL-RUN-011` | warning | upload source directory is empty | An output directory has no files after the task. |
| `FUNNEL-RUN-012` | error | download failed | An input could not be downloaded (`url` and `error` fields). Check the URL and the access rights to it. |
| `FUNNEL-RUN-013` | error | upload failed | An output could not be uploaded (`url` and `error` fields). Check the URL and the write access to it. |
| `FUNNEL-RUN-014` | error | Error in AssignTask | The scheduler could not assign the task to a node. |
| `FUNNEL-RUN-015` | error | error submitting task to … | The compute backend (Slurm, PBS, HTCondor, Grid Engine, AWS Batch, GCP Batch) rejected the task. The `error` and `stderr` fields give its answer. |
| `FUNNEL-RUN-016` | error | … reports system error for task | The HPC scheduler reports that the task's job failed (`error` field: its reason). |
| `FUNNEL-RUN-017` | error | … job in FAILED state | The job of the task (Kubernetes, AWS Batch, GCP Batch) failed. |
| `FUNNEL-RUN-018` | error | Error cleaning up compute resources during cancel | The task is canceled, but its resources in the compute backend could not be removed. The operator should remove them. |
| `FUNNEL-RUN-019` | error | Kubernetes worker pod has a terminal container waiting error | The worker pod cannot start a container, e.g. because its image cannot be pulled. |
| `FUNNEL-RUN-020` | error | Kubernetes worker job failed to create pod | Kubernetes did not create the worker pod, e.g. because of a quota. The `error` field gives the reason. |
| `FUNNEL-RUN-021` | error | Kubernetes job not found for task | The task's Kubernetes job disappeared, e.g. it was deleted by hand. |
| `FUNNEL-RUN-022` | error | GCP Batch job failed: … | GCP Batch reports why the job failed. |
