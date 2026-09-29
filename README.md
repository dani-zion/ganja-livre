# 🌿 Ganja Livre

A secure, scalable GraphQL API for cannabis retail — built in Go — plus its React web frontend.
Architecture mirrors the reliability of large marketplace platforms, with Temporal.io handling all order lifecycle orchestration.

Monorepo layout: `backend/` (Go API + Temporal worker) and `frontend/` (Vite + React + TypeScript), sharing the GraphQL schema as the API contract.

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                          CLIENT / MOBILE                         │
└──────────────────────────────┬──────────────────────────────────┘
                               │ HTTPS  POST /query
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│                        API SERVER (Go)                           │
│                                                                  │
│  ┌────────────┐  ┌──────────────┐  ┌────────────────────────┐  │
│  │ Chi Router │→ │  JWT Auth MW │→ │   GraphQL (gqlgen)     │  │
│  └────────────┘  └──────────────┘  └──────────┬─────────────┘  │
│                                               │                  │
│         ┌──────────────────────────────────── ┤                  │
│         ▼                                     ▼                  │
│  ┌─────────────┐                    ┌─────────────────────┐     │
│  │  Products   │                    │  Orders Resolver    │     │
│  │  Resolver   │                    │  → Temporal Client  │     │
│  └──────┬──────┘                    └────────┬────────────┘     │
└─────────┼──────────────────────────────────── ┼ ────────────────┘
          │                                      │
          ▼                                      ▼
┌──────────────────┐                  ┌──────────────────────────┐
│    MongoDB       │                  │     Temporal Server      │
│  ┌────────────┐  │                  │  ┌────────────────────┐  │
│  │   users    │  │                  │  │  OrderWorkflow     │  │
│  │  products  │  │◄─────────────────┤  │  ┌─────────────┐  │  │
│  │   orders   │  │  Activities read │  │  │ReserveStock │  │  │
│  └────────────┘  │  / write directly│  │  │UpdateStatus │  │  │
└──────────────────┘                  │  │  │NotifySeller │  │  │
                                      │  │  │  ShipOrder  │  │  │
                                      │  │  └─────────────┘  │  │
                                      │  └────────────────────┘  │
                                      │                           │
                                      │  ┌────────────────────┐  │
                                      │  │  Temporal Worker   │  │
                                      │  │  (separate pod)    │  │
                                      │  └────────────────────┘  │
                                      └──────────────────────────┘
```

---

## Order Lifecycle (Temporal Workflow)

```
PlaceOrder ──► [PENDING]
                   │
                   ▼
          ReserveStock (MongoDB TX)
                   │
                   ▼
         [PAYMENT_PROCESSING]
                   │
           ┌───────┴──────────┐
           │  await signal    │ (max 30 min)
           │                  │
     payment-confirmed   order-cancelled / timeout
           │                  │
           ▼                  ▼
  [PAYMENT_CONFIRMED]   ReleaseStock
           │                  │
           ▼                  ▼
     RecordPayment       [CANCELLED]
           │
           ▼
      [PREPARING]
           │
           ▼
       ShipOrder
           │
           ▼
       [SHIPPED]
           │
     await signal (15 days)
           │
           ▼
      [DELIVERED]
```

---

## Project Structure

```
ganja-livre/
├── backend/                 # Go API + Temporal worker
│   ├── cmd/
│   │   ├── server/          # HTTP + GraphQL entrypoint
│   │   └── worker/          # Temporal worker entrypoint
│   ├── internal/
│   │   ├── auth/            # JWT issuance & validation
│   │   ├── config/          # Env-based config loader (incl. CORS allow-list)
│   │   ├── graph/
│   │   │   ├── model/       # Domain models (User, Product, Order)
│   │   │   ├── generated/   # gqlgen output
│   │   │   └── resolvers/   # GraphQL resolvers (schema.resolvers.go)
│   │   ├── middleware/      # JWT auth, CORS, security headers
│   │   ├── mongodb/         # Client, indexes, collection helpers
│   │   ├── temporal/
│   │   │   └── workflows/   # OrderWorkflow
│   │   └── validator/       # Input validation (email, password, price…)
│   ├── schema.graphql       # Single source of truth for the API contract
│   ├── gqlgen.yml           # Server code generation config
│   ├── go.mod  go.sum
│   └── Dockerfile           # Multi-stage: builder → scratch images
├── frontend/                # Vite + React + TypeScript web app
│   ├── src/
│   │   ├── graphql/         # Documents + codegen output (generated/)
│   │   ├── lib/             # GraphQL client (graphql-request)
│   │   └── features/        # Feature modules (grows over time)
│   ├── codegen.ts           # Typed client generation from ../backend/schema.graphql
│   ├── vite.config.ts       # Dev server + /query proxy → API
│   ├── Dockerfile           # dev / build / prod (nginx) stages
│   └── nginx.conf           # Static serving + /query reverse proxy
├── scripts/
│   └── mongo-init.js        # DB user + collection validation rules
├── config/                  # Temporal dynamic config
├── docker-compose.yml       # Full local stack (API, worker, web, infra)
├── Makefile                 # Developer workflow (namespaced targets)
└── .env.example             # Config template
```

---

## Quick Start

### Prerequisites
- Docker & Docker Compose v2
- Go 1.25+ (for local backend development)
- Node 20+ & npm (for local frontend development)
- `make`

### 1. Configure secrets

```bash
cp .env.example .env

