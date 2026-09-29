SHELL := /bin/bash
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)

.PHONY: help
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

# ---------------------------------------------------------------- codec ----

.PHONY: vectors
vectors: ## Regenerate the codec's shared artifacts and vectors
	cd backend && go run ./cmd/genvectors

.PHONY: conformance
conformance: ## Verify Go and TypeScript agree; fails if vectors are stale
	cd backend && go run ./cmd/genvectors -skip-bitmap
	@if ! git diff --quiet -- spec/vectors backend/internal/codec/spec_pins.go backend/internal/codec/tables/validity.bin; then \
		echo "ERROR: codec artifacts drifted. Commit the regenerated files."; \
		git --no-pager diff --stat -- spec/vectors backend/internal/codec; \
		exit 1; \
	fi
	@echo "codec artifacts are in sync"
	cd frontend && npx vitest run src/codec/__tests__/

# ---------------------------------------------------------------- build ----

.PHONY: web
web: ## Build the frontend into the Go embed directory
	cd frontend && npm ci && npm run build

.PHONY: build
build: ## Build the local binary (embeds the current frontend dist)
	cd backend && go build -trimpath -o bin/qrforqa ./cmd/server

.PHONY: build-linux
build-linux: ## Cross-compile a static linux/amd64 binary for the server
	cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-trimpath -tags 'netgo osusergo' \
		-ldflags "$(LDFLAGS)" -o ../dist/qrforqa-linux-amd64 ./cmd/server
	@ls -lh dist/qrforqa-linux-amd64

# ---------------------------------------------------------------- test -----

.PHONY: test
test: ## Run all tests (Go + TypeScript)
	cd backend && go test ./... -count=1
	cd frontend && npx vitest run

.PHONY: check
check: ## Vet, typecheck, and run the full test suite
	cd backend && go vet ./...
	cd backend && go test ./... -count=1
	cd frontend && npm run typecheck
	cd frontend && npx vitest run

.PHONY: dev
dev: ## Run the backend and the Vite dev server together
	@echo "backend on :8080, frontend on :5173 (proxying /api)"
	@(cd backend && DEV=1 go run ./cmd/server) & \
	 (cd frontend && npm run dev) ; \
	 wait

# ---------------------------------------------------------------- deploy ---

.PHONY: deploy
deploy: build-linux ## Build and deploy to the production server
	./scripts/deploy.sh

.PHONY: smoke
smoke: ## Run the end-to-end smoke test against a target URL
	python3 scripts/smoke.py $${BASE_URL:-http://127.0.0.1:8080}
