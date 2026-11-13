# Web UI — a Vue SPA embedded in the Go binary

How a new app ships a frontend inside its single binary. Copy `webui/` (keep the layout and remove the notes views),
`app/spa/` (+ `spa_test.go`), the `attachUI` mount in `app/router/router.go`, and the `ui-*` / `package-ui` make
targets. Background: [../agents/web-ui.md](../agents/web-ui.md).

## Stack

Vue 3 + TypeScript + Vite, PrimeVue (Aura preset, theme in `src/theme.ts`, light/dark from the OS), TanStack Query,
axios, vue-router, Vitest + jsdom. This is the stack of [aether](https://github.com/andresbott/aether). Keep it
unless the app has a reason to differ.

## Layers in `webui/src/`

| Path | Rule |
|---|---|
| `lib/api/types.ts` | wire types written by hand from the OpenAPI spec; never imported from the server or generated |
| `lib/api/client.ts` | one axios instance on `/api/v0`, same origin |
| `lib/api/api.ts` | one plain async function per API operation; it unwraps envelopes (`items`) and escapes path params |
| `lib/apiError.ts` | the **only** code that reads problem+json: a user message, the status, per-field errors from JSON pointers |
| `composables/useApi.ts` | TanStack Query hooks, with all query keys in one `queryKeys` object |
| `views/`, `router/` | screens and routes, with a `NotFoundView` for unknown client routes |

- **Server state lives in TanStack Query, so there is no Pinia or other client store.** The defaults in `main.ts` are:
  no refetch on window focus, `retry: 1` for queries, no retry for mutations, a `staleTime` of 30 s.
- **A mutation updates exactly what it changed**: seed the detail query with the result, and invalidate the list
  with `exact: true`.
- **Do not retry a 404** (`retry: false` on detail queries).
- **Views never parse errors.** They call `apiErrorMessage` and `fieldErrors` and show the result, with toasts for
  general errors and inline messages for `422` fields.
- **A rate limit is not a failure.** Add an `isRateLimitError` (a `429`, or the `upstream_rate_limited` slug) to
  `apiError.ts`. A view that calls a third-party lookup then shows a warning that invites a retry instead of an
  error. *(aether)*
- **UI-only state** (a sidebar width, the theme, a collapsed panel) is not server state. Keep it in `localStorage`
  under an app-prefixed key (`<app>:sidebarWidth`), read it in a `try`, and fall back to a default. Use Pinia only if
  several components share such state. Server state never goes there.
- **Under `proxy` auth, a `401` reloads the page.** The SPA has no login screen, and the reload gets the proxy's
  login redirect. Only the `/me` call or a global response interceptor decides this, not each view. *(aether:
  `useAuth`, which reads the auth method from `/me`)*

## Trees and drag and drop *(reference-app tags)*

- **One editor dialog per tree**, opened where the tree shows (reference-app: the sidebar's pencil, the note page's
  Tags card). The sidebar only navigates and takes drops; nothing about the tree goes into the settings.
- **Keep tree rules in pure functions** (`lib/tagTree.ts`): which drop is allowed (no cycle, no no-op) and which
  PATCH it means, unit-tested without a DOM. Components only measure the pointer and call them.
- **Give each drag its own MIME type** (`application/x-refapp-note`, `…-tag`) and accept a drop only when
  `dataTransfer.types` holds it, so links, files and the other drag are ignored.
- **Every drag has a non-drag twin**: HTML5 drag fires neither on touch nor from the keyboard (reference-app: "Move
  to…" in the tag editor). Clear drop marks on `dragleave`, `dragend` and when the dialog closes.
- **Links that differ only by query** (filters of one list) go through `RouterLink`'s `custom` slot with your own
  `aria-current`: RouterLink ignores the query and marks them all active.
- **Each drop or pick awaits its own `mutateAsync`**, so quick repeats each get their feedback.
- jsdom has no `DragEvent` or `DataTransfer`: test with a fake (`__tests__/helpers.ts` `dispatchDrag`), and give a
  row a `getBoundingClientRect` to aim at.

## Embedding (`app/spa`)

- `make package-ui` builds `webui/` and copies `dist/` into `app/spa/files/ui`. `//go:embed all:files/ui` embeds it.
  Gitignore the copied files and keep a `.gitkeep`.
- If that directory holds no `index.html`, `spa.Handler` serves a `503` "not built" page instead of failing. A plain
  `go build` and the Go tests then work without Node.
- The SPA handler (`go-bumbu/http/spa`) serves assets by path and falls back to `index.html`, so client-side routes
  survive reloads and deep links.
- **Mount it as the catch-all for GET/HEAD outside the API, after the API routes**, with the "not an API path"
  matcher first. An unknown `/api/...` must stay a problem+json `404`, never `index.html`. Copy
  `TestUIIsTheCatchAllOutsideTheAPI`.
- Because the binary serves both, they share an origin, so no CORS and no runtime base-URL config.
- **Register the MIME types Go does not know** when you add PWA files to `webui/public/`: `.webmanifest`
  (`application/manifest+json`) and `.ico`. A manifest sniffed as `text/plain` turns off install and standalone
  mode. *(aether: `app/spa/spa.go`)*

## Look and feel

- **Write UI conventions down once views multiply.** A short, maintained registry per pattern (the layout of a main
  view, how editing works, where play/save actions sit) with a checklist for a new view. When a view and the
  registry disagree, the registry wins. *(aether: `docs/architecture/*.md`; codex: the mobile shell rules in its
  `CLAUDE.md`)*
- **Colour tokens are CSS variables with a value per theme.** PrimeVue's palette colours (`--p-red-*`) do not change
  with the theme, and no single red passes WCAG AA on both a light and a dark background. Define `--app-danger` and
  similar tokens per theme, and add a Vitest spec that checks their contrast. *(aether:
  `assets/scss/__tests__/themeContrast.spec.ts`)*
- **Self-host icons and fonts**, never from a CDN. The binary must work offline and behind a strict CSP. aether
  builds icon CSS only for the names the sources use, and the build **fails** on an unknown name, so a typo never
  ships as a blank icon.
- Generate the icon set (favicon, PWA sizes, Apple touch icon, maskable icons) from one master SVG with a make
  target, and commit the output. *(aether: `make icons`)*

## Development loop

- `make ui-prepare` once. Then run `make run` (the backend on `:8081`) and `make ui-dev` (Vite with hot reload). The
  dev server proxies `/api` to `BACKEND_URL`.
- To test `proxy` auth without a real proxy, the dev server injects the identity headers (reference-app: by default,
  and `make run` starts the backend in `proxy` mode): `DEV_USER=alice DEV_TENANT=acme npm run dev`. Keep the header
  names in `vite.config.ts` in step with your config, and pass a key-only path (`/api/key`) through without them.

## For each new API operation

Add the type to `types.ts`, the function to `api.ts`, a hook and key in `useApi.ts`, and a Vitest spec for the view.
`make ui-test` (`vue-tsc --noEmit` + `vitest run`) is part of `make verify` and of the `webui` CI workflow. That
workflow also runs `make package-ui`, so a broken production build fails before release.

## Keeping the SPA origin-agnostic *(aether)*

If the same build may one day run outside the Go binary (a desktop shell such as Tauri, or the UI zip served by
another host), keep it origin-agnostic from the start: no hard-coded origins, no server-side templating of
`index.html`, no logic that assumes cookies reach the API. Put every "where am I running" question in one small
host module (aether: `webui/src/lib/host/`), and pin that rule with a test that fails when other code reads
`location.origin`.

## Dropping the UI

Remove `webui/` and `app/spa/`, the `UI` field and `attachUI` from the router, `package-ui` from `build` and
`run-ui`, the `webui` workflow, `make package-ui` from `release.yml`, and the spa archive from `.goreleaser.yaml`.
