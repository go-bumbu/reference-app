COMMIT_SHA_SHORT ?= $(shell git rev-parse --short=12 HEAD)
PWD_DIR := ${CURDIR}

default: help

#==========================================================================================
##@ Testing
#==========================================================================================
test: ## run fast go tests
	@go test ./... -cover

test-alldbs: ## run go tests on every database, Postgres included (needs docker)
	@TESTDBS_ALL=1 go test ./... -cover

lint: ## run go linter
	# depends on https://github.com/golangci/golangci-lint
	@golangci-lint run

benchmark: ## run go benchmarks
	@go test -run=^$$ -bench=. ./...

spec-lint: ## lint the OpenAPI spec against .spectral.yaml (needs node/npx)
	@npx -y @stoplight/spectral-cli@6.15.0 lint docs/openapi/api-v0.yaml --ruleset .spectral.yaml --fail-severity=warn

license-check: ## check for invalid licenses
	# depends on : https://github.com/elastic/go-licence-detector
	@go list -m -mod=readonly  -json all  | go-licence-detector -includeIndirect -rules zarf/allowedLicenses.json

COVERAGE_THRESHOLD ?= 70
.PHONY: coverage
coverage:
	@fail=0; \
	for pkg in $$(go list ./internal/... ./libs/... 2>/dev/null); do \
		go test -coverprofile=coverage.out -covermode=atomic $$pkg > /dev/null; \
		if [ -f coverage.out ]; then \
			coverage=$$(go tool cover -func=coverage.out | grep total: | awk '{print $$3}' | sed 's/%//'); \
			if [ $$(echo "$$coverage < $(COVERAGE_THRESHOLD)" | bc -l) -eq 1 ]; then \
				echo "❌ Coverage in $$pkg is below $(COVERAGE_THRESHOLD)!"; \
				fail=1; \
			fi; \
			rm -f coverage.out; \
		else \
			echo "⚠️ No coverage data for $$pkg"; \
			fail=1; \
		fi; \
	done; \
	exit $$fail

ui-test: ## run the web UI type check and unit tests (needs node)
	@cd webui && npm test

.PHONY: verify
verify: ## run all checks; runs every check and fails if any fail
	@fail=0; \
	for target in test license-check lint benchmark coverage spec-lint ui-test; do \
		echo "==================== make $$target ===================="; \
		$(MAKE) --no-print-directory $$target || fail=1; \
	done; \
	if [ $$fail -ne 0 ]; then \
		echo "❌ verify failed (see above)"; \
		exit 1; \
	fi; \
	echo "✅ verify passed"

cover-report: ## generate a coverage report
	go test -covermode=count -coverpkg=./... -coverprofile cover.out  ./...
	go tool cover -html cover.out -o cover.html
	open cover.html

#==========================================================================================
##@ Running
#==========================================================================================
# Local data of `make run`: the SQLite dir of zarf/config.yaml (db, -wal, -shm) and
# the marker telling the demo data was added to it. `make clean` deletes it all.
RUN_DATA_DIR := zarf/appData/sqlite
DEMO_DATA_MARKER := $(RUN_DATA_DIR)/.demo-data

run: ## start the GO service, adding demo data on the first run
	@if [ ! -f $(DEMO_DATA_MARKER) ]; then \
		go run main.go seed -c zarf/config.yaml && touch $(DEMO_DATA_MARKER); \
	fi
	@REFAPP_ENV_LOGLEVEL="debug" go run main.go start -c zarf/config.yaml

run-ui: package-ui run ## build the web UI, then start the GO service with it embedded

#==========================================================================================
##@ Web UI
#==========================================================================================
ui-prepare: ## install the web UI dependencies
	@cd webui && npm ci

ui-dev: ## run the web UI dev server with hot reload, proxying /api to `make run`
	@cd webui && npm run dev

package-ui: ## build the web UI and copy it into app/spa/files/ui for embedding
	@cd webui && npm ci && npm run build
	@rm -rf app/spa/files/ui
	@mkdir -p app/spa/files/ui
	@cp -r webui/dist/. app/spa/files/ui/
	@touch app/spa/files/ui/.gitkeep


#==========================================================================================
##@ Building
#==========================================================================================

build: package-ui ## use goreleaser to build to current OS/Arch, web UI included
	@goreleaser build --auto-snapshot --clean --single-target

#==========================================================================================
##@ Release
#==========================================================================================
# Releases are built in CI: pushing a v*.*.* tag runs .github/workflows/release.yml
# (goreleaser: zips, debs, the ghcr.io container image, the web UI zip).

.PHONY: check-branch
check-branch:
	@current_branch=$$(git symbolic-ref --short HEAD) && \
	if [ "$$current_branch" != "main" ]; then \
		echo "Error: You are on branch '$$current_branch'. Please switch to 'main'."; \
		exit 1; \
	fi

.PHONY: check-git-clean
check-git-clean: # check if git repo is clean
	@git diff --quiet

tag: check-git-clean check-branch ## create a git tag to publish a new release
	@[ "${version}" ] || ( echo ">> version is not set, usage: make tag version=\"v1.2.3\" "; exit 1 )
	@echo "$(version)" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$$' || ( echo ">> version \"$(version)\" must look like v1.2.3 (leading v): release.yml only publishes v*.*.* tags"; exit 1 )
	@git tag -d $(version) || true
	@git tag -a $(version) -m "Release version: $(version)"
	@git push --delete origin $(version) || true
	@git push origin $(version) || true

clean: ## remove build output and the local database and job logs of `make run` (demo data included)
	@rm -rf dist $(RUN_DATA_DIR) zarf/appData/job-logs


#==========================================================================================
#  Help
#==========================================================================================
.PHONY: help
help: # Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
