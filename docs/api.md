# API

The API is designed first: [`openapi/api-v0.yaml`](openapi/api-v0.yaml) is the hand-written contract, and the routes in
`app/router/api_v0.go` implement it.

| Endpoint                  | Description                                          |
|---------------------------|------------------------------------------------------|
| `GET /api/v0/info`        | version, build time, commit and Go version (public)  |
| `GET /api/v0/me`          | the authenticated caller: user and tenant            |
| `GET /api/v0/notes`       | list the tenant's notes, newest first                |
| `POST /api/v0/notes`      | create a note                                        |
| `GET /api/v0/notes/{id}`  | get a note                                           |

* Spec paths are mount-relative; the `/api/v0` base lives only in the spec's `servers` entry.
* Errors are RFC 9457 problem details (`application/problem+json`, via
  [go-bumbu/http/problemjson](https://github.com/go-bumbu/http)); `Env.Production: true` masks their internal detail.
* Handlers map the module's types to their own request/response structs, so the wire format does not change when
  the module's model does.
* The spec is enforced by tests in `app/router/openapi_test.go`: every mounted route must be documented and vice versa,
  and real responses are validated against the documented schema, status and content type
  (with [kin-openapi](https://github.com/getkin/kin-openapi)).
* `make spec-lint` lints the spec with [spectral](https://github.com/stoplightio/spectral) and `.spectral.yaml`.
* `libs/client` is a hand-written Go client of the API (stdlib only). It shares no types with the server;
  `app/router/client_test.go` runs it against the real router, so a drift on either side fails there.
  API errors come back as `*client.Problem`, comparable with `errors.Is` to `client.ErrNotFound`,
  `ErrUnauthorized` and `ErrValidation`.
