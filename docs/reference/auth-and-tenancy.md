# Auth and tenancy — delegated identity, tenants and users, isolation

How a new app authenticates callers and isolates their data. Copy `app/router/auth.go` (+ `auth_test.go`),
`app/reqctx/` and `internal/tenants/`. Then apply the isolation rules to every tenant-scoped module. Background:
[../agents/auth-and-tenancy.md](../agents/auth-and-tenancy.md).

## The decision: delegate authentication

The app never stores passwords, issues sessions or renders a login page. Something in front of it vouches for the
caller, and the app maps that identity onto a tenant and a user. This removes credential storage, password resets,
MFA and session security from the app's scope. Building login into the app reverses this decision, so treat it as
a design change and not a feature.

| `Auth.Method` | Use for |
|---|---|
| `none` | demos, local development, tests: every request is `admin` of tenant `default` |
| `proxy` | production behind an authenticating reverse proxy (Authelia, oauth2-proxy, Traefik/Caddy forward-auth) |
| `oidc` | reserved, not implemented: the server refuses to start with it |

## Deploying `proxy` safely

- **The app must be reachable only through the proxy**: bind to localhost or a private network, or use network
  policy. Configure the proxy to **strip** the identity headers from incoming client requests.
- List the proxy's addresses in `Auth.ProxyHeader.TrustedProxies` (CIDRs or bare IPs). This is the second line of
  defense: if the list is empty, every peer is trusted, and the startup log warns about it.
- Set `UserHeader` to the header your proxy uses (`Remote-User` for Authelia, `X-Forwarded-User` for oauth2-proxy).
- Set `TenantHeader` only if the proxy knows the tenant. When it is set, a request without the header is refused
  (fail closed). When it is unset, every user belongs to tenant `default`.
- **A co-located proxy needs both loopback forms in `TrustedProxies`**: `127.0.0.1` and `::1`. A proxy told to dial
  `localhost` often tries `::1` first, and a list with only `127.0.0.1` rejects its headers. Nothing reports an
  error: the app just sees an anonymous caller. To debug, call `/api/v0/me` once over `127.0.0.1` and once over
  `[::1]` and compare. *(aether)*
- Configure the proxy to answer a non-HTML request with `401`, not a `302` to its login page (Authelia decides by
  the `Accept` header). The SPA then sees a clean failure and reloads the page, and the reload gets the portal
  redirect ([web-ui.md](web-ui.md)).
- **Do not let `none` listen on every interface.** *(aether, not yet in reference-app: its default is `none` on all
  interfaces.)* Under `none`, an unset bind address resolves to `127.0.0.1`, and an explicit wildcard (`0.0.0.0`,
  `::`) is a startup error. A specific address (a LAN interface a proxy on another host reaches) is allowed. Check
  this in the config loader. The rule covers the main server only. A container image then has to default to an
  authenticating method. *(aether: `isWildcardBind`, `app/cmd/config.go`)*

## How a request is resolved

1. `guard.protect` asks the configured method for `(tenantSlug, subject)`. If there is none → `401`.
2. `tenants.Provision` turns the pair into IDs, creating the tenant and user on first sight (just-in-time
   provisioning). It is idempotent and race-safe. Unusable values (empty, too long) → `401`.
3. The guard stores a `reqctx.Identity` on the request context.
4. The handler builds the module's `Actor{TenantID, UserID}` and passes it to every module call.

Routes are public unless wrapped in `guard.protect`. Wrap each route explicitly, never a whole subtree, so a public
route is visible at a glance.

## Isolation rules for every tenant-scoped module

- Every method takes an `Actor`. If `Actor.TenantID` is empty, return `ErrNoTenant` and **fail closed**. Never fall
  back to "all tenants".
- The **store** adds `tenant_id = ?` to every query. Handlers never filter, so no caller can forget to.
- Scope by the opaque `TenantID`/`UserID`, never by slug or subject. Those are for display only.
- Another tenant's row is `ErrNotFound` → `404`, not `403`, so callers cannot probe IDs.
- Every tenant-scoped table has a `tenant_id` column (indexed first). Unique constraints include it.
- The module never imports `internal/tenants`. It only sees the IDs.
- Copy the tests `TestTenantIsolation` and `TestNoTenantFailsClosed` (module) and `TestTenantIsolationOverHTTP`
  (router) for each new module.

