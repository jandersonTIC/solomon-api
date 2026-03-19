package main

import (
	"net/http"
	"strconv"
)

type MonthHandler struct {
	store   *Store
	persist *Persister
}

func (h *MonthHandler) Summary(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	ym, err := strconv.Atoi(r.PathValue("ym"))
	if err != nil || ym < 100000 || ym > 999912 {
		writeErr(w, http.StatusBadRequest, "invalid ym")
		return
	}
	s := h.store.GetSummary(uid, int32(ym))
	if s == nil {
		s = &MonthSummary{YearMonth: int32(ym), Income: map[string]int64{}, Expense: map[string]int64{}}
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *MonthHandler) Categories(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	ym, err := strconv.Atoi(r.PathValue("ym"))
	if err != nil || ym < 100000 || ym > 999912 {
		writeErr(w, http.StatusBadRequest, "invalid ym")
		return
	}
	s := h.store.GetCategories(uid, int32(ym))
	if s == nil {
		writeJSON(w, http.StatusOK, map[string]any{"income": map[string]int64{}, "expense": map[string]int64{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"income": s.Income, "expense": s.Expense})
}

func (h *MonthHandler) Duplicate(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	ym, err := strconv.Atoi(r.PathValue("ym"))
	if err != nil || ym < 100000 || ym > 999912 {
		writeErr(w, http.StatusBadRequest, "invalid ym")
		return
	}
	var req DuplicateMonthRequest
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.TargetYearMonth < 100000 || req.TargetYearMonth > 999912 {
		writeErr(w, http.StatusBadRequest, "invalid target_year_month")
		return
	}
	created := h.store.DuplicateMonth(uid, int32(ym), req.TargetYearMonth)
	for _, tx := range created {
		h.persist.Enqueue(
			`INSERT INTO transactions(id, user_id, account_id, type, status, date, description, category, amount_cents, currency, recurrence_type, installment_cur, installment_tot, year_month, created_at, updated_at)
			 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
			tx.ID, tx.UserID, tx.AccountID, tx.Type, tx.Status, tx.Date, tx.Description, tx.Category, tx.AmountCents, tx.Currency, tx.RecurrenceType, tx.InstallmentCur, tx.InstallmentTot, tx.YearMonth, tx.CreatedAt, tx.UpdatedAt,
		)
	}
	// persist carry-over
	summary := h.store.GetSummary(uid, req.TargetYearMonth)
	if summary != nil {
		h.persist.Enqueue(
			`INSERT INTO month_balances(user_id, year_month, carry_over_cents) VALUES($1,$2,$3) ON CONFLICT(user_id, year_month) DO UPDATE SET carry_over_cents=EXCLUDED.carry_over_cents`,
			uid, req.TargetYearMonth, summary.CarryOver,
		)
	}
	writeJSON(w, http.StatusCreated, created)
}
