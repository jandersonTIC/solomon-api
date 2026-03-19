package main

import (
	"sync"
	"testing"
	"time"
)

func TestStoreAddAndGetTx(t *testing.T) {
	s := NewStore()
	s.SetIDCounters(0, 0)
	tx := &Transaction{
		ID: s.NextTxID(), UserID: 1, Type: TxIncome, Status: StatusConfirmed,
		Date: "2026-03-15", Description: "Salary", Category: "Salário",
		AmountCents: 500000, Currency: "BRL", YearMonth: 202603,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	s.AddTx(tx)
	got := s.GetTx(tx.ID)
	if got == nil || got.ID != tx.ID {
		t.Fatal("expected to find tx")
	}
	list := s.GetTxByMonth(1, 202603, nil)
	if len(list) != 1 {
		t.Fatalf("expected 1 tx, got %d", len(list))
	}
}

func TestStoreFilterByType(t *testing.T) {
	s := NewStore()
	s.SetIDCounters(0, 0)
	s.AddTx(&Transaction{ID: s.NextTxID(), UserID: 1, Type: TxIncome, AmountCents: 100, YearMonth: 202603, Date: "2026-03-01", Category: "A", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	s.AddTx(&Transaction{ID: s.NextTxID(), UserID: 1, Type: TxExpense, AmountCents: 50, YearMonth: 202603, Date: "2026-03-02", Category: "B", CreatedAt: time.Now(), UpdatedAt: time.Now()})

	inc := TxIncome
	list := s.GetTxByMonth(1, 202603, &inc)
	if len(list) != 1 || list[0].Type != TxIncome {
		t.Fatalf("expected 1 income tx, got %d", len(list))
	}
}

func TestStoreSummaryRecompute(t *testing.T) {
	s := NewStore()
	s.SetIDCounters(0, 0)
	s.AddTx(&Transaction{ID: s.NextTxID(), UserID: 1, Type: TxIncome, AmountCents: 10000, YearMonth: 202603, Date: "2026-03-01", Category: "Salário", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	s.AddTx(&Transaction{ID: s.NextTxID(), UserID: 1, Type: TxExpense, AmountCents: 3000, YearMonth: 202603, Date: "2026-03-02", Category: "Moradia", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	s.AddTx(&Transaction{ID: s.NextTxID(), UserID: 1, Type: TxExpense, AmountCents: 2000, YearMonth: 202603, Date: "2026-03-03", Category: "Transporte", CreatedAt: time.Now(), UpdatedAt: time.Now()})

	sum := s.GetSummary(1, 202603)
	if sum == nil {
		t.Fatal("expected summary")
	}
	if sum.IncomeCents != 10000 {
		t.Fatalf("income: want 10000, got %d", sum.IncomeCents)
	}
	if sum.ExpenseCents != 5000 {
		t.Fatalf("expense: want 5000, got %d", sum.ExpenseCents)
	}
	if sum.BalanceCents != 5000 {
		t.Fatalf("balance: want 5000, got %d", sum.BalanceCents)
	}
	if sum.Income["Salário"] != 10000 {
		t.Fatalf("category income: want 10000, got %d", sum.Income["Salário"])
	}
	if sum.Expense["Moradia"] != 3000 {
		t.Fatalf("category expense Moradia: want 3000, got %d", sum.Expense["Moradia"])
	}
}

func TestStoreUpdateTx(t *testing.T) {
	s := NewStore()
	s.SetIDCounters(0, 0)
	id := s.NextTxID()
	s.AddTx(&Transaction{ID: id, UserID: 1, Type: TxExpense, AmountCents: 1000, YearMonth: 202603, Date: "2026-03-01", Category: "A", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	tx, ok := s.UpdateTx(id, func(t *Transaction) bool {
		t.AmountCents = 2000
		return true
	})
	if !ok || tx.AmountCents != 2000 {
		t.Fatal("update failed")
	}
	sum := s.GetSummary(1, 202603)
	if sum.ExpenseCents != 2000 {
		t.Fatalf("expected recomputed expense 2000, got %d", sum.ExpenseCents)
	}
}

func TestStoreDeleteTx(t *testing.T) {
	s := NewStore()
	s.SetIDCounters(0, 0)
	id := s.NextTxID()
	s.AddTx(&Transaction{ID: id, UserID: 1, Type: TxIncome, AmountCents: 5000, YearMonth: 202603, Date: "2026-03-01", Category: "A", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	_, ok := s.DeleteTx(id)
	if !ok {
		t.Fatal("delete failed")
	}
	if s.GetTx(id) != nil {
		t.Fatal("tx should be gone")
	}
	sum := s.GetSummary(1, 202603)
	if sum.IncomeCents != 0 {
		t.Fatalf("expected 0 income after delete, got %d", sum.IncomeCents)
	}
}

func TestStoreDuplicateMonth(t *testing.T) {
	s := NewStore()
	s.SetIDCounters(0, 0)
	cur := int16(2)
	tot := int16(5)
	s.AddTx(&Transaction{ID: s.NextTxID(), UserID: 1, Type: TxIncome, AmountCents: 10000, YearMonth: 202603, Date: "2026-03-15", Category: "Salário", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	s.AddTx(&Transaction{ID: s.NextTxID(), UserID: 1, Type: TxExpense, AmountCents: 3000, YearMonth: 202603, Date: "2026-03-10", Category: "IPTU", InstallmentCur: &cur, InstallmentTot: &tot, CreatedAt: time.Now(), UpdatedAt: time.Now()})

	// Add a completed installment that should be skipped
	done := int16(5)
	s.AddTx(&Transaction{ID: s.NextTxID(), UserID: 1, Type: TxExpense, AmountCents: 1000, YearMonth: 202603, Date: "2026-03-20", Category: "IPVA", InstallmentCur: &done, InstallmentTot: &done, CreatedAt: time.Now(), UpdatedAt: time.Now()})

	created := s.DuplicateMonth(1, 202603, 202604)
	if len(created) != 2 {
		t.Fatalf("expected 2 duplicated txs, got %d", len(created))
	}
	// Check installment advanced
	var iptu *Transaction
	for _, tx := range created {
		if tx.Category == "IPTU" {
			iptu = tx
		}
	}
	if iptu == nil {
		t.Fatal("IPTU not found in duplicated")
	}
	if *iptu.InstallmentCur != 3 {
		t.Fatalf("expected installment_cur=3, got %d", *iptu.InstallmentCur)
	}
	if iptu.Status != StatusPending {
		t.Fatal("duplicated tx should be pending")
	}

	// Check carry-over
	sum := s.GetSummary(1, 202604)
	if sum == nil {
		t.Fatal("expected summary for target month")
	}
	srcSum := s.GetSummary(1, 202603)
	if sum.CarryOver != srcSum.FinalBalance {
		t.Fatalf("carry-over: want %d, got %d", srcSum.FinalBalance, sum.CarryOver)
	}
}

func TestStoreAccountCRUD(t *testing.T) {
	s := NewStore()
	s.SetIDCounters(0, 0)
	a := &Account{ID: s.NextAcctID(), UserID: 1, Name: "Nubank", BalanceCents: 50000, Currency: "BRL", CreatedAt: time.Now()}
	s.AddAccount(a)
	accs := s.GetAccounts(1)
	if len(accs) != 1 {
		t.Fatalf("expected 1 account, got %d", len(accs))
	}
	got := s.GetAccount(a.ID)
	if got == nil || got.Name != "Nubank" {
		t.Fatal("account not found")
	}
	s.UpdateAccount(a.ID, func(a *Account) { a.Name = "Inter" })
	got = s.GetAccount(a.ID)
	if got.Name != "Inter" {
		t.Fatal("update failed")
	}
	_, ok := s.DeleteAccount(a.ID)
	if !ok {
		t.Fatal("delete failed")
	}
	if len(s.GetAccounts(1)) != 0 {
		t.Fatal("account should be deleted")
	}
}

func TestStoreConcurrentAccess(t *testing.T) {
	s := NewStore()
	s.SetIDCounters(0, 0)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx := &Transaction{
				ID: s.NextTxID(), UserID: 1, Type: TxIncome,
				AmountCents: 100, YearMonth: 202603, Date: "2026-03-01",
				Category: "Test", CreatedAt: time.Now(), UpdatedAt: time.Now(),
			}
			s.AddTx(tx)
		}()
	}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.GetTxByMonth(1, 202603, nil)
			s.GetSummary(1, 202603)
		}()
	}
	wg.Wait()
	list := s.GetTxByMonth(1, 202603, nil)
	if len(list) != 100 {
		t.Fatalf("expected 100 txs, got %d", len(list))
	}
}

func TestStoreUpdateTxMoveMonth(t *testing.T) {
	s := NewStore()
	s.SetIDCounters(0, 0)
	id := s.NextTxID()
	s.AddTx(&Transaction{ID: id, UserID: 1, Type: TxExpense, AmountCents: 1000, YearMonth: 202603, Date: "2026-03-01", Category: "A", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	newYM := int32(202604)
	_, ok := s.UpdateTx(id, func(t *Transaction) bool {
		t.YearMonth = newYM
		return true
	})
	if !ok {
		t.Fatal("update failed")
	}
	if len(s.GetTxByMonth(1, 202603, nil)) != 0 {
		t.Fatal("old month should be empty")
	}
	if len(s.GetTxByMonth(1, 202604, nil)) != 1 {
		t.Fatal("new month should have 1 tx")
	}
}

func BenchmarkStoreGetTxByMonth(b *testing.B) {
	s := NewStore()
	s.SetIDCounters(0, 0)
	now := time.Now()
	for i := 0; i < 50; i++ {
		s.AddTx(&Transaction{
			ID: s.NextTxID(), UserID: 1, Type: TxExpense, AmountCents: int64(i * 100),
			YearMonth: 202603, Date: "2026-03-01", Category: "Test",
			CreatedAt: now, UpdatedAt: now,
		})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.GetTxByMonth(1, 202603, nil)
	}
}

func BenchmarkStoreGetSummary(b *testing.B) {
	s := NewStore()
	s.SetIDCounters(0, 0)
	now := time.Now()
	for i := 0; i < 50; i++ {
		typ := TxExpense
		if i%3 == 0 {
			typ = TxIncome
		}
		s.AddTx(&Transaction{
			ID: s.NextTxID(), UserID: 1, Type: typ, AmountCents: int64(i * 100),
			YearMonth: 202603, Date: "2026-03-01", Category: "Test",
			CreatedAt: now, UpdatedAt: now,
		})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.GetSummary(1, 202603)
	}
}

func BenchmarkStoreAddTx(b *testing.B) {
	s := NewStore()
	s.SetIDCounters(0, 0)
	now := time.Now()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.AddTx(&Transaction{
			ID: s.NextTxID(), UserID: 1, Type: TxExpense, AmountCents: 100,
			YearMonth: 202603, Date: "2026-03-01", Category: "Test",
			CreatedAt: now, UpdatedAt: now,
		})
	}
}
