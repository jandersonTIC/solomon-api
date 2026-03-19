package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	json "github.com/goccy/go-json"
	"github.com/golang-jwt/jwt/v5"
)

func TestCreateAndListTx(t *testing.T) {
	_, handler, secret := newTestServer(t)
	tok := testToken(t, 1, secret)

	body := `{"type":1,"status":0,"date":"2026-03-15","description":"Salary","category":"Salário","amount_cents":500000,"currency":"BRL","year_month":202603}`
	req := httptest.NewRequest("POST", "/v1/transactions", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest("GET", "/v1/transactions?ym=202603", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list: want 200, got %d", w.Code)
	}
	var txs []*Transaction
	json.Unmarshal(w.Body.Bytes(), &txs)
	if len(txs) != 1 {
		t.Fatalf("expected 1 tx, got %d", len(txs))
	}
	if txs[0].Description != "Salary" {
		t.Fatalf("description: want Salary, got %s", txs[0].Description)
	}
}

func TestListTxFilterByType(t *testing.T) {
	_, handler, secret := newTestServer(t)
	tok := testToken(t, 1, secret)

	for _, b := range []string{
		`{"type":1,"status":0,"date":"2026-03-01","description":"Income","category":"Salário","amount_cents":100,"currency":"BRL","year_month":202603}`,
		`{"type":2,"status":0,"date":"2026-03-02","description":"Expense","category":"Moradia","amount_cents":50,"currency":"BRL","year_month":202603}`,
	} {
		req := httptest.NewRequest("POST", "/v1/transactions", bytes.NewBufferString(b))
		req.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}

	req := httptest.NewRequest("GET", "/v1/transactions?ym=202603&type=1", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	var txs []*Transaction
	json.Unmarshal(w.Body.Bytes(), &txs)
	if len(txs) != 1 || txs[0].Type != TxIncome {
		t.Fatalf("expected 1 income, got %d", len(txs))
	}
}

func TestUnauthorized(t *testing.T) {
	_, handler, _ := newTestServer(t)
	req := httptest.NewRequest("GET", "/v1/transactions?ym=202603", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestUpdateTx(t *testing.T) {
	_, handler, secret := newTestServer(t)
	tok := testToken(t, 1, secret)

	body := `{"type":2,"status":0,"date":"2026-03-10","description":"IPTU","category":"Moradia","amount_cents":30000,"currency":"BRL","year_month":202603}`
	req := httptest.NewRequest("POST", "/v1/transactions", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	var tx Transaction
	json.Unmarshal(w.Body.Bytes(), &tx)

	update := `{"amount_cents":35000}`
	req = httptest.NewRequest("PUT", "/v1/transactions/"+fmtID(tx.ID), bytes.NewBufferString(update))
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update: want 200, got %d: %s", w.Code, w.Body.String())
	}
	var updated Transaction
	json.Unmarshal(w.Body.Bytes(), &updated)
	if updated.AmountCents != 35000 {
		t.Fatalf("amount: want 35000, got %d", updated.AmountCents)
	}
}

func TestDeleteTx(t *testing.T) {
	_, handler, secret := newTestServer(t)
	tok := testToken(t, 1, secret)

	body := `{"type":2,"status":0,"date":"2026-03-10","description":"IPTU","category":"Moradia","amount_cents":30000,"currency":"BRL","year_month":202603}`
	req := httptest.NewRequest("POST", "/v1/transactions", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	var tx Transaction
	json.Unmarshal(w.Body.Bytes(), &tx)

	req = httptest.NewRequest("DELETE", "/v1/transactions/"+fmtID(tx.ID), nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: want 204, got %d", w.Code)
	}
}

func TestPatchStatus(t *testing.T) {
	_, handler, secret := newTestServer(t)
	tok := testToken(t, 1, secret)

	body := `{"type":1,"status":0,"date":"2026-03-01","description":"Test","category":"A","amount_cents":100,"currency":"BRL","year_month":202603}`
	req := httptest.NewRequest("POST", "/v1/transactions", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	var tx Transaction
	json.Unmarshal(w.Body.Bytes(), &tx)

	req = httptest.NewRequest("PATCH", "/v1/transactions/"+fmtID(tx.ID)+"/status", bytes.NewBufferString(`{"status":1}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("patch: want 200, got %d: %s", w.Code, w.Body.String())
	}
	var updated Transaction
	json.Unmarshal(w.Body.Bytes(), &updated)
	if updated.Status != StatusConfirmed {
		t.Fatalf("status: want confirmed, got %d", updated.Status)
	}
}

func TestCreateTxValidation(t *testing.T) {
	_, handler, secret := newTestServer(t)
	tok := testToken(t, 1, secret)

	cases := []struct {
		name string
		body string
	}{
		{"invalid type", `{"type":3,"date":"2026-03-01","description":"X","category":"A","amount_cents":100,"currency":"BRL","year_month":202603}`},
		{"zero amount", `{"type":1,"date":"2026-03-01","description":"X","category":"A","amount_cents":0,"currency":"BRL","year_month":202603}`},
		{"invalid ym", `{"type":1,"date":"2026-03-01","description":"X","category":"A","amount_cents":100,"currency":"BRL","year_month":999}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/v1/transactions", bytes.NewBufferString(tc.body))
			req.Header.Set("Authorization", "Bearer "+tok)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("want 400, got %d", w.Code)
			}
		})
	}
}

func BenchmarkListTxHandler(b *testing.B) {
	store := NewStore()
	store.SetIDCounters(0, 0)
	secret := []byte("bench-secret")
	persist := &Persister{ch: make(chan persistOp, 4096)}
	handler := buildRouter(store, persist, nil, secret)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"uid": float64(1),
		"exp": float64(time.Now().Add(time.Hour).Unix()),
	})
	tok, _ := token.SignedString(secret)

	now := time.Now()
	for i := 0; i < 50; i++ {
		store.AddTx(&Transaction{
			ID: store.NextTxID(), UserID: 1, Type: TxExpense, AmountCents: 100,
			YearMonth: 202603, Date: "2026-03-01", Category: "Test",
			CreatedAt: now, UpdatedAt: now,
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/v1/transactions?ym=202603", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}
}
