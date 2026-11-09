# Web UI — Vue SPA embedded in the binary

`webui/` is a small Vue 3 single-page app on the stack of [aether](https://github.com/andresbott/aether): TypeScript,
Vite, PrimeVue (Aura preset, `c` prefix), TanStack Query for server state, axios, Vitest. It lists and creates the
notes of the caller's tenant and shows who is signed in. `app/spa` embeds its build into the Go binary.
See [api.md](api.md) for the contract it consumes.

## Layout

| Path | Role |
|---|---|
| `src/lib/api/types.ts` | wire types, hand-written from the spec (like `libs/client`, no shared types) |
| `src/lib/api/client.ts`, `api.ts` | one axios instance on `/api/v0`; one function per operation |
| `src/lib/apiError.ts` | **the one place** that reads problem+json: `apiErrorMessage`, `apiErrorStatus`, `fieldErrors` |
| `src/composables/useApi.ts` | TanStack Query hooks and `queryKeys`; mutations invalidate exactly what they change |
| `src/App.vue` | the shell, after codex: top bar (brand, **Back** off the notes list, search, **Add note**, profile) over a collapsible sidebar and the scrolling content; the settings pages drop the search |
| `src/components/ProfileMenu.vue` | the top bar's profile button (the user's initial): a popup menu with who is signed in (subject, tenant) and Settings. The mode and theme are set only in Settings → Appearance |
| `src/components/AppNav.vue` | the sidebar: tags only — **placeholders for now**, an inert fixed list until the API serves tags |
| `src/components/NoteDialog.vue` | the note form, in a dialog: blank, it creates (the top bar's **Add note**, from any page); given a `note`, it edits that note (the note page's **Edit**). `NotesView` is a read-only card grid |
| `src/components/SearchField.vue` | the top-bar search input; the shell binds it to the `?q=` query parameter |
| `src/composables/useSidebar.ts`, `useTheme.ts` | module-level UI preferences kept in `localStorage`: sidebar collapsed; light / dark / auto mode and accent theme (`src/theme.ts` holds the palettes) |
| `src/views/settings/`, `src/components/settings/` | the settings, after codex: `SettingsShell` (title + left nav) around Appearance (theme, mode), Jobs (**placeholder**: sample jobs, Run now only toasts) and About; the shell hides the tags rail there |
| `src/views/NoteView.vue` | one note, after codex's file and wiki pages: a read-only A4 sheet with a details card on its right, Edit / Delete at the card's foot. Edit opens `NoteDialog` (no in-place editing); delete asks through the shell's `ConfirmDialog` |
| `src/views/`, `src/router/` | screens and routes |
| `src/__tests__/` | Vitest specs (kept despite the `**/__tests__` gitignore rule, via an explicit exception) |

## Invariants

- **Server state lives in TanStack Query; there is no client-side store.** Query defaults in `main.ts`: no refetch on
  focus, `retry: 1`, `staleTime` 30 s; mutations never retry. `useNote` sets `retry: false`: a `404` will not turn
  into a `200` by asking again.
- **UI state lives in the URL or in a composable, not a store.** The search query is `?q=` (typing on another page
  goes to the notes list, which filters on the client: the API returns every note in one page). The sidebar's
  collapsed state is a `useSidebar` singleton kept in `localStorage`, as are the mode and theme (`useTheme`, applied
  in `main.ts` after PrimeVue installs, whose install would reset a swapped primary palette).
- **Views never parse errors themselves**; they call `apiError.ts`. A `422` maps to per-field messages through the
  problem's JSON pointers (`/title` → `title`).
- **Same origin in production**: the UI is served by the Go binary, so no CORS and no base URL config.
- **The router mounts the UI as the catch-all for GET/HEAD outside `/api`**, after the API routes, so deep links and
  reloads reach `index.html` while an unknown API path stays a problem+json `404` (`TestUIIsTheCatchAllOutsideTheAPI`).

## Build and embed

- `make package-ui` runs `npm ci && npm run build` and copies `webui/dist/` into `app/spa/files/ui`, which
  `app/spa` embeds with `go:embed`. Its contents are gitignored except `.gitkeep`.
- A binary built without it serves a `503` "not built" page instead of failing, so a plain `go build` keeps working.
- `make build`, `make run-ui` and the release workflow run `package-ui` first — every release path must keep the
  `package-ui → goreleaser` order ([releasing.md](releasing.md)).

## Development

- `make ui-prepare` once, then `make run` in one terminal and `make ui-dev` in another: Vite hot reload, proxying
  `/api` to `BACKEND_URL` (default `http://localhost:8081`, the `make run` port).
- With the backend in `proxy` mode, the dev server plays the proxy: `DEV_USER=alice DEV_TENANT=acme npm run dev`
  (in `webui/`) sends them as `Remote-User` / `Remote-Tenant` — keep those names in step with `zarf/config.yaml`.
- `make ui-test` runs `vue-tsc --noEmit` and `vitest run`; it is part of `make verify` and the `webui` workflow.
