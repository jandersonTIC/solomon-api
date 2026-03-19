package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	json "github.com/goccy/go-json"
)

func TestAccountCRUD(t *testing.T) {
	_, handler, secret := newTestServer(t)
	tok := testToken(t, 1, secret)

	// Create
	body := `{"name":"Nubank","balance_cents":50000,"currency":"BRL"}`
	req := httptest.NewRequest("POST", "/v1/accounts", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d: %s", w.Code, w.Body.String())
	}
	var acct Account
	json.Unmarshal(w.Body.Bytes(), &acct)
	if acct.Name != "Nubank" {
		t.Fatalf("name: want Nubank, got %s", acct.Name)
	}

	// List
	req = httptest.NewRequest("GET", "/v1/accounts", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list: want 200, got %d", w.Code)
	}
	var accts []*Account
	json.Unmarshal(w.Body.Bytes(), &accts)
	if len(accts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(accts))
	}

	// Update
	update := `{"name":"Inter"}`
	req = httptest.NewRequest("PUT", "/v1/accounts/"+fmtID(acct.ID), bytes.NewBufferString(update))
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update: want 200, got %d", w.Code)
	}
	var updated Account
	json.Unmarshal(w.Body.Bytes(), &updated)
	if updated.Name != "Inter" {
		t.Fatalf("name: want Inter, got %s", updated.Name)
	}

	// Delete
	req = httptest.NewRequest("DELETE", "/v1/accounts/"+fmtID(acct.ID), nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: want 204, got %d", w.Code)
	}

	// Verify deleted
	req = httptest.NewRequest("GET", "/v1/accounts", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	json.Unmarshal(w.Body.Bytes(), &accts)
	if len(accts) != 0 {
		t.Fatalf("expected 0 accounts after delete, got %d", len(accts))
	}
}

func TestAccountDefaultCurrency(t *testing.T) {
	_, handler, secret := newTestServer(t)
	tok := testToken(t, 1, secret)

	body := `{"name":"Test"}`
	req := httptest.NewRequest("POST", "/v1/accounts", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d", w.Code)
	}
	var acct Account
	json.Unmarshal(w.Body.Bytes(), &acct)
	if acct.Currency != "BRL" {
		t.Fatalf("currency: want BRL, got %s", acct.Currency)
	}
}

func TestAccountNameRequired(t *testing.T) {
	_, handler, secret := newTestServer(t)
	tok := testToken(t, 1, secret)

	req := httptest.NewRequest("POST", "/v1/accounts", bytes.NewBufferString(`{}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}
