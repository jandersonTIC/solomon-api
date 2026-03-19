package main

import (
	"net/http"
	"strconv"
	"time"
)

type AccountHandler struct {
	store   *Store
	persist *Persister
}

func (h *AccountHandler) List(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	accs := h.store.GetAccounts(uid)
	writeJSON(w, http.StatusOK, accs)
}

func (h *AccountHandler) Create(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	var req CreateAccountRequest
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	if req.Currency == "" {
		req.Currency = "BRL"
	}
	a := &Account{
		ID:           h.store.NextAcctID(),
		UserID:       uid,
		Name:         req.Name,
		BalanceCents: req.BalanceCents,
		Currency:     req.Currency,
		CreatedAt:    time.Now().UTC(),
	}
	h.store.AddAccount(a)
	h.persist.Enqueue(
		`INSERT INTO accounts(id, user_id, name, balance_cents, currency, created_at) VALUES($1,$2,$3,$4,$5,$6)`,
		a.ID, a.UserID, a.Name, a.BalanceCents, a.Currency, a.CreatedAt,
	)
	writeJSON(w, http.StatusCreated, a)
}

func (h *AccountHandler) Update(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req UpdateAccountRequest
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	a, ok := h.store.UpdateAccount(id, func(a *Account) {
		if req.Name != nil {
			a.Name = *req.Name
		}
		if req.BalanceCents != nil {
			a.BalanceCents = *req.BalanceCents
		}
		if req.Currency != nil {
			a.Currency = *req.Currency
		}
	})
	if !ok || a.UserID != uid {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	h.persist.Enqueue(
		`UPDATE accounts SET name=$1, balance_cents=$2, currency=$3 WHERE id=$4`,
		a.Name, a.BalanceCents, a.Currency, a.ID,
	)
	writeJSON(w, http.StatusOK, a)
}

func (h *AccountHandler) Delete(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	a, ok := h.store.DeleteAccount(id)
	if !ok || a.UserID != uid {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	h.persist.Enqueue(`DELETE FROM accounts WHERE id=$1`, id)
	w.WriteHeader(http.StatusNoContent)
}
