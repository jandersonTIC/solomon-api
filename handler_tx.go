package main

import (
	"net/http"
	"strconv"
	"time"

	json "github.com/goccy/go-json"
)

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

type TxHandler struct {
	store   *Store
	persist *Persister
}

func (h *TxHandler) List(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	ymStr := r.URL.Query().Get("ym")
	ym, err := strconv.Atoi(ymStr)
	if err != nil || ym < 100000 || ym > 999912 {
		writeErr(w, http.StatusBadRequest, "invalid ym")
		return
	}
	var txType *TxType
	if t := r.URL.Query().Get("type"); t != "" {
		v, err := strconv.Atoi(t)
		if err != nil || (v != 1 && v != 2) {
			writeErr(w, http.StatusBadRequest, "invalid type")
			return
		}
		tt := TxType(v)
		txType = &tt
	}
	txs := h.store.GetTxByMonth(uid, int32(ym), txType)
	writeJSON(w, http.StatusOK, txs)
}

func (h *TxHandler) Create(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	var req CreateTxRequest
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Type != TxIncome && req.Type != TxExpense {
		writeErr(w, http.StatusBadRequest, "invalid type")
		return
	}
	if req.AmountCents <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid amount")
		return
	}
	if req.YearMonth < 100000 || req.YearMonth > 999912 {
		writeErr(w, http.StatusBadRequest, "invalid year_month")
		return
	}
	now := time.Now().UTC()
	tx := &Transaction{
		ID:             h.store.NextTxID(),
		UserID:         uid,
		AccountID:      req.AccountID,
		Type:           req.Type,
		Status:         req.Status,
		Date:           req.Date,
		Description:    req.Description,
		Category:       req.Category,
		AmountCents:    req.AmountCents,
		Currency:       req.Currency,
		RecurrenceType: req.RecurrenceType,
		InstallmentCur: req.InstallmentCur,
		InstallmentTot: req.InstallmentTot,
		YearMonth:      req.YearMonth,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if tx.Currency == "" {
		tx.Currency = "BRL"
	}
	h.store.AddTx(tx)
	h.persist.Enqueue(
		`INSERT INTO transactions(id, user_id, account_id, type, status, date, description, category, amount_cents, currency, recurrence_type, installment_cur, installment_tot, year_month, created_at, updated_at)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		tx.ID, tx.UserID, tx.AccountID, tx.Type, tx.Status, tx.Date, tx.Description, tx.Category, tx.AmountCents, tx.Currency, tx.RecurrenceType, tx.InstallmentCur, tx.InstallmentTot, tx.YearMonth, tx.CreatedAt, tx.UpdatedAt,
	)
	writeJSON(w, http.StatusCreated, tx)
}

func (h *TxHandler) Update(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req UpdateTxRequest
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	tx, ok := h.store.UpdateTx(id, func(t *Transaction) bool {
		if t.UserID != uid {
			return false
		}
		if req.Type != nil {
			t.Type = *req.Type
		}
		if req.Status != nil {
			t.Status = *req.Status
		}
		if req.Date != nil {
			t.Date = *req.Date
		}
		if req.Description != nil {
			t.Description = *req.Description
		}
		if req.Category != nil {
			t.Category = *req.Category
		}
		if req.AmountCents != nil {
			t.AmountCents = *req.AmountCents
		}
		if req.Currency != nil {
			t.Currency = *req.Currency
		}
		if req.RecurrenceType != nil {
			t.RecurrenceType = *req.RecurrenceType
		}
		if req.InstallmentCur != nil {
			t.InstallmentCur = req.InstallmentCur
		}
		if req.InstallmentTot != nil {
			t.InstallmentTot = req.InstallmentTot
		}
		if req.AccountID != nil {
			t.AccountID = req.AccountID
		}
		if req.YearMonth != nil {
			t.YearMonth = *req.YearMonth
		}
		return true
	})
	if !ok {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	h.persist.Enqueue(
		`UPDATE transactions SET type=$1, status=$2, date=$3, description=$4, category=$5, amount_cents=$6, currency=$7, recurrence_type=$8, installment_cur=$9, installment_tot=$10, account_id=$11, year_month=$12, updated_at=$13 WHERE id=$14`,
		tx.Type, tx.Status, tx.Date, tx.Description, tx.Category, tx.AmountCents, tx.Currency, tx.RecurrenceType, tx.InstallmentCur, tx.InstallmentTot, tx.AccountID, tx.YearMonth, tx.UpdatedAt, tx.ID,
	)
	writeJSON(w, http.StatusOK, tx)
}

func (h *TxHandler) Delete(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	tx, ok := h.store.DeleteTx(id)
	if !ok || tx.UserID != uid {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	h.persist.Enqueue(`DELETE FROM transactions WHERE id=$1`, id)
	w.WriteHeader(http.StatusNoContent)
}

func (h *TxHandler) PatchStatus(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req StatusPatchRequest
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	tx, ok := h.store.UpdateTx(id, func(t *Transaction) bool {
		if t.UserID != uid {
			return false
		}
		t.Status = req.Status
		return true
	})
	if !ok {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	h.persist.Enqueue(`UPDATE transactions SET status=$1, updated_at=$2 WHERE id=$3`, tx.Status, tx.UpdatedAt, tx.ID)
	writeJSON(w, http.StatusOK, tx)
}
