package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	json "github.com/goccy/go-json"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupIntegrationDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	pg, err := postgres.Run(ctx, "postgres:17-alpine",
		postgres.WithDatabase("solomon_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { pg.Terminate(ctx) })

	connStr, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("pgxpool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	if err := runMigrations(ctx, pool); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	return pool
}

func createTestUser(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users(apple_sub, email, name) VALUES('test-sub', 'test@test.com', 'Test') RETURNING id`,
	).Scan(&id)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return id
}

func integrationToken(t *testing.T, uid int64, secret []byte) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"uid": float64(uid),
		"exp": float64(time.Now().Add(time.Hour).Unix()),
	})
	s, err := tok.SignedString(secret)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestIntegrationPersistAndReload(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := setupIntegrationDB(t)
	uid := createTestUser(t, pool)
	secret := []byte("integration-secret")

	// Phase 1: Create transactions via API, let persister flush to DB
	store1 := NewStore()
	store1.SetIDCounters(0, 0)
	persister1 := NewPersister(pool)
	handler1 := buildRouter(store1, persister1, pool, secret)
	tok := integrationToken(t, uid, secret)

	txBodies := []string{
		`{"type":1,"status":1,"date":"2026-03-01","description":"Salary","category":"Salário","amount_cents":500000,"currency":"BRL","year_month":202603}`,
		`{"type":2,"status":0,"date":"2026-03-05","description":"Rent","category":"Moradia","amount_cents":200000,"currency":"BRL","year_month":202603}`,
		`{"type":2,"status":1,"date":"2026-03-10","description":"IPTU 3/12","category":"Moradia","amount_cents":45000,"currency":"BRL","year_month":202603}`,
	}
	for _, body := range txBodies {
		req := httptest.NewRequest("POST", "/v1/transactions", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		handler1.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create: want 201, got %d: %s", w.Code, w.Body.String())
		}
	}

	// Stop persister to flush all writes to DB
	persister1.Stop()

	// Verify data is in PostgreSQL
	var count int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM transactions WHERE user_id=$1`, uid).Scan(&count)
	if count != 3 {
		t.Fatalf("expected 3 rows in DB, got %d", count)
	}

	// Phase 2: Create a fresh store and reload from DB (simulates restart)
	store2 := NewStore()
	ctx := context.Background()
	if err := loadStore(ctx, pool, store2); err != nil {
		t.Fatalf("reload store: %v", err)
	}

	// Verify reloaded data matches
	txs := store2.GetTxByMonth(uid, 202603, nil)
	if len(txs) != 3 {
		t.Fatalf("reloaded store: expected 3 txs, got %d", len(txs))
	}

	sum := store2.GetSummary(uid, 202603)
	if sum == nil {
		t.Fatal("expected summary after reload")
	}
	if sum.IncomeCents != 500000 {
		t.Fatalf("income: want 500000, got %d", sum.IncomeCents)
	}
	if sum.ExpenseCents != 245000 {
		t.Fatalf("expense: want 245000, got %d", sum.ExpenseCents)
	}
	if sum.BalanceCents != 255000 {
		t.Fatalf("balance: want 255000, got %d", sum.BalanceCents)
	}

	// Verify categories recomputed correctly
	if sum.Expense["Moradia"] != 245000 {
		t.Fatalf("Moradia: want 245000, got %d", sum.Expense["Moradia"])
	}
}

func TestIntegrationUpdateAndDelete(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := setupIntegrationDB(t)
	uid := createTestUser(t, pool)
	secret := []byte("integration-secret")

	store := NewStore()
	store.SetIDCounters(0, 0)
	persister := NewPersister(pool)
	handler := buildRouter(store, persister, pool, secret)
	tok := integrationToken(t, uid, secret)

	// Create
	req := httptest.NewRequest("POST", "/v1/transactions", bytes.NewBufferString(
		`{"type":2,"status":0,"date":"2026-03-10","description":"IPTU","category":"Moradia","amount_cents":30000,"currency":"BRL","year_month":202603}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	var tx Transaction
	json.Unmarshal(w.Body.Bytes(), &tx)

	// Update
	req = httptest.NewRequest("PUT", "/v1/transactions/"+fmtID(tx.ID), bytes.NewBufferString(`{"amount_cents":35000}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update: want 200, got %d", w.Code)
	}

	// Flush and reload
	persister.Stop()
	store2 := NewStore()
	loadStore(context.Background(), pool, store2)
	reloaded := store2.GetTx(tx.ID)
	if reloaded == nil || reloaded.AmountCents != 35000 {
		t.Fatalf("reloaded amount: want 35000, got %v", reloaded)
	}

	// Delete via a new persister
	persister2 := NewPersister(pool)
	handler2 := buildRouter(store2, persister2, pool, secret)
	req = httptest.NewRequest("DELETE", "/v1/transactions/"+fmtID(tx.ID), nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	handler2.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: want 204, got %d", w.Code)
	}
	persister2.Stop()

	// Reload again, verify deleted
	store3 := NewStore()
	loadStore(context.Background(), pool, store3)
	if store3.GetTx(tx.ID) != nil {
		t.Fatal("tx should be deleted after reload")
	}
}

func TestIntegrationDuplicateMonthPersistence(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := setupIntegrationDB(t)
	uid := createTestUser(t, pool)
	secret := []byte("integration-secret")

	store := NewStore()
	store.SetIDCounters(0, 0)
	persister := NewPersister(pool)
	handler := buildRouter(store, persister, pool, secret)
	tok := integrationToken(t, uid, secret)

	// Create March transactions
	for _, body := range []string{
		`{"type":1,"status":1,"date":"2026-03-01","description":"Salary","category":"Salário","amount_cents":500000,"currency":"BRL","year_month":202603}`,
		`{"type":2,"status":1,"date":"2026-03-05","description":"Rent","category":"Moradia","amount_cents":200000,"currency":"BRL","year_month":202603}`,
	} {
		req := httptest.NewRequest("POST", "/v1/transactions", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}

	// Duplicate to April
	req := httptest.NewRequest("POST", "/v1/months/202603/duplicate", bytes.NewBufferString(`{"target_year_month":202604}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("duplicate: want 201, got %d: %s", w.Code, w.Body.String())
	}

	// Flush and reload
	persister.Stop()
	store2 := NewStore()
	loadStore(context.Background(), pool, store2)

	// Verify April transactions exist after reload
	aprilTxs := store2.GetTxByMonth(uid, 202604, nil)
	if len(aprilTxs) != 2 {
		t.Fatalf("april txs: want 2, got %d", len(aprilTxs))
	}
	for _, tx := range aprilTxs {
		if tx.Status != StatusPending {
			t.Fatal("duplicated tx should be pending")
		}
	}

	// Verify carry-over persisted
	aprilSum := store2.GetSummary(uid, 202604)
	if aprilSum == nil {
		t.Fatal("expected april summary")
	}
	if aprilSum.CarryOver != 300000 {
		t.Fatalf("carry-over: want 300000, got %d", aprilSum.CarryOver)
	}
}

func TestIntegrationAccountPersistence(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := setupIntegrationDB(t)
	uid := createTestUser(t, pool)
	secret := []byte("integration-secret")

	store := NewStore()
	store.SetIDCounters(0, 0)
	persister := NewPersister(pool)
	handler := buildRouter(store, persister, pool, secret)
	tok := integrationToken(t, uid, secret)

	// Create account
	req := httptest.NewRequest("POST", "/v1/accounts", bytes.NewBufferString(`{"name":"Nubank","balance_cents":150000,"currency":"BRL"}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d", w.Code)
	}

	// Flush and reload
	persister.Stop()
	store2 := NewStore()
	loadStore(context.Background(), pool, store2)

	accs := store2.GetAccounts(uid)
	if len(accs) != 1 {
		t.Fatalf("accounts: want 1, got %d", len(accs))
	}
	if accs[0].Name != "Nubank" || accs[0].BalanceCents != 150000 {
		t.Fatalf("account mismatch: %+v", accs[0])
	}
}
