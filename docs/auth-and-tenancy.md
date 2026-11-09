# Authentication and tenancy

Authentication is delegated: the app never handles passwords or sessions. `Auth.Method` picks who vouches for
the caller, and the app maps that identity onto a tenant and a user:

| `Auth.Method` | Who is the caller                                                                                 |
|---------------|---------------------------------------------------------------------------------------------------|
| `none`        | default; every request is user `admin` of tenant `default`. Demos and local development only.     |
| `proxy`       | a reverse proxy (Authelia, oauth2-proxy, ...) asserts the user in `Auth.ProxyHeader.UserHeader` and, optionally, the tenant in `TenantHeader` |
| `oidc`        | reserved; the server refuses to start with it until it is implemented                             |

* `proxy` trusts the headers only from `Auth.ProxyHeader.TrustedProxies` (via
  [go-bumbu/userauth](https://github.com/go-bumbu/userauth)'s `headerauth`). The deployment must still make the
  app reachable only through the proxy, and the proxy must strip the headers from client requests.
  Without a `TenantHeader` every user lands in tenant `default`; with one, a request missing it is a 401.
* **Tenant → User**: a user belongs to exactly one tenant; the same login in two tenants is two users.
* **Just-in-time provisioning**: `internal/tenants` creates an unknown tenant or user on its first authenticated
  request, so onboarding stays with whoever authenticates. Nothing is ever created from an unauthenticated request.
* **Isolation**: the auth guard (`app/router/auth.go`) puts the resolved IDs on the request context
  (`app/reqctx`); handlers pass them to the module as a `notes.Actor`, and the module's store adds the tenant filter
  to every query itself. A note of another tenant is a 404, notes are shared by all users of their tenant, and a
  call without a tenant fails closed (`notes.ErrNoTenant`).
* Routes are public unless wrapped in `guard.protect` in `app/router/api_v0.go`.
