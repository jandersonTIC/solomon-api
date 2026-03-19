package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	json "github.com/goccy/go-json"
)

func TestMonthSummary(t *testing.T) {
	_, handler, secret := newTestServer(t)
	tok := testToken(t, 1, secret)

	// Create transactions
	for _, b := range []string{
		`{"type":1,"status":1,"date":"2026-03-01","description":"Salary","category":"Salário","amount_cents":500000,"currency":"BRL","year_month":202603}`,
		`{"type":2,"status":1,"date":"2026-03-05","description":"Rent","category":"Moradia","amount_cents":200000,"currency":"BRL","year_month":202603}`,
		`{"type":2,"status":0,"date":"2026-03-10","description":"IPTU","category":"Moradia","amount_cents":50000,"currency":"BRL","year_month":202603}`,
	} {
		req := httptest.NewRequest("POST", "/v1/transactions", bytes.NewBufferString(b))
		req.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}

	req := httptest.NewRequest("GET", "/v1/months/202603/summary", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var sum MonthSummary
	json.Unmarshal(w.Body.Bytes(), &sum)
	if sum.IncomeCents != 500000 {
		t.Fatalf("income: want 500000, got %d", sum.IncomeCents)
	}
	if sum.ExpenseCents != 250000 {
		t.Fatalf("expense: want 250000, got %d", sum.ExpenseCents)
	}
	if sum.BalanceCents != 250000 {
		t.Fatalf("balance: want 250000, got %d", sum.BalanceCents)
	}
}

func TestMonthCategories(t *testing.T) {
	_, handler, secret := newTestServer(t)
	tok := testToken(t, 1, secret)

	for _, b := range []string{
		`{"type":2,"status":1,"date":"2026-03-05","description":"Rent","category":"Moradia","amount_cents":200000,"currency":"BRL","year_month":202603}`,
		`{"type":2,"status":1,"date":"2026-03-10","description":"Gas","category":"Transporte","amount_cents":30000,"currency":"BRL","year_month":202603}`,
	} {
		req := httptest.NewRequest("POST", "/v1/transactions", bytes.NewBufferString(b))
		req.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}

	req := httptest.NewRequest("GET", "/v1/months/202603/categories", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var cat map[string]map[string]int64
	json.Unmarshal(w.Body.Bytes(), &cat)
	if cat["expense"]["Moradia"] != 200000 {
		t.Fatalf("Moradia: want 200000, got %d", cat["expense"]["Moradia"])
	}
	if cat["expense"]["Transporte"] != 30000 {
		t.Fatalf("Transporte: want 30000, got %d", cat["expense"]["Transporte"])
	}
}

func TestDuplicateMonth(t *testing.T) {
	_, handler, secret := newTestServer(t)
	tok := testToken(t, 1, secret)

	for _, b := range []string{
		`{"type":1,"status":1,"date":"2026-03-01","description":"Salary","category":"Salário","amount_cents":500000,"currency":"BRL","year_month":202603}`,
		`{"type":2,"status":1,"date":"2026-03-05","description":"Rent","category":"Moradia","amount_cents":200000,"currency":"BRL","year_month":202603}`,
	} {
		req := httptest.NewRequest("POST", "/v1/transactions", bytes.NewBufferString(b))
		req.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}

	dup := `{"target_year_month":202604}`
	req := httptest.NewRequest("POST", "/v1/months/202603/duplicate", bytes.NewBufferString(dup))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("duplicate: want 201, got %d: %s", w.Code, w.Body.String())
	}
	var created []*Transaction
	json.Unmarshal(w.Body.Bytes(), &created)
	if len(created) != 2 {
		t.Fatalf("expected 2 duplicated txs, got %d", len(created))
	}
	for _, tx := range created {
		if tx.Status != StatusPending {
			t.Fatal("duplicated tx should be pending")
		}
		if tx.YearMonth != 202604 {
			t.Fatalf("year_month: want 202604, got %d", tx.YearMonth)
		}
	}

	// Check April summary has carry-over from March
	req = httptest.NewRequest("GET", "/v1/months/202604/summary", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	var sum MonthSummary
	json.Unmarshal(w.Body.Bytes(), &sum)
	if sum.CarryOver != 300000 {
		t.Fatalf("carry-over: want 300000 (March balance), got %d", sum.CarryOver)
	}
}

func TestEmptyMonthSummary(t *testing.T) {
	_, handler, secret := newTestServer(t)
	tok := testToken(t, 1, secret)

	req := httptest.NewRequest("GET", "/v1/months/202601/summary", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
}
