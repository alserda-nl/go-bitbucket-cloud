GOLANGCI_VERSION := 2.12.2

help: ## Show this help.
	@egrep -h '\s##\s' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m  %-30s\033[0m %s\n", $$1, $$2}'

.PHONY: lint
lint: ## Run golangci-lint
	@golangci-lint run --timeout 5m

.PHONY: test
test: ## Run every test: the unit tests and the end-to-end tests against the fake
	@go test -count=1 -race ./...

.PHONY: cover
cover: ## Run every test and print how much of client, models and fakebitbucket they cover
	@go test -count=1 -race -coverpkg=./client/...,./models/...,./fakebitbucket/... -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | tail -n 1

.PHONY: install-golangci
install-golangci: ## Install golangci-lint
	@curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $$(go env GOPATH)/bin v$(GOLANGCI_VERSION)
