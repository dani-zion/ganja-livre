.PHONY: help dev down down-volumes generate generate-api generate-web \
        backend-build backend-lint backend-test backend-sec backend-tidy \
        frontend-install frontend-dev frontend-build frontend-lint clean gen-secrets

BACKEND  = backend
FRONTEND = frontend

BINARY_API    = $(BACKEND)/dist/server
BINARY_WORKER = $(BACKEND)/dist/worker

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
	  awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

# ── Local development ─────────────────────────────────────────────────────────

dev: ## Start full stack with Docker Compose (hot-reload)
	@cp -n .env.example .env 2>/dev/null || true
	docker compose up --build

down: ## Stop and remove containers (keep volumes)
	docker compose down

down-volumes: ## Stop containers AND delete all data volumes
	docker compose down -v

# ── Code generation ───────────────────────────────────────────────────────────

generate: generate-api generate-web ## Regenerate both GraphQL server and web client
	@echo "Generated API and web code"

generate-api: ## Run gqlgen to regenerate GraphQL server code
	cd $(BACKEND) && go tool gqlgen generate

generate-web: ## Run GraphQL codegen to regenerate the typed web client
	cd $(FRONTEND) && npm run generate

# ── Backend ───────────────────────────────────────────────────────────────────

backend-build: ## Compile both binaries to backend/dist
	@mkdir -p $(BACKEND)/dist
	cd $(BACKEND) && CGO_ENABLED=0 go build -ldflags="-w -s" -o dist/server ./cmd/server
	cd $(BACKEND) && CGO_ENABLED=0 go build -ldflags="-w -s" -o dist/worker ./cmd/worker
	@echo "Built: $(BINARY_API)  $(BINARY_WORKER)"

backend-lint: ## Run golangci-lint
	cd $(BACKEND) && golangci-lint run ./...

backend-test: ## Run backend tests with race detector
	cd $(BACKEND) && go test -race -cover ./...

backend-sec: ## Run gosec security scanner
	cd $(BACKEND) && gosec -quiet ./...

backend-tidy: ## Tidy backend go.mod/go.sum
	cd $(BACKEND) && go mod tidy

# ── Frontend ──────────────────────────────────────────────────────────────────

frontend-install: ## Install frontend dependencies
	cd $(FRONTEND) && npm install

frontend-dev: ## Start Vite dev server (hot-reload)
	cd $(FRONTEND) && npm run dev

frontend-build: ## Type-check and build frontend for production
	cd $(FRONTEND) && npm run build

frontend-lint: ## Run ESLint on the frontend
	cd $(FRONTEND) && npm run lint

# ── Utilities ─────────────────────────────────────────────────────────────────

clean: ## Remove build artifacts
	rm -rf $(BACKEND)/dist/ $(FRONTEND)/dist/

gen-secrets: ## Print random JWT secrets to copy into .env
	@echo "JWT_ACCESS_SECRET=$$(openssl rand -hex 64)"
	@echo "JWT_REFRESH_SECRET=$$(openssl rand -hex 64)"
	@echo "MONGO_INITDB_ROOT_PASSWORD=$$(openssl rand -hex 32)"
	@echo "TEMPORAL_DB_PASSWORD=$$(openssl rand -hex 32)"
