# API keys — design

Date: 2026-11-12 · Status: implemented 2026-11-13 · Model to copy: codex (`andresbott/codex`, see "Prior art")

## Intent

reference-app is the boilerplate the author's Go apps are built from. It should ship user-generated, scoped API keys so
that every app built from it can be scripted or integrated (scripts, the Go client, third-party tools) without a
browser login. There is no specific client asking for this today; the point is to have the pattern right in the
template, so a developer copying it gets keys, and a developer adding an endpoint knows exactly how to open it to keys.

## Goals

- In `proxy` (header) auth mode, a signed-in user can create, list and revoke their own API keys.
- A key authenticates requests to the API surface (`/api/v0`) only, as its owner, inside its owner's tenant.
- Each key carries scopes: per-area `read` / `write` levels. `write` includes `read`. `GET` and `HEAD` need `read`,
  every other method needs `write`.
- Which routes a key may reach, and at which area, is **declared on the route where it is registered**.
- Initially only the **notes** area is open to keys (`notes:read`, `notes:write`).

## Non-goals

- Keys in `none` mode. In `none` mode there are no keys and no key-management endpoints.
- Fine-grained per-action scopes (`notes:delete`, `jobs:run`). Per-area read/write only.
- Opening tags, jobs, schedules, settings or `/api-keys` itself to keys (for now).
- Editing a key (rename, change scopes, extend expiry). Revoke and create a new one.
- Keys owned by a tenant or a service account; a key always belongs to one user.
- Admin views of other users' keys.

## Anti-goals (would be a failure even if it "works")

- A key that reaches a route nobody explicitly opened to keys. Forgetting the declaration must fail closed.
- A key that can manage keys (create, list, revoke) — keys never mint keys.
- A key request that provisions a tenant or a user, or lands in a tenant other than its owner's.
- Error responses that tell a caller *why* a key is bad (unknown vs revoked vs expired vs wrong secret).
- A key presented alongside a valid proxy identity changing who the caller is.
- Plaintext keys stored anywhere, or shown more than once.

## Constraints

- Follow the repo's architecture docs (`docs/agents/architecture.md`, `auth-and-tenancy.md`, `api.md`): delegated
  auth, isolation enforced in module stores by `TenantID`/`UserID`, problem+json errors, contract-first OpenAPI
  (`docs/openapi/api-v0.yaml`, checked by `app/router/openapi_test.go`), gates in `docs/agents/testing.md`.
- Dependencies point `app/` → `internal/` → `libs/`; modules never import each other or `app/`.
- Key storage, hashing, prefix, expiry, `last_used_at` and the per-user key limit come from
  `github.com/go-bumbu/userauth/service/pat` (already a dependency at v0.8.0, same version codex uses). Do not
  re-implement token crypto.
- Update `docs/agents/auth-and-tenancy.md`, `api.md`, `features.md` (and `architecture.md` if the layering picture
  changes) in the same change.

## Decided (by the author, in the conversation)

1. **Replicate codex's API key mechanism**, adapted to reference-app (no spaces; tenant comes from the key owner).
2. **Header auth only**: keys exist only when `Auth.Method` is `proxy`.
3. **Only the API surface** is reachable with a key.
4. **Scopes are per area, read/write**, like codex.
5. **Scope is declared on the route at registration time**, not in a central allowlist. Add a TODO item to review
   alternatives later.
6. **Only notes are manageable by keys for now.**

## How it behaves

### Authentication flow (proxy mode)

Decided after the builder check (2026-11-13): keys get **their own mount, `/api/key/v0`**, serving the same routes as
`/api/v0`. The proxy bypasses its login for `^/api/key/` and nothing else. This follows
`docs/reference/auth-and-tenancy.md` and makes the "a key next to a proxy identity changes the caller" anti-goal
impossible by construction: neither mount reads the other's credential.

- `/api/v0`: today's guard, unchanged. It never reads `Authorization`.
- `/api/key/v0`: the key guard only. It never reads identity headers.
  - The header must be `Bearer <token>` (scheme case-insensitive); anything else → 401.
  - `pat.Service.Verify(token)`; not found / wrong secret / revoked / expired / owner gone → 401.
  - The stored scopes are parsed against the declared vocabulary; any unknown scope → 401 (fail closed).
  - The owner is resolved with `tenants.PrincipalByUserID`; **nothing is provisioned**.
  - The owner's `reqctx.Identity` goes on the context, plus `reqctx.Key` (id, name, grant).
- In `none` mode the key mount does not exist (problem+json 404).

Every way a key can be bad is the **same 401 with the same message** ("a valid API key is required"). Lookup
failures are 500 and logged.

### Route policy (scope check)

