# Auth and tenancy — delegated identity, just-in-time provisioning, store-enforced isolation

Authentication is delegated: the app never handles passwords, sessions or login screens. `Auth.Method` picks who
vouches for the caller; the auth guard (`app/router/auth.go`) maps that identity onto a tenant and a user through
`internal/tenants`, and every tenant-scoped module filters by the resulting IDs itself.
See [api.md](api.md) for how routes are protected, [features.md](features.md) for what is not implemented.

| `Auth.Method` | Who the caller is |
|---|---|
| `none` (default) | every request is user `admin` of tenant `default`; demos and local development only, logged as a warning |
| `proxy` | a reverse proxy (Authelia, oauth2-proxy, ...) asserts the user in `ProxyHeader.UserHeader` and, optionally, the tenant in `TenantHeader`; on `/api/key/v0`, an API key the user created (below) |
| `oidc` | reserved; the config loader accepts it, `router.New` returns `ErrOIDCNotImplemented`, so `start` fails |

## Request flow

    request ─► guard.protect ─► assertFunc (none | proxy)  → (tenantSlug, subject) or 401
                              ─► tenants.Provision          → Principal{TenantID, UserID, ...}
                              ─► reqctx.WithIdentity        → handler
    handler ─► notes.Actor{TenantID, UserID} ─► module ─► store adds WHERE tenant_id = ?

- `app/reqctx` sits apart from the router so both the guard and `app/router/handlers` can use it without an import
  cycle. `IdentityFrom` returning `ok=false` in a protected handler is a wiring bug → `500`.
- The slug and subject on `Identity` are for display only; **scope by `TenantID` / `UserID`, never by slug or
  subject.**

## API keys (`proxy` mode)

A signed-in user creates API keys (`/api-keys`, Settings → API keys) so a script can use the API without the proxy's
login. A key is a personal access token of `go-bumbu/userauth`'s `service/pat` (hashed at rest, prefixed with the app
name, table `user_pats`), owned by the user's ID, built in `app/cmd/apikeys.go` only when `Auth.Method` is `proxy`.

    /api/v0/...      ─► guard.protect ─► proxy identity → Provision → handler          (never reads Authorization)
    /api/key/v0/...  ─► keyGuard      ─► Bearer key → pat.Verify → scopes → owner →    (never reads identity headers)
                                         rule check (403) → Identity + reqctx.Key → the same handler

- **Two mounts, one credential each.** Every protected route is registered on both by `apiMount.protected`; the key
  mount exists only with keys on. The proxy must let `^/api/key/` through without a login, and nothing else; on that
  path any identity header is the client's own claim, so the key guard never calls `headerauth`.
- **Each route declares its key rule on its registration line** (`keyscope.Closed()`, `AnyKey()` for `/me`,
  `Area("notes")` at the level the method implies: GET/HEAD read, else write). Closed is the zero value, so a route
  is closed to keys unless it says otherwise; a closed route still exists on the key mount and answers `403`.
- **The grantable scopes are the declared areas** (`keyscope.Registry`, `GET /api-keys/scopes`). A malformed area
  fails `router.New`. A stored scope the app no longer knows makes the key unusable (`401`), not half honoured; it is
  still listed, `usable: false`, so its owner can revoke it.
- **Every bad key is the same `401`** ("a valid API key is required"): missing, malformed, unknown, wrong secret,
  expired, revoked, unknown scope, owner gone. A key on a route it may not use is a `403` saying what it lacks.
- **A key's tenant is its owner's** (`tenants.PrincipalByUserID`); a key request never provisions anything.
- **Keys never manage keys**: `/api-keys` is closed on the key mount, and the handler refuses a keyed request too.
- **Keys expire after a year by default** (`handlers.DefaultAPIKeyLifetime`): JIT provisioning never learns that a
  user left, and the app has no disabled users. `pat` limits a user to 25 keys (`409`).

## Invariants

- **Proxy mode trusts headers only from `ProxyHeader.TrustedProxies`** (CIDRs or bare IPs, parsed by the config
  loader; enforced by `go-bumbu/userauth`'s `headerauth`). That is the second line of defense: the deployment must
  make the app reachable only through the proxy, and the proxy must strip the identity headers from client requests.
  An empty `TrustedProxies` trusts every peer and is logged as a warning at startup.
- **`Server.TrustedProxies` is a separate list, with the opposite default.** It only decides which client ip the
  request log records (`X-Forwarded-For` walked past these proxies); empty trusts no one and logs the direct peer.
  Behind a proxy, list it in both.
- **A configured `TenantHeader` fails closed**: a request without it is a `401`, never a silent fall into `default`.
  Without a `TenantHeader`, every user lands in `default`.
- **Nothing is provisioned from an unauthenticated request.** `Provision` creates tenants; only call it with an
  identity the auth method verified.
- **`Provision` is idempotent and race-safe**: insert with `ON CONFLICT DO NOTHING`, then read back, so concurrent
  first logins of one identity resolve to one row (`TestProvisionConcurrentFirstLogin`).
- **An identity the app cannot hold** (empty or overlong slug/subject, `tenants.ValidationError`) is a `401`, not a
  `422` — the authenticator vouched for something unusable.
- **Isolation is enforced in the module store, not the handler.** Every module method takes an `Actor`, returns
  `ErrNoTenant` when `TenantID` is empty (fail closed instead of reading every tenant), and its store adds the tenant
  filter to every query. Another tenant's row is `ErrNotFound` → `404`, never `403`, so IDs cannot be probed.
- **Tenant → User is one-to-many**: a user belongs to exactly one tenant; the same login in two tenants is two users
  (unique index on `tenant_id, subject`). Notes are shared by every user of their tenant; `CreatedBy` records the
  author but grants nothing.

## Tests that guard this

`app/router/auth_test.go` (`TestAuthProxy`, `TestTenantIsolationOverHTTP`, `TestProvisionFailureIsInternal`),
`app/router/apikeys_test.go` (`TestAPIKeyBadKeysLookAlike`, `TestAPIKeyScopePolicy`, `TestAPIKeyMountsStayApart`,
`TestAPIKeyTenantIsolation`), `TestOpenAPIKeyScopes` (each route's rule matches the spec), `libs/keyscope`,
`internal/notes` (`TestTenantIsolation`, `TestNoTenantFailsClosed`), `internal/tenants` (`TestProvisionIsolation`,
`TestProvisionConcurrentFirstLogin`). A new tenant-scoped module copies the two notes tests.

## Not designed yet

OIDC would be another `assertFunc` in `newAuthGuard`, but it brings sessions and callbacks into the app, which the
"delegated" decision avoids; treat it as a design task, not a plug-in. Tenant/user administration and roles are not
implemented ([features.md](features.md)).
