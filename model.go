package main

import "time"

type TxType int8

const (
	TxIncome  TxType = 1
	TxExpense TxType = 2
)

type TxStatus int8

const (
	StatusPending   TxStatus = 0
	StatusConfirmed TxStatus = 1
)

type RecurrenceType int8

const (
	RecurNone     RecurrenceType = 0
	RecurFixed    RecurrenceType = 1
	RecurVariable RecurrenceType = 2
)

type Transaction struct {
	ID             int64          `json:"id"`
	UserID         int64          `json:"user_id"`
	AccountID      *int64         `json:"account_id,omitempty"`
	Type           TxType         `json:"type"`
	Status         TxStatus       `json:"status"`
	Date           string         `json:"date"`
	Description    string         `json:"description"`
	Category       string         `json:"category"`
	AmountCents    int64          `json:"amount_cents"`
	Currency       string         `json:"currency"`
	RecurrenceType RecurrenceType `json:"recurrence_type"`
	InstallmentCur *int16         `json:"installment_cur,omitempty"`
	InstallmentTot *int16         `json:"installment_tot,omitempty"`
	YearMonth      int32          `json:"year_month"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type User struct {
	ID        int64     `json:"id"`
	AppleSub  string    `json:"apple_sub"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type Account struct {
	ID           int64     `json:"id"`
	UserID       int64     `json:"user_id"`
	Name         string    `json:"name"`
	BalanceCents int64     `json:"balance_cents"`
	Currency     string    `json:"currency"`
	CreatedAt    time.Time `json:"created_at"`
}

type MonthSummary struct {
	YearMonth    int32            `json:"year_month"`
	IncomeCents  int64            `json:"income_cents"`
	ExpenseCents int64            `json:"expense_cents"`
	BalanceCents int64            `json:"balance_cents"`
	CarryOver    int64            `json:"carry_over"`
	FinalBalance int64            `json:"final_balance"`
	Income       map[string]int64 `json:"income_categories"`
	Expense      map[string]int64 `json:"expense_categories"`
}

type CreateTxRequest struct {
	AccountID      *int64         `json:"account_id,omitempty"`
	Type           TxType         `json:"type"`
	Status         TxStatus       `json:"status"`
	Date           string         `json:"date"`
	Description    string         `json:"description"`
	Category       string         `json:"category"`
	AmountCents    int64          `json:"amount_cents"`
	Currency       string         `json:"currency"`
	RecurrenceType RecurrenceType `json:"recurrence_type"`
	InstallmentCur *int16         `json:"installment_cur,omitempty"`
	InstallmentTot *int16         `json:"installment_tot,omitempty"`
	YearMonth      int32          `json:"year_month"`
}

type UpdateTxRequest struct {
	AccountID      *int64         `json:"account_id,omitempty"`
	Type           *TxType        `json:"type,omitempty"`
	Status         *TxStatus      `json:"status,omitempty"`
	Date           *string        `json:"date,omitempty"`
	Description    *string        `json:"description,omitempty"`
	Category       *string        `json:"category,omitempty"`
	AmountCents    *int64         `json:"amount_cents,omitempty"`
	Currency       *string        `json:"currency,omitempty"`
	RecurrenceType *RecurrenceType `json:"recurrence_type,omitempty"`
	InstallmentCur *int16         `json:"installment_cur,omitempty"`
	InstallmentTot *int16         `json:"installment_tot,omitempty"`
	YearMonth      *int32         `json:"year_month,omitempty"`
}

type StatusPatchRequest struct {
	Status TxStatus `json:"status"`
}

type CreateAccountRequest struct {
	Name         string `json:"name"`
	BalanceCents int64  `json:"balance_cents"`
	Currency     string `json:"currency"`
}

type UpdateAccountRequest struct {
	Name         *string `json:"name,omitempty"`
	BalanceCents *int64  `json:"balance_cents,omitempty"`
	Currency     *string `json:"currency,omitempty"`
}

type DuplicateMonthRequest struct {
	TargetYearMonth int32 `json:"target_year_month"`
}
