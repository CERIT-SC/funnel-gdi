---
title: OAuth2
menu:
  main:
    parent: Security
    weight: 10
---
# OAuth2

By default, a Funnel server allows open access to its API endpoints, but in
addition to Basic authentication it can also be configured to require a valid
JWT in the request.

API requests must present the token in the `Authorization: Bearer` header;
Funnel validates that the token is issued by a trusted service (specified in
the YAML configuration file) and has not expired. For the web dashboard,
Funnel implements the login itself: the `/login` endpoint (`RedirectURL`)
redirects the user to the OIDC service and receives the authorization code
when the user returns.
In addition, if the OIDC provides a token introspection endpoint (in its
configuration JSON), Funnel server also calls that endpoint to make sure the
token is still active (i.e., no token invalidation before expiring).

Optionally, Funnel can also validate the scope and audience claims to contain
specific values.

To enable JWT authentication, specify `OidcAuth` section in your config file:

```yaml
Server:
  OidcAuth:
    # URL of the OIDC service configuration:
    ServiceConfigURL: "https://my.oidc.service/.well-known/openid-configuration"

    # Client ID and secret are sent with the token introspection request
    # (Basic authentication):
    ClientId: your-client-id
    ClientSecret: your-client-secret

    # Optional: if specified, this scope value must be in the token:
    RequireScope: funnel-id

    # Optional: if specified, this audience value must be in the token:
    RequireAudience: tes-api

    # The URL where OIDC should redirect after login (keep the path '/login')
    RedirectURL: "http://localhost:8000/login"

    # List of OIDC subjects promoted to Admin status.
    Admins:
      - user.one@example.org
      - user.two@example.org

  TaskAccess: OwnerOrAdmin
```

The `TaskAccess` property configures the visibility and access-mode for tasks:

* `All` (default) - all tasks are visible to everyone
* `Owner` - tasks are visible to the users who created them
* `OwnerOrAdmin` - extends `Owner` by allowing Admin-users (defined under
  `Admins`) access everything

As new tasks are created, the username behind the request is recorded as the
owner of the task. Depending on the `TaskAccess` property, if owner-based
access-mode is enabled, the owner of the task is compared to username of current
request to decide if the user may see and interact with the task.

Workers authenticate to the server only with Basic credentials
(`RPCClient.Credential`), never with a JWT. With BoltDB or Badger (and the
`local` compute backend), OIDC therefore always needs `Server.BasicAuth` with
a user for the workers as well — see [Basic Auth](/docs/security/basic/).
With `OidcAuth` alone, workers are rejected and tasks stay `QUEUED`.

Make sure to properly protect the configuration file so that it's not readable
by everyone:

```bash
$ chmod 600 funnel.config.yml
```

The command-line client (`funnel task ...`) does not perform the OIDC login,
but it sends a token you already have. Set `FUNNEL_SERVER_TOKEN`; it takes
precedence over `FUNNEL_SERVER_USER` / `FUNNEL_SERVER_PASSWORD`:

```bash
$ export FUNNEL_SERVER_TOKEN=<access-token>
$ funnel task list
```
