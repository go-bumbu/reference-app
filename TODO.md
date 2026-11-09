<!-- todo:guide — managed by todo; this block is rewritten on save. Docs: https://github.com/candy-tools/todo
This file is a todo list managed by "todo", a terminal TODO app:
https://github.com/candy-tools/todo

todo watches this file and reloads it automatically when it changes on disk, so
you — human or agent — can edit it directly in any editor. Keep to this format
so todo can parse what you write:

  # Heading           Headings ("#" to "######") are categories; they nest by
                      heading level.
  - [ ] Open task     A "- [ ]" line is an open task; "- [x]" marks it done.
  - [/] In progress   "- [/]" flags a task in progress, "- [>]" defers it.
  - [x] Done task     Tasks must live under a category heading.
    - [ ] Subtask     Indent by two spaces to nest a subtask under a task.
    Description text  An indented, non-checkbox line is the task's description.

Notes for editors:
- Text above the first heading (this block included) is preserved on save.
- todo rewrites the file into the canonical form above on every change, so any
  other free-form markdown placed between items is not kept.

Prefer the CLI over editing by hand: it writes this file in the exact format
above. `todo add`, `todo done|progress|defer|reopen`, `todo edit`, `todo rm`
and `todo list --json` cover the common actions — run `todo --help`.
-->

# Verify and code quality

## Fix the verify process

- [ ] Run the test suite only once in verify
  Don't run both test and coverage; write one cover profile and have the coverage gate read it.
- [ ] Smoke-run benchmarks in verify with -benchtime=1x
  Shows the benchmarks still build and don't panic; keep the full benchmark run as its own target.
- [ ] Add a formatting gate via golangci v2 formatters
  gofumpt plus goimports/gci in the formatters: section, so golangci-lint run checks them.
- [ ] Clean dead golangci settings
  Settings for linters that aren't enabled (e.g. gocognit), linters listed twice (unused is in standard), non-linters in exclusions (noop).
- [ ] Pin tool versions in one place shared by local make and CI
  tools/go.mod used through go tool -modfile, or version vars in the Makefile that CI reads; keep tool deps out of a library's main go.mod.
- [ ] Make CI call the same make targets as local verify
  No separate lint action or install steps that can drift; trigger on pull_request too, not only push.
- [ ] Replace the awk coverage parser with go-test-coverage
  vladopajic/go-test-coverage: per-package and total thresholds plus exclusions from a config file.
- [ ] Make check-git-clean catch untracked files
  Use git status --porcelain instead of git diff --quiet.

## Cheap high-value checks

- [ ] go mod tidy -diff
  Fails when go.mod/go.sum aren't tidy.
- [ ] go mod verify
  Checks that downloaded modules match go.sum.
- [ ] govulncheck ./...
  Reports known CVEs only in reachable code.
- [ ] go test -race -shuffle=on
  Catches data races and tests that depend on run order.
- [ ] Generated-code freshness check
  go generate ./... then git diff --exit-code.
- [ ] Short fuzz runs for parsers and normalizers
  -fuzztime=10s.

## Linters to add

- [ ] Correctness linters
  errorlint, nilerr, nilnil, bodyclose, noctx, contextcheck, rowserrcheck/sqlclosecheck, forcetypeassert, exhaustive.
- [ ] Idiom and modern Go linters
  gocritic, revive, modernize, intrange, perfsprint, usestdlibvars, unconvert, unparam, errname.
- [ ] Test linters
  thelper, tparallel, testifylint when testify is used.
- [ ] Docs linters
  revive exported rule (doc comments on the public API), godot, misspell.
- [ ] Enforce architecture layers with depguard
  Turn documented layer rules (e.g. a go-deps-view config) into failing lint rules; go-arch-lint is the heavier option.

## Library-specific checks

- [ ] gorelease before tagging
  golang.org/x/exp/cmd/gorelease (built on apidiff): detects breaking API changes against the last tag and reports the next semver.
- [ ] go directive floor check
  Fail if go.mod's go line rises above the chosen minimum; go get bumps it silently and consumers inherit it.

## Repo hygiene

- [ ] actionlint and zizmor for GitHub workflows
  zizmor flags Actions security issues, such as actions pinned to a tag instead of a SHA.
- [ ] shellcheck for scripts
- [ ] typos or codespell for docs and comments
- [ ] gitleaks for committed secrets
- [ ] Renovate or Dependabot for dependency and Action updates

## Standardize across projects

- [ ] Shared base .golangci.yaml that each project extends
- [ ] Shared go.mk Makefile include
  Tool pins plus standard targets: fmt, lint, tidy-check, vuln, test-race, coverage, verify.
- [ ] Reusable GitHub workflow that calls make verify
- [ ] Split a fast make check from the full make verify
  check (fmt, tidy, lint, unit tests with -race) for pre-commit via lefthook; verify for CI and before tagging.

# pending

- [ ] docker compose
- [ ] docker build
- [ ] CI release
- [ ] go-debs-view

# next itrataion

- [x] create a simple webui app based on aether
- [x] add multi tenantcy
  - look into codex to manage tenants and users
  - goal is to have N tenants and each tenant can have M users
- [x] diffeernt auth methods
  3 main auth methods:
  - none => default tenant and user, used of demo
  - forwarded header => used ina proxy setup, the proxy send the relevant headers
  - oidc => just placeholder for now
- [x] in libs we want a sample http client library for the api
