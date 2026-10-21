---
gdi: true
title: Error codes
menu:
  main:
    weight: 15
---

# Error codes

Errors the Funnel API returns to a client start with a code when they are
listed on this page:

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
