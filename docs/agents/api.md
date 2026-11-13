# API — contract-first `/api/v0`, problem+json, the Go client

The API is designed first: [`../openapi/api-v0.yaml`](../openapi/api-v0.yaml) (OpenAPI 3.1) is the hand-written
contract, and `app/router/api_v0.go` implements it. Three independent consumers — the handlers, `libs/client` and
the web UI — each map the wire format with their own types, and tests hold all of them to the spec.
See [architecture.md](architecture.md) for the tiers, [auth-and-tenancy.md](auth-and-tenancy.md) for `guard.protect`.

| Endpoint | Auth | Description |
|---|---|---|
| `GET /api/v0/info` | public | version, build time, commit, Go version |
| `GET /api/v0/me` | protected | the caller: user and tenant |
| `GET /api/v0/notes` | protected | the tenant's notes, newest first |
| `POST /api/v0/notes` | protected | create a note; `201` + `Location` |
| `GET /api/v0/notes/{noteId}` | protected | one note; another tenant's note is `404` |
| `PUT /api/v0/notes/{noteId}` | protected | replace a note's title and body; any member of the tenant may edit |
| `DELETE /api/v0/notes/{noteId}` | protected | delete a note; `204`; any member of the tenant may delete |
| `PUT /api/v0/notes/{noteId}/tags/{tagId}` | protected | give the note the tag; idempotent; answers the note |
| `DELETE /api/v0/notes/{noteId}/tags/{tagId}` | protected | take the tag off the note; idempotent; answers the note |
| `GET /api/v0/tags` | protected | the tenant's tag tree (`{items}`: the roots, each with its `children`) |
| `POST /api/v0/tags` | protected | create a tag (`parentId` 0 = root, `afterId` 0 = first); `201` + `Location`; `409` for a sibling's name |
| `PATCH /api/v0/tags/{tagId}` | protected | rename / move (`parentId`) / reorder (`afterId`); absent fields untouched; applied whole or not at all |
| `DELETE /api/v0/tags/{tagId}` | protected | delete the tag and its subtree, taking them off the notes; `204` |
| `GET /api/v0/jobs` | protected | the jobs the app defines, each with the tenant's schedules of it |
| `POST /api/v0/jobs/{jobName}/runs` | protected | queue a run for the tenant: `202` + `Location`, or `200` with the tenant's run already waiting or running; `429` when the queue is full |
| `POST /api/v0/jobs/{jobName}/schedules` | protected | add a cron schedule; `201` + `Location`; `422` for a bad cron, one firing more than once a minute, or a 6th schedule |
| `PATCH` / `DELETE /api/v0/jobs/{jobName}/schedules/{scheduleId}` | protected | change the cron or pause / resume (absent fields untouched); delete, `204` |
| `GET /api/v0/job-runs` | protected | the tenant's runs, newest first, with progress while running |
| `GET /api/v0/job-runs/{runId}` | protected | one run; another tenant's is `404` |
| `POST /api/v0/job-runs/{runId}/cancel` | protected | cancel a waiting run or stop a running one; `204`, also for a run that already ended |
| `GET /api/v0/job-runs/{runId}/log` | protected | `text/plain`, the last `?tail=` lines (1–1000, default 1000) |
| `GET` / `POST /api/v0/api-keys` | protected, `proxy` mode only | the caller's API keys; create one (`201`, token shown once; `422` bad name / scopes / expiry, `409` past the limit) |
| `GET /api/v0/api-keys/scopes` | protected, `proxy` mode only | the areas a key can be granted, each at `read` / `write` |
| `DELETE /api/v0/api-keys/{keyId}` | protected, `proxy` mode only | revoke one of the caller's keys; `204`, another user's is `404` |

**The key-only mount `/api/key/v0`** serves the same routes for API keys (`Authorization: Bearer <key>`), in `proxy`
mode only — [auth-and-tenancy.md](auth-and-tenancy.md). It is the spec's second `servers` entry; an operation open to
keys lists `apiKey` in its `security` and its scope in `x-api-key-scope`, and every protected operation documents the
`403` a key gets where it may not go. Today `/me` (any key) and the notes routes (`notes:read` / `notes:write`) are
open; tags, jobs and `/api-keys` are closed.

Jobs, their runs and their schedules are scoped to the caller's tenant like notes (`internal/jobs` filters tempo's
data by the tenant in each run's params). Run and schedule ids are tempo's UUID strings, the one exception to the
16-hex-char ids; a malformed one is a `404`, like an unknown one.

`GET /notes?tagId={id}` narrows the list to the notes that carry the tag or any tag below it; an unknown tag is an
empty list, a malformed one (not a positive int64, or given twice) `400`. Every `Note` carries `tagIds` (ascending).
Tag ids are integers (closure-tree owns them, up to int64); note ids stay 16 hex chars. A tag id in a path that is
not a positive int64 names no tag: `404`. A tenant holds at most `notes.MaxTagsPerTenant` (1000) tags, which is why
`GET /tags` is not paginated.

- **Relationship toggles** are `PUT` / `DELETE /notes/{noteId}/tags/{tagId}`: idempotent, `200` with the parent
  (the note), no `201` and no `GET` of the link — read `Note.tagIds`.
