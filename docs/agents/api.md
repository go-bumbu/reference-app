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
- **Routes are public unless wrapped in `guard.protect`.** Protect each route explicitly, so the public ones stay
  visible at a glance.
- **Handlers hold no business logic.** They decode, call the module, map the result. Wire structs
  (`noteResponse`, `noteRequest`, ...) are mapped explicitly from module types, never the module types themselves.
- **Every error is RFC 9457 problem+json** (`go-bumbu/http/problemjson`), including unknown API paths (`404`) and
  wrong methods (`405`). Outside `/api` they stay plain text. `Env.Production: true` switches the writer to masked
  mode: status and instance only.
- **Internal errors are logged, never returned.** Handlers log the real error and answer a generic
  `500 internal "unexpected error"`.

## Error mapping

| Source | Status | Slug |
|---|---|---|
| body not one valid JSON object, unknown field, over 1 MiB (`decodeJSON`) | 400 | `bad_request` |
| guard: no / untrusted identity, or a `tenants.ValidationError` | 401 | `unauthorized` |
| module `ErrNotFound` | 404 | `not_found` |
| module `*ValidationError` (`WriteValidation`, pointer `/<field>`) | 422 | `validation_error` |
| anything else, incl. a protected handler without identity | 500 | `internal` |

## Adding or changing an endpoint

1. Edit the spec; run `make spec-lint`.
2. Add the module method (business rules, typed errors) — [database.md](database.md).
3. Add the handler in `app/router/handlers/<area>.go`: extend the consumer-side interface, map wire structs.
4. Mount it in `attachApiV0`, wrapped in `guard.protect` unless it is public.
5. Add a case to the contract test in `app/router/openapi_test.go` for every documented status you can trigger.
6. Add the method to `libs/client` and cover it in `app/router/client_test.go` (runs the client against the real
   router).
7. Update `webui/src/lib/api/types.ts` and `api.ts` if the UI uses it — [web-ui.md](web-ui.md).

## Go client (`libs/client`)

Hand-written against the spec, stdlib only, shares no types with the server. API failures come back as
`*client.Problem`, matching `client.ErrNotFound`, `ErrUnauthorized` and `ErrValidation` with `errors.Is`.
`WithHeader` lets development code play the trusted proxy.

## Versioning

`v0` makes no stability promise. A breaking `v1` is a new spec file, a new `attachApiV1` with its own mount prefix
and its own contract tests; `isAPIPath` must then cover both prefixes.
