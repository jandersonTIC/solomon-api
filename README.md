# Solomon API

Go REST API backend for the Solomon personal finance app. Replaces a Google Sheets workflow for tracking income and expenses.

## Architecture

All reads are served from an **in-memory store** — zero database queries on GET requests. PostgreSQL is used only for durability via async write-behind persistence. This achieves sub-1ms p99 response times for all read operations.

```
Read path (< 1ms):
  Request → Auth middleware → In-memory store → JSON encode → Response

Write path (< 5ms perceived):
  Request → Auth middleware → Update memory → Response → Async persist to PostgreSQL
```

## For AI Agents

This project is primarily maintained by AI agents (Claude Code). Human readability is secondary to performance.

### Key decisions to preserve

- **Flat structure**: all Go files in `package main` at the root. No sub-packages.
- **3 external deps only**: `pgx/v5` (Postgres), `golang-jwt/v5` (auth), `goccy/go-json` (fast JSON). Do not add dependencies without strong justification.
- **No ORM, no framework**: stdlib `net/http` with Go 1.22+ pattern matching. Keep it this way.
- **Integer money**: all monetary values are `int64` cents. Never use float for money.
- **Pre-computed aggregates**: month summaries and category breakdowns are updated on every write, not computed on read.
- **Trunk-based development**: small, buildable commits pushed directly to `main`. Each commit must compile and pass `go test -race ./...`.

### File map

| File | Purpose |
|------|---------|
| `main.go` | Config, DB pool, store init, router, server startup, graceful shutdown |
| `model.go` | All struct definitions (Transaction, User, Account, MonthSummary, request/response types) |
| `store.go` | In-memory store: `sync.RWMutex` + maps, pre-computed aggregates, month duplication |
| `persist.go` | Write-behind async PostgreSQL persistence with 100ms batch coalescing, store loader |
| `migrate.go` | Embedded SQL migration runner |
| `auth.go` | Apple Sign-In JWT validation (cached public keys), session JWT issuance |
| `handler_tx.go` | Transaction CRUD handlers + `readJSON` helper shared by all handlers |
| `handler_month.go` | Month summary, categories, and duplication handlers |
| `handler_account.go` | Bank account CRUD handlers |
| `resp.go` | `writeJSON` and `writeErr` response helpers |
| `middleware.go` | JWT auth (extracts user_id, no DB), request timing, CORS |
| `sql/001_init.sql` | PostgreSQL schema (users, accounts, transactions, month_balances) |
| `testutil_test.go` | Shared test helpers: `newTestServer`, `testToken`, `fmtID` |
| `*_test.go` | Unit tests and benchmarks for each component |

### API endpoints

```
GET    /health
POST   /v1/auth/apple                     # Apple Sign-In token exchange (unprotected)
GET    /v1/transactions?ym=202603&type=1   # list by month, optional type filter
POST   /v1/transactions                    # create
PUT    /v1/transactions/{id}               # update
DELETE /v1/transactions/{id}               # delete
PATCH  /v1/transactions/{id}/status        # toggle confirmed/pending
GET    /v1/months/{ym}/summary             # pre-computed month summary
POST   /v1/months/{ym}/duplicate           # duplicate month for forecasting
GET    /v1/months/{ym}/categories          # category breakdown
GET    /v1/accounts                        # list accounts
POST   /v1/accounts                        # create account
PUT    /v1/accounts/{id}                   # update account
DELETE /v1/accounts/{id}                   # delete account
```

All `/v1/*` endpoints (except auth) require `Authorization: Bearer <jwt>`.

### Adding a new migration

Create `sql/002_<name>.sql`. The migration runner auto-discovers embedded SQL files and applies them in order. No code changes needed.

---

## For Humans

When AI agent APIs are unavailable and you need to work on this directly.

### Prerequisites

- **Go 1.22+** (project uses `go 1.26.1` but any 1.22+ works for the routing patterns)
- **PostgreSQL 15+** running locally
- **Docker** (only needed for integration tests with testcontainers)

### Quick start

```bash
# 1. Start PostgreSQL (pick one)
# Option A: Docker
docker run -d --name solomon-pg \
  -e POSTGRES_USER=solomon \
  -e POSTGRES_PASSWORD=solomon \
  -e POSTGRES_DB=solomon \
  -p 5432:5432 \
  postgres:17

# Option B: Use an existing PostgreSQL instance and set DATABASE_URL

# 2. Run the server
go run .

# 3. Server is now at http://localhost:8080
curl http://localhost:8080/health
```

### Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP listen port |
| `DATABASE_URL` | `postgres://solomon:solomon@localhost:5432/solomon?sslmode=disable` | PostgreSQL connection string |
| `JWT_SECRET` | `dev-secret-change-in-prod` | HMAC secret for session JWTs |

### Running tests

```bash
# Unit tests (no DB required)
go test ./... -count=1

# With race detector
go test -race ./... -count=1

# Benchmarks
go test -bench=. -benchmem ./...
```

### Testing endpoints manually

```bash
# Generate a dev JWT (valid for user_id=1)
# Using Go: or use https://jwt.io with HS256 and your JWT_SECRET
# Payload: {"uid": 1, "exp": <future_timestamp>}

TOKEN="<your_jwt_here>"

# Create a transaction
curl -X POST http://localhost:8080/v1/transactions \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "type": 2,
    "status": 0,
    "date": "2026-03-10",
    "description": "IPTU 3/12",
    "category": "Moradia",
    "amount_cents": 45000,
    "currency": "BRL",
    "year_month": 202603
  }'

# List transactions for a month
curl http://localhost:8080/v1/transactions?ym=202603 \
  -H "Authorization: Bearer $TOKEN"

# Get month summary
curl http://localhost:8080/v1/months/202603/summary \
  -H "Authorization: Bearer $TOKEN"

# Duplicate month (forecast next month)
curl -X POST http://localhost:8080/v1/months/202603/duplicate \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"target_year_month": 202604}'
```

### How things connect

1. **Startup**: `main.go` connects to PostgreSQL, runs migrations, loads ALL data into memory, starts the HTTP server.
2. **Reads**: Go directly to the in-memory `Store` (maps protected by `sync.RWMutex`). No SQL queries.
3. **Writes**: Update the in-memory store first (user gets immediate response), then the `Persister` batches the SQL write asynchronously every 100ms.
4. **Auth**: Apple Sign-In gives the iOS app an identity token → backend validates it → issues a session JWT → all subsequent requests carry that JWT. The middleware extracts `user_id` from the JWT, no DB lookup.
5. **Month duplication**: Copies all transactions from month A to month B, advances installment counters, skips completed installments, carries over the final balance.

### Transaction types and statuses

- **Type**: `1` = income, `2` = expense
- **Status**: `0` = pending (not confirmed), `1` = confirmed
- **Recurrence**: `0` = none, `1` = fixed installments, `2` = variable
- **year_month**: integer format `YYYYMM` (e.g., `202603` for March 2026)
- **amount_cents**: integer in cents (e.g., `893295` = R$ 8.932,95)

### Common categories

Salário, Empresa, Financiamentos, Moradia, Transporte, Empréstimos, Cartão de crédito, Educação, Doações