## Tenancy model

Tenant → users is one-to-many. A user belongs to exactly one tenant, so the same login in two tenants is two users.
In the prototype, all users of a tenant share its data. Per-user ownership (`CreatedBy`) is recorded, but it grants
nothing. If your app needs roles or private data, decide that in the module's API and put the check in the module,
using `Actor.UserID`. It never goes in the handler.

## Machine clients: API keys *(codex, aether)*

A browser extension, a script or a third-party client cannot pass a proxy login. Let users issue **API keys**,
without bringing passwords into the app. A key is a personal access token from `go-bumbu/userauth`'s `service/pat`,
owned by a provisioned user. The app stores only its hash.

- **Read the key only when the proxy sent no identity.** The proxy's identity always wins, and `Authorization` is not
  read then. Under `none`, keys are off.
- **A key's tenant comes from its owner's user row, never from the key or a header.** A key request never
  provisions a tenant or a user.
- **Scope keys narrowly**: codex scopes a key to one sub-partition and a read or write level per app
  (`space:<id>`, `<app>:read`), in a module that only encodes and decodes scopes (`internal/apikeys`). A scope the
  app no longer knows (a renamed app) fails closed.
- **Routes are closed to keys unless a policy lists them.** Keep that list in one file (codex:
  `app/router/keypolicy.go`) and mark the same operations in the spec with `security: [{}, {apiKey: []}]`. Add a test
  that fails when the list names a route the router does not serve. Settings and key management are never open to a
  key.
- **Every bad key gets the same `401`** (malformed, unknown, revoked, expired, owner gone), so a caller learns
  nothing about which part was wrong. A key used on a route it may not use is a `403` that says what the key cannot
  do. A resource outside the key's scope is a `404`, the same as another tenant's.
- **Keys outlive the identity provider.** JIT provisioning never learns that a user left, so a key stays valid until
  it expires or is revoked. Default new keys to an expiry (codex: one year) instead of none.
- Key management (list, create, revoke) is session-only, and each user sees only their own keys. Show the plaintext
  once, at creation.
- **Exempt the key-only paths from the proxy login**, and nothing else (aether: Authelia `bypass` on `/rest`). The
  app must then never read identity headers on those paths, and the proxy still strips them.

## Variations

- **Single tenant**: keep everything. Run `none` or `proxy` without a `TenantHeader`, and every row carries the
  `default` tenant's ID. Switching to multi-tenant later is then a config change, not a data migration.
- **OIDC**: it would be another `assertFunc` in `newAuthGuard`, but it brings redirects, token validation and
  sessions into the app. Design it first, and keep the guard's output (`reqctx.Identity`) unchanged, so modules and
  handlers do not change.
- **Tenant/user administration** (listing, renaming, disabling): add methods to `internal/tenants` and protect their
  routes with an authorization check. reference-app has none. An `Enabled` flag on the provisioned user row works as
  the app's own kill switch: a disabled user gets `403`, and their keys stop working, even while the proxy still
  vouches for them. *(aether)*
- **Roles from the proxy**: forward the user's groups in a header (`Remote-Groups`) and map one configured group to
  the admin role on **every** request. The identity provider stays the authority, and a role change takes effect
  without a re-login. *(aether: `Auth.ProxyHeader.AdminGroup`)*
- **Partitions inside a tenant** (codex: spaces, such as "work" and "private"): a child of the tenant that every user
  of the tenant can see, with no per-partition membership. Scope rows by `tenant_id` first and the partition second.
  When JIT provisioning creates a tenant, seed one default partition, so a new tenant has somewhere to start.
- **Deprovisioning**: JIT provisioning creates tenants and users but never removes them, because a login never says
  that someone left. When you need it, add a small authenticated endpoint that the auth service calls. Do not infer
  removal from missing logins. *(codex lists this as known debt.)*
- **Signed assertions**: trusted headers depend on the network. A signed JWT per request carries the trust with it.
  codex keeps it as the planned upgrade. JIT provisioning works the same with either.