# Generate strong secrets
make gen-secrets
# Copy the output values into your .env
```

### 2. Run the stack

```bash
make dev
```

Services:
| Service | URL |
|---|---|
| Web App | http://localhost:5173 |
| GraphQL API | http://localhost:8080/query |
| GraphQL Playground | http://localhost:8080/playground |
| Temporal UI | http://localhost:8088 |
| MongoDB | mongodb://localhost:27017 |

The web app runs Vite with hot-reload; its dev server proxies `/query` to the API
(no CORS needed during development). The API also accepts direct cross-origin
requests from origins listed in `CORS_ALLOWED_ORIGINS`.

### 3. Frontend setup (local, outside Docker)

```bash
make frontend-install   # npm install
make frontend-dev       # Vite dev server (runs codegen first)
```

### 4. Generate GraphQL code

```bash
make generate       # both sides
make generate-api   # gqlgen → backend server code
make generate-web   # GraphQL codegen → typed frontend client
```

The frontend client is generated from `backend/schema.graphql` directly, so
schema changes are caught at compile time in both projects.

---

## GraphQL Examples

### Register
```graphql
mutation {
  register(input: {
    email: "user@example.com"
    password: "Secure123"
    name: "João Silva"
  }) {
    accessToken
    refreshToken
    user { id name role }
  }
}
```

### Login
```graphql
mutation {
  login(input: { email: "user@example.com", password: "Secure123" }) {
    accessToken
    refreshToken
  }
}
```

### Browse Products
```graphql
query {
  products(
    filter: { category: FLOWER, maxPrice: 100 }
    first: 10
  ) {
    totalCount
    edges {
      cursor
      node {
        id name price thcContent cbdContent stock
      }
    }
    pageInfo { hasNextPage endCursor }
  }
}
```

### Place Order (requires Bearer token)
```graphql
mutation {
  placeOrder(input: {
    items: [{ productID: "<id>", quantity: 2 }]
    shippingAddress: {
      street: "Rua das Flores"
      number: "42"
      neighborhood: "Centro"
      city: "Salvador"
      state: "BA"
      zipCode: "40000-000"
      country: "Brasil"
    }
  }) {
    id
    status
    totalAmount
    temporalWorkflowID
  }
}
```

---

<!-- ## Security Notes

- **Passwords** hashed with `bcrypt` (cost 12). Timing-safe login prevents user enumeration.
- **JWT**: Short-lived access tokens (15 min) + long-lived refresh tokens (7 days) with separate secrets.
- **RBAC**: Role enforcement inside resolvers, not just middleware.
- **MongoDB**: App user has `readWrite` only — no cluster admin. Schema-level validation enforced by MongoDB validators.
- **Input validation**: All user inputs are validated before any DB operation.
- **Price integrity**: Order prices are resolved from the DB, not trusted from the client.
- **Rate limiting**: Per-IP token bucket on all routes. Replace with Redis-backed limiter for multi-instance deployments.
- **Security headers**: `X-Content-Type-Options`, `X-Frame-Options`, `HSTS`, `CSP` on every response.
- **Docker**: Final images built `FROM scratch` — no shell, no package manager, minimal attack surface.
- **GraphQL**: Introspection disabled in production. Query complexity limit of 100 prevents DoS.
- **Temporal**: Idempotent workflows with retry policies. Stock reservation uses MongoDB transactions.

---

## Running Tests

```bash
make backend-test    # all backend tests with race detector
make backend-lint    # golangci-lint
make backend-sec     # gosec security scanner
make frontend-lint   # ESLint
make frontend-build  # type-check + production build
```

---

## Cloud Deployment Notes

The stack is cloud-ready out of the box:

- **API & Worker** compile to static binaries → deploy as Docker containers to ECS, GKE, or Fly.io
- **MongoDB** → swap `MONGODB_URI` for a MongoDB Atlas connection string (TLS included)
- **Temporal** → use [Temporal Cloud](https://temporal.io/cloud) and update `TEMPORAL_HOST_PORT` + `TEMPORAL_NAMESPACE`
- **Secrets** → inject via AWS Secrets Manager, GCP Secret Manager, or Kubernetes Secrets
- **Rate limiter** → swap the in-memory token bucket for `go-redis/redis_rate` when running multiple replicas -->