- Every protected route is registered with `m.protected(method, path, rule, handler)` in `app/router/api_v0.go`;
  `rule` is a `libs/keyscope` value: `Closed()` (the zero value), `AnyKey()`, or `Area("notes")` at the level the
  method implies (GET/HEAD → read, else write), or `AreaAt(area, level)`.
- For a keyed request: route not open to keys → **403** "API keys can't use this endpoint"; key lacks the area →
  **403** "this API key has no access to notes"; key has read but route needs write → **403** "this API key is
  read-only for notes". Non-keyed requests are untouched by the policy.
- Always open to any valid key: `GET /api/v0/me` (so a client can ask who it is). `/me` additionally returns an
  `api_key` object (id, name, scopes) when the request was keyed, like codex. Public routes (`/info`) stay public.
- **The scope vocabulary is derived from the declarations**: the set of areas a user may grant is the set of areas
  declared on routes. No separate registry to keep in sync.
- Malformed declarations (empty area, invalid characters, a `:` in the area name) fail at router construction
  (`router.New` returns an error), so `start` fails rather than a route silently 403-ing.
- Second lock, as in codex: the `/api-keys` handlers themselves refuse a keyed request with 403 even though the
  policy already keeps keys off them.

### Initial key surface (notes only)

| Route | Key access |
|---|---|
| `GET /notes`, `GET /notes/{noteId}` | `notes:read` |
| `POST /notes`, `PUT /notes/{noteId}`, `DELETE /notes/{noteId}` | `notes:write` |
| `PUT/DELETE /notes/{noteId}/tags/{tagId}` | `notes:write` (builder's-call note below) |
| `/tags*`, `/jobs*`, `/job-runs*`, `/api-keys*` | closed |
| `GET /me` | any valid key |

### Key management API

Wire fields are camelCase like the rest of the API; validation failures are `422` with a field pointer (`400` stays
for a body that is not one JSON object), as the repo's error mapping prescribes.

Under `/api/v0/api-keys`, registered only in proxy mode (in `none` mode the routes don't exist → 404, which the UI
uses to know keys are off, as in codex). All operate on the caller's own keys only.

- `GET /api-keys` → `{items: [...]}`; each item: `id`, `name`, `scopes` (e.g. `["notes:write"]`), `usable`
  (false when the stored scopes no longer decode — such a key opens nothing but is listed so it can be revoked),
  `createdAt`, `lastUsedAt`, `expiresAt`.
- `POST /api-keys` with `{name, scopes, expiresAt?}` → 201 with the item plus `token` (the only time it is shown) and
  a `Location` header. Validation (422 with a pointer): name 1–100 chars after trim; at least one scope; every scope
  known; an area listed once; `expiresAt`, if set, in the future. Too many keys → 409.
- `DELETE /api-keys/{id}` → 204; another user's or unknown id → 404.
- `GET /api-keys/scopes` → the grantable scopes derived from the route declarations (`[{area, levels}]`), so the UI
  builds its create form from the server and never hard-codes `notes`. Session-only like the rest.
- **Expiry defaults to one year** (as codex, and as `docs/reference/auth-and-tenancy.md` requires: JIT provisioning
  never learns a user left, and `internal/tenants` has no disabled flag). The API applies the default when
  `expires_at` is omitted; the UI pre-fills it and lets the user choose another future date. "Never expires" is not
  offered.

Errors use the app's existing problem+json writer and slugs, not codex's `{error, message}` JSON.

### Key format and storage

- `pat.Service` over its DB store (`pat/store/db`) on the app's database, both SQLite and Postgres (the DB matrix in
  `testing.md` applies).
- Token prefix identifies the app so a leaked key is recognisable; it is a constant an app built from the boilerplate
  changes (default: `refapp`). Storage mode `pat.HashOnly`.
- Owner lookup for `pat` (`userauth.UserGetter`): by user ID via `internal/tenants`; `GetUserByLogin` never finds
  anyone (the app has no logins), same as codex `keyowners.go`.

### Wiring

- The keys service is built only when `Auth.Method == proxy`; the router gets nil otherwise, meaning: no key
  management routes, and no key is ever accepted.
- `internal/tenants` gains a lookup of a principal by user ID (its own table, its own store).
- `app/reqctx` gains the "keyed request" marker next to `Identity`.

### Contract and client

- `docs/openapi/api-v0.yaml`: a second `servers` entry `/api/key/v0`; an `apiKey` http-bearer security scheme; the
  `/api-keys` operations with schemas; `/me`'s optional `apiKey`; each key-reachable operation has
  `security: [{}, {apiKey: []}]` and `x-api-key-scope` (spectral refuses scopes on an http scheme); a `403` on every
  protected operation but `/me`. `TestOpenAPIKeyScopes` holds the route rules to the spec.
