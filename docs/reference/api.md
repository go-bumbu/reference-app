# API — contract-first endpoints, problem+json, contract tests, Go client

How a new app designs and serves its REST API. Copy `docs/openapi/api-v0.yaml` (as a skeleton), `.spectral.yaml`,
`app/router/{router.go,api_v0.go,openapi_test.go,client_test.go}`, `app/router/handlers/json.go` and `libs/client`.
Background: [../agents/api.md](../agents/api.md).

## Principles

- **The spec comes first and is written by hand.** It is the contract. Code follows it, and tests fail when either
  side drifts. Do not generate the spec from code, or code from the spec.
- **Three consumers, no shared types.** Server handlers, the Go client and the UI each define their own wire
  structs from the spec. A module's model can then change without changing the wire format, and the contract
  tests catch any mismatch.
- **Every API error is RFC 9457 problem+json**, including unknown paths and wrong methods, via
  [go-bumbu/http/problemjson](https://github.com/go-bumbu/http). Set `problemBaseURI` to your own namespace.
- **Mask errors in production** (`Env.Production: true` → masked mode), and never put an internal error in a
  response. Log it and answer `500 internal`.

## Spec conventions (`docs/openapi/api-vN.yaml`, OpenAPI 3.1)

- Paths are relative to the mount point. The `/api/vN` base appears only in `servers`. The spectral rule
  `no-hardcoded-api-base` enforces this.
- Every operation has an `operationId`, a defined tag and a description (spectral errors).
- Define schemas, parameters and error responses once in `components` and reference them
  (`#/components/responses/Unauthorized`, `ValidationError`, `Internal`, …).
- List every status the handler can return, problem responses included. An undocumented status fails the contract
  test.
- `make spec-lint` runs spectral. `make verify` and CI run it too.

House style that codex and aether converged on (codex writes it down in `docs/openapi/CONVENTIONS.md`; give your
app such a file once the spec grows):

- **Plural nouns, no verbs in paths.** `/notes`, `/notes/{id}`. An action is an HTTP method, not `/notes/{id}/archive`.
  Nest sub-resources one level at most. Deeper, use a top-level collection with a filter.
- **Request schemas are separate from resource schemas** and set `additionalProperties: false`. Server-set fields
  (`id`, `created_at`) never appear in a request. A field that cannot change after create is in the create body
  only.
- `snake_case` field names. RFC 3339 timestamps. In OpenAPI 3.1, write `type: [string, "null"]`, not `nullable`.
- Lists return an envelope (`{items: [...]}`), so fields can be added later. **Paginate only a list that can grow
  large.** Use a `limit` with a default and a cap, and an opaque `next_cursor` that is valid only for the same
  sort. An unknown cursor is a `400`. A list that the server already keeps small needs no cursor.
- A relationship between two resources is toggled with `PUT` / `DELETE /parents/{id}/children/{childId}`: both
  idempotent, both `200` with the parent. The link has no `GET` of its own. *(reference-app: note tags)*
- A sentinel `0` ("the root", "first") is acceptable for integer ids a library owns (closure-tree's tags); give
  string-id resources a nullable field instead. *(reference-app)*
- A resource of another tenant is a `404`, never a `403`. `403` means "you may not do this", and never reveals that
  a resource exists.

## Adding an endpoint

1. **Spec**: add the operation, then run `make spec-lint`.
2. **Module**: add the method, with business rules and typed errors
   ([modules-and-database.md](modules-and-database.md)).
3. **Handler** in `app/router/handlers/<area>.go`:
   - Extend the consumer-side interface (`NotesService`) with the new method.
   - Decode the body with `decodeJSON`. It enforces a single JSON object, rejects unknown fields and caps the body
     at 1 MiB.
   - Map the request into the module's input type, and the result into a response struct, field by field.
   - Map errors: decode error → `400 bad_request`; `ErrNotFound` → `404 not_found`; `*ValidationError` →
     `WriteValidation` with pointer `/<field>` → `422`; anything else → log, then `500 internal`.
   - On create, answer `201` with a `Location` header.
4. **Route** in `attachApiV0`: register it with `r.Path(prefix + path).Methods(...)`, on the root router with its
   full path. Wrap it in `guard.protect` unless it is meant to be public.
5. **Contract test**: `TestOpenAPICoversAllRoutes` fails until steps 1 and 4 agree. Add requests to
   `TestOpenAPIResponseContract` for each documented status you can trigger.
6. **Client**: add a method to `libs/client`, and a case in `app/router/client_test.go`, which runs the client
   against the real router.
7. **UI**: add types and an API function ([web-ui.md](web-ui.md)).

## Bounded URLs: lists travel in the body *(aether)*

**No `GET` or `DELETE` may carry a variable-length list in its URL**: no repeated query parameter, no array path
segment. Behind a forward-auth proxy (Caddy + Authelia), the request URI is copied into a header of the auth
sub-request. Authelia answers headers over about 4 KB with `431` before the request reaches the app, so the app
cannot log it or explain it. A multi-select of a few hundred items is enough to cause it.

- **A read that needs a list is a `POST` to a named read sub-resource** (`POST /tracks/raw-tags`,
  `POST /libraries/preview`). Describe it as safe and idempotent, and return an ordinary read response. Do not use
  a `GET` with a body (proxies drop it). Do not use the `QUERY` method yet: OpenAPI, spectral and `fetch` do not
  support it.
- A delete of a selection is a named `POST` too (`POST /pictures/removals`), not a `DELETE` with a body.
- A single scalar that a previous response handed back may still ride in a `GET` query (an image URL for
  `<img src>`).
- Cap the list in the handler anyway (`maxSelectionPaths = 50`). The body removes the `431`. The cap bounds the work
  one request can ask for.
- Enforce it in `.spectral.yaml` with aether's rules `no-array-in-get-delete-url` and `array-query-needs-maxitems`.
  Check parameters at operation level **and** path-item level. Keep negative fixtures in `docs/openapi/testdata/`
  that prove the rules fire.

## Validation and batch responses *(aether)*

- **`400` means malformed, `422` means invalid.** A body that is not JSON, or a missing required field, is `400`. A
  well-formed request with a wrong value (an unknown enum value, a list over its cap) is `422`.
- **A `422` lists every problem, not only the first.** Validate the whole input and return one field error per
  problem. Each pointer follows the shape of the *request* JSON, array indexes included (`/filters/1/values/0`). The
  caller can then mark every bad field at once. Make the module's `ValidationError` able to hold several fields
  before your first form needs it.
- **A batch endpoint reports per item.** First check everything the whole request needs (shape, cap, permissions).
  A failure there is an ordinary problem response. Once the request is accepted, answer **`200`** with
  `{results: [{id, ok, error}]}`, even when every item failed. A `5xx` would tell a retry wrapper "nothing happened",
  which is false for items already written. One exception: when every item failed for the same outside cause (the
  upstream service is down), report that cause as a problem response before the items.

## Request ids *(aether)*

Put a `requestID` middleware **first** in the chain. It keeps an inbound `Request-Id` (the proxy may set one) or
creates a short random id, stores it on the request for the logger and the problem writer, and echoes it on the
response. Set `problemjson.Cfg.RequestID` to read it back, so a masked production error still carries a `reference`
that matches the full server log line. Set `problemjson.Cfg.OnProblem` to log the unmasked problem (detail, field
errors, upstream reason) under that reference: the masked body omits all of it. Give the request middleware the same
id (`middleware.Cfg.RequestID`) so the request log's `req-id` matches too. Only keep an inbound id that is short and
made of safe characters; generate one otherwise. *(aether, reference-app: `app/router/requestid.go`, `logProblem`)*

## Calling third-party services

- **Use `go-bumbu/http/outbound` for every outbound client** (`outbound.New(outbound.Cfg{Service, UserAgent, RPS})`).
  It throttles before sending, retries transient failures (5xx, 429, timeouts) a bounded number of times, honours a
  short `Retry-After`, and does not retry a refusal (other 4xx). Do not write your own retry and rate limiter.
  *(aether)*
- **Map its errors with `Writer.WriteUpstream`**: `429` when the provider rate-limits you, `504` on timeout, `502`
  otherwise. The `detail` is the error's `UserMessage()`, a sentence that names the service. Never a raw Go error.
- **Cache successful answers in front of the throttle**, so a cache hit uses no rate-limit token. Cache an empty
  match too: it is often the most expensive answer. Never cache a failure: a rate-limited lookup must stay
  retryable. *(aether: `identify.Cache`, `albumidentify.CachingReleaseLookup`)*
- **A URL that a user supplies goes through an SSRF guard.** Refuse loopback, private, link-local, multicast and
  reserved addresses, and treat an IPv4-mapped IPv6 address as its IPv4 address. Check in the dialer's `Control`
  hook, after DNS resolution, so DNS rebinding and redirects to internal hosts are caught too. Anything that is not
  an address is refused. *(codex: `internal/scrape/ssrf.go`)*

## Serving files that users uploaded *(codex)*

A user's file can be anything, HTML and SVG included. Serve it with one shared helper
(`httpx.SetServedFileHeaders`) that sets:

- `Content-Type` from the recorded type, `application/octet-stream` when none;
- `Content-Disposition` built with `mime.FormatMediaType`, so any file name comes back whole and cannot break the
  header;
- `Cache-Control: private` and `X-Content-Type-Options: nosniff`;
- `Content-Security-Policy: sandbox` for everything except PDF (the browser's PDF viewer refuses a sandboxed
  document).

The browser then runs no script from the file and gives it no origin.

## Body limits

`decodeJSON` caps a body at 1 MiB. When an area needs more (a wiki page, a captured web page), give that area its
own cap in the same helper. Do not raise the global one. Multipart uploads keep their own limits. *(codex:
`httpx.DecodeJSON`, 2 MiB for the wiki, 5 MiB for bookmarks)*

## gorilla/mux traps (already solved in `api_v0.go` / `router.go`)

- Do not mount the API on a `PathPrefix` subrouter. A later subroute match clears an earlier method mismatch, so a
  wrong method becomes `404` instead of `405`.
- A catch-all (the UI) must put its "not the API" matcher **first**, for the same reason.
- Set `NotFoundHandler` and `MethodNotAllowedHandler`, so API paths answer with problem+json and other paths with
  plain text.

## Contract tests (`app/router/openapi_test.go`)

These load and validate the spec with kin-openapi. The test resolves the spec path relative to the test file. It
builds the real router on a temporary SQLite database, with real modules. Then it checks two things: that the
documented and mounted (method, path) sets are equal, and that each response's status, content type and body
schema match the operation. Copy the file, and point `specPath` and the route walk at your API.

## Go client (`libs/client`)

Use only the stdlib. `New(baseURL, ...Option)` validates the URL. `WithHTTPClient` and `WithHeader` are the
options. There is one method per operation, and responses are read up to a 10 MiB limit. Errors are a `*Problem`
that `errors.Is` matches against `ErrNotFound`, `ErrUnauthorized` and `ErrValidation`. Treat it as the public SDK:
its exported types are a promise to its users.

## Versioning

`v0` makes no stability promise. For a breaking `v1`, write a new spec file, an `attachApiV1` with its own prefix,
its own contract tests and client methods. Also widen `isAPIPath` so the UI catch-all and the problem+json
404/405 handling cover both versions.