- **`0` is the root and "first"** in `parentId` / `afterId` (closure-tree's own convention, integer ids only). A
  `null` field of a `TagPatch` counts as absent; the spec says so, it is not a merge patch.
- **A missing or blank required text field is `422`** with its pointer, as `NoteInput.title` was: the module owns
  the rule, so the UI gets a field to mark. `400` stays for a body that is not one JSON object of the right shape.

## Invariants

- **The spec and the router must match exactly.** `TestOpenAPICoversAllRoutes` fails when a mounted
  (method, path) is undocumented or a documented one is not mounted. `TestOpenAPIResponseContract` sends real
  requests and validates status, content type and body against the spec with kin-openapi — a renamed field, a
  dropped `required` or an undocumented status fails there.
- **Spec paths are mount-relative.** `/api/v0` lives only in the spec's `servers` entry and in `apiV0MountPrefix`;
  `.spectral.yaml`'s `no-hardcoded-api-base` rule rejects `/api` in a `paths` key. Every operation needs an
  `operationId`, a defined tag and a description (spectral errors).
- **Routes are registered with their full path on the root router, not on a `PathPrefix` subrouter.** With
  gorilla/mux a later subrouter match clears an earlier "method mismatch", turning `405` into `404`. For the same
  reason the UI catch-all puts its not-the-API matcher first (`attachUI`).
- **Routes are public unless mounted with `protected`**, which wraps them in `guard.protect` (and in the key guard on
  the key-only mount). Protect each route explicitly, so the public ones stay visible at a glance.
- **Both mounts serve the same routes.** `TestOpenAPICoversAllRoutes` also fails when a route is on one mount only.
- **Handlers hold no business logic.** They decode, call the module, map the result. Wire structs
  (`noteResponse`, `noteRequest`, ...) are mapped explicitly from module types, never the module types themselves.
- **Every error is RFC 9457 problem+json** (`go-bumbu/http/problemjson`), including unknown API paths (`404`) and
  wrong methods (`405`). Outside `/api` they stay plain text. `Env.Production: true` switches the writer to masked
  mode: status and instance only.
- **Every problem is logged unmasked, with a reference.** `withRequestID` (`app/router/requestid.go`) wraps the whole
  handler: it keeps a well-formed inbound `Request-Id` or generates one, and echoes it on the response. The request
  log's `req-id`, the problem body's `reference` and the `problem` log line (`logProblem`, `OnProblem` hook: full
  detail, field errors; error level for 5xx, info otherwise) all carry it, so a masked answer stays traceable.
- **Internal errors are logged, never returned.** Handlers log the real error and answer a generic
  `500 internal "unexpected error"`.

## Error mapping

| Source | Status | Slug |
|---|---|---|
| body not one valid JSON object, unknown field, over 1 MiB (`decodeJSON`) | 400 | `bad_request` |
| guard: no / untrusted identity, or a `tenants.ValidationError` | 401 | `unauthorized` |
| key guard: any invalid API key (one message for all) | 401 | `unauthorized` |
| key guard: route closed to keys, or the key lacks its area / level; `/api-keys` with a key | 403 | `forbidden` |
| `pat.ErrTooManyTokens` (a create past the per-user limit) | 409 | `conflict` |
| module `ErrNotFound`, `ErrTagNotFound`, a tag id that is not a positive integer | 404 | `not_found` |
| module `ErrTagExists` (a sibling already has the name) | 409 | `conflict` |
| module `ErrTooManyTags` (a create past `MaxTagsPerTenant`) | 422 | `validation_error`, no field |
| `jobs.ErrJobNotFound`, `ErrRunNotFound`, `ErrScheduleNotFound` | 404 | `not_found` |
| `jobs.ErrQueueFull` | 429 | `queue_full` |
| `jobs.ErrTooManySchedules` (`MaxSchedulesPerJob` per tenant) | 422 | `validation_error`, no field |
| a `tail` that is not one integer from 1 to 1000 | 400 | `bad_request` |
| module `*ValidationError` (`WriteValidation`, pointer `/<field>`; the first failing field only, for now) | 422 | `validation_error` |
| anything else, incl. a protected handler without identity | 500 | `internal` |

## Adding or changing an endpoint

1. Edit the spec; run `make spec-lint`.
2. Add the module method (business rules, typed errors) — [database.md](database.md).
3. Add the handler in `app/router/handlers/<area>.go`: extend the consumer-side interface, map wire structs.
4. Mount it in `attachApiV0` with `m.protected(method, path, rule, handler)`, or `m.public` for a public one. `rule`
   is what an API key needs: `keyscope.Closed()` unless the route is meant for scripts, else `keyscope.Area("<area>")`
   (a new area becomes grantable by being declared). Mirror it in the spec (`security`, `x-api-key-scope`, `403`);
   `TestOpenAPIKeyScopes` fails when they disagree.
5. Add a case to the contract test in `app/router/openapi_test.go` for every documented status you can trigger.
6. Add the method to `libs/client` and cover it in `app/router/client_test.go` (runs the client against the real
   router).
7. Update `webui/src/lib/api/types.ts` and `api.ts` if the UI uses it — [web-ui.md](web-ui.md).

The mapping of every `internal/notes` error lives once, in `handlers/noteserrors.go` (`notesErrors`), for the notes
and the tags handler alike; the jobs handler maps `internal/jobs` errors in `JobsHandler.writeError`.

The contract test validates a `text/plain` response (a run's log) by its documented media type only; every other
body is validated against its schema.

## Go client (`libs/client`)

Hand-written against the spec, stdlib only, shares no types with the server. API failures come back as
`*client.Problem`, matching `client.ErrNotFound`, `ErrUnauthorized`, `ErrForbidden`, `ErrValidation`, `ErrConflict`
and `ErrQueueFull` with `errors.Is`. `RunJob` reports a `200` (a run already in flight) as `reused`.
`WithHeader` lets development code play the trusted proxy; `WithAPIKey` sends a key to `/api/key/v0` instead.

## Versioning

`v0` makes no stability promise. A breaking `v1` is a new spec file, a new `attachApiV1` with its own mount prefix
and its own contract tests (and its own key-only mount); `isAPIPath` must then cover every prefix.