- `libs/client`: `WithAPIKey(key)` (Bearer header, `/api/key/v0`), `ErrForbidden`, and the `/api-keys` methods.

### Web UI

- A new "API keys" section in the existing settings shell (`webui/src/views/settings/`, sidebar entry in
  `SettingsSidebar.vue`): list keys (name, scopes, created, last used, expiry, unusable badge), create (name,
  scope per area as none/read/write, optional expiry), show the token once with a copy button and a clear "you won't
  see this again", revoke with confirmation.
- Hidden when `GET /api-keys` is 404 (keys off). Follow `docs/agents/web-ui.md` conventions (TanStack Query, PrimeVue).
- Note: the UI in reference-app is served outside the auth guard (the proxy protects it), so "keys only reach the
  API" holds by construction — a key authenticates nothing outside `/api/v0`. No codex-style `uiGuard` needed.

### Docs and TODO

- `auth-and-tenancy.md`: the key path in the request flow, its invariants (proxy identity wins, no provisioning,
  uniform 401, closed by default), and the tests that guard them.
- `api.md`: how to declare a route's key scope when adding an endpoint.
- `features.md`: API keys (proxy mode) implemented; keys in `none`/`oidc`, per-action scopes, non-notes areas not.
- `TODO.md` (via the `todo` CLI or its format): "Review alternatives to declaring API key scopes on routes" —
  e.g. a central allowlist (codex), deriving from the OpenAPI spec.

## Tests that must exist (minimum)

- Guard: proxy identity wins over a key; key without proxy identity authenticates as owner in owner's tenant; each
  bad-key case → identical 401; keyed request never provisions; owner deleted → 401.
- Policy: undeclared route → 403 for keys, unaffected for proxy users; read key on write route → 403; key without the
  area → 403; `/me` open to any key; `/api-keys` closed to keys (policy and handler both).
- Tenant isolation over HTTP with keys: a key of tenant A cannot see tenant B's notes (404).
- Management API: create/list/revoke happy paths, every 400, 409, 404 on another user's key, token shown only on
  create; routes absent in `none` mode.
- Router construction fails on a malformed declaration.
- UI component/composable specs mirroring the existing settings tests.

## Left to the builder

- The exact Go shape of the per-route declaration (wrapper, option on `api(...)`, metadata read back via
  `mux.CurrentRoute`, etc.), as long as it sits on the registration line and defaults to closed.
- Package placement of the route-policy mechanism: decided after review (2026-11-13), `libs/keyscope` (stdlib
  only: rules, registry, grant, check); `app/router` keeps the guard, the 403 mapping and the handlers.
- How `/me` exposes key info, field names in wire types (snake_case, matching existing payloads).
- How the OpenAPI spec records per-operation scopes.

## Calls I made (not discussed with the author)

- Note–tag attach/detach (`PUT/DELETE /notes/{noteId}/tags/{tagId}`) count as `notes:write`; tag listing/CRUD stays
  closed, so a key can attach only tag IDs it already knows.
- `GET /me` open to any valid key; `/info` stays public.
- Token prefix default `refapp`; storage `HashOnly`; name limit 100 runes; expiry defaults to one year, no "never".
- Grantable scopes served by `GET /api-keys/scopes` rather than hard-coded in the UI.
- Browser clients on another origin (CORS, preflight) are out of scope; `features.md` says so. A browser extension
  with host permissions is not affected by CORS.
- `docs/reference/auth-and-tenancy.md` ("Machine clients") says to keep the key policy in one file; this design
  replaces that with per-route declarations (author's decision 5), so that guide is updated in the same change, and
  its "test that the list names only served routes" becomes moot (declarations live on served routes by construction).
  The spec still marks key-reachable operations with `security: [{}, {apiKey: []}]` as the guide prescribes.
- Keys-off is signalled to the UI by `/api-keys` returning 404.
- Error bodies use the app's problem+json, not codex's JSON shape.

## Prior art (codex)

Files worth reading before planning, all in `andresbott/codex`:
`internal/apikeys/apikeys.go` (vocabulary, encode/decode, fail-closed decode), `app/router/keyauth.go` (key
identity), `app/router/keypolicy.go` (route policy — reference-app replaces its central `keyRoutes` map with
per-route declarations), `app/router/keyowners.go` (`pat` owner lookup), `app/router/auth.go` (`proxyGuard` order:
proxy identity first, then key), `app/router/handlers/apikeys/apikeys.go` (management API), `app/cmd/apikeys.go`
(building `pat.Service` only in proxy mode), `webui/src/**/ApiKeysSection.vue`, `useApiKeys.ts`.
Differences: codex binds each key to one space (`space:<id>` scope) and checks app enablement per space; reference-app
has neither.
