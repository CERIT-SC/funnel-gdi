---
gdi: true
title: Error codes
menu:
  main:
    weight: 15
---

# Error codes

Errors the Funnel API returns to a client about authentication, access, task
requests and task validation start with a code:

```
FUNNEL-<AREA>-<NNN>: <message>
```

The code identifies the error independently of the message text, so scripts
can match it and the text can be translated. The HTTP status code (or gRPC
status code) of the response is unchanged and follows the TES specification.
When you ask for help ([Getting help](/gdi/deployment/#getting-help)),
include the code.

The messages are kept outside the code, in
[`errcodes/messages.en.yaml`](https://github.com/CERIT-SC/funnel-gdi/blob/master/errcodes/messages.en.yaml).
Only English ships. A translation is a copy of that file named
`messages.<language>.yaml` with the same codes and translated messages;
selecting a language is not implemented yet.

| Area | Errors about |
|---|---|
| `GEN` | Funnel itself |
| `AUTH` | Authentication (Basic, OIDC) and access to other users' tasks |
| `TASK` | Creating, reading and canceling tasks |
| `VAL` | Validation of a submitted task; all problems of a task are listed together under `FUNNEL-TASK-006` |

## Codes

| Code | HTTP status | Message | Meaning and what to do |
|---|---|---|---|
| `FUNNEL-GEN-000` | 500 | internal error (unknown code …) | Funnel used an error code that is not in its catalog. This is a bug: please [report it](/gdi/deployment/#getting-help). |
| `FUNNEL-AUTH-001` | 400 | Missing metadata in the context | The request reached the server without gRPC metadata. Use the HTTP API or a gRPC client that sends metadata. |
| `FUNNEL-AUTH-002` | 401 | Basic/Bearer authorization token missing | The server requires authentication. Send `-u <user>:<password>` (Basic) or `Authorization: Bearer <token>` (OIDC). A Bearer token is also rejected with this code when the server has no OIDC configured. |
| `FUNNEL-AUTH-003` | 401 | Basic-authentication failed | Wrong user name or password. |
| `FUNNEL-AUTH-004` | 401 | Bearer authorization token not accepted | The OIDC token is invalid, expired, or lacks the required audience or scope. Get a new token. |
| `FUNNEL-AUTH-005` | 400 | Unexpected value in the 'state' query-parameter | The OIDC login (`/login`) was called with a `state` that this server did not issue, e.g. a login started in another browser. Start the login again. |
| `FUNNEL-AUTH-006` | 500 | OIDC authentication flow failed [*error*]: *description* | The identity provider rejected the login; *error* and *description* come from it (shortened). |
| `FUNNEL-AUTH-007` | 500 | Failed to receive a JWT for the authorization code: … | The server could not exchange the login code for a token at the identity provider. Ask the operator to check the OIDC client configuration. |
| `FUNNEL-AUTH-008` | 403 | permission denied: taskID: … | The task belongs to another user and the server shows tasks only to their owners (`Server.TaskAccess`). Use the account that created the task, or ask an administrator. |
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
