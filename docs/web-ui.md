# Web UI

`webui/` is a small Vue 3 single-page app on the stack of [aether](https://github.com/andresbott/aether):
TypeScript, Vite, PrimeVue (Aura preset), TanStack Query for server state and axios, tested with Vitest.
It lists and creates the notes of the caller's tenant and shows who is signed in.

* `make package-ui` builds it and copies it into `app/spa/files/ui`, which `app/spa` embeds with `go:embed`;
  `make build` and goreleaser run it first. A binary built without it serves a "not built" page instead.
* The router mounts the UI as the catch-all for GET/HEAD outside `/api`, so deep links and reloads reach
  `index.html` while an unknown API path stays a problem+json 404.
* `make ui-dev` runs the Vite dev server with hot reload, proxying `/api` to `make run` (`localhost:8081`).
  With the backend in `proxy` mode, the dev server can play the proxy:
  `DEV_USER=alice DEV_TENANT=acme npm run dev` (in `webui/`).
* `make ui-test` type-checks and runs the unit tests; it is part of `make verify`.
* Wire types live in `webui/src/lib/api/types.ts`, hand-written like `libs/client`; errors are read in one
  place, `webui/src/lib/apiError.ts`.
