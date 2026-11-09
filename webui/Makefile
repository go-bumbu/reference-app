default: help

prepare: ## install the dependencies
	@npm install

fmt: ## format the code
	@npm run format

type-check: ## check the types
	@npm run type-check

run: ## run the dev server with hot reload; it proxies /api to the backend of `make run`
	@npm run dev

build: ## build a production release into dist/
	@npm run build

test: ## run the type check and the unit tests
	@npm test

test-watch: ## run the unit tests in watch mode
	@npm run test:watch

.PHONY: help
help: # Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
