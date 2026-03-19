package main

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type Store struct {
	mu         sync.RWMutex
	txByMonth  map[int64]map[int32][]*Transaction
	txByID     map[int64]*Transaction
	summaries  map[int64]map[int32]*MonthSummary
	accounts   map[int64][]*Account
	accountIdx map[int64]*Account
	carryOver  map[int64]map[int32]int64
	nextTxID   atomic.Int64
	nextAcctID atomic.Int64
}

func NewStore() *Store {
	return &Store{
		txByMonth:  make(map[int64]map[int32][]*Transaction),
		txByID:     make(map[int64]*Transaction),
		summaries:  make(map[int64]map[int32]*MonthSummary),
		accounts:   make(map[int64][]*Account),
		accountIdx: make(map[int64]*Account),
		carryOver:  make(map[int64]map[int32]int64),
	}
}

func (s *Store) recompute(uid int64, ym int32) {
	m := s.summaries[uid]
	if m == nil {
		m = make(map[int32]*MonthSummary)
		s.summaries[uid] = m
	}
	txs := s.txByMonth[uid][ym]
	inc := make(map[string]int64)
	exp := make(map[string]int64)
	var incTotal, expTotal int64
	for _, t := range txs {
		if t.Type == TxIncome {
			incTotal += t.AmountCents
			inc[t.Category] += t.AmountCents
		} else {
			expTotal += t.AmountCents
			exp[t.Category] += t.AmountCents
		}
	}
	bal := incTotal - expTotal
	co := s.getCarryOver(uid, ym)
	m[ym] = &MonthSummary{
		YearMonth:    ym,
		IncomeCents:  incTotal,
		ExpenseCents: expTotal,
		BalanceCents: bal,
		CarryOver:    co,
		FinalBalance: co + bal,
		Income:       inc,
		Expense:      exp,
	}
}

func (s *Store) getCarryOver(uid int64, ym int32) int64 {
	if m := s.carryOver[uid]; m != nil {
		return m[ym]
	}
	return 0
}

func (s *Store) setCarryOver(uid int64, ym int32, cents int64) {
	m := s.carryOver[uid]
	if m == nil {
		m = make(map[int32]int64)
		s.carryOver[uid] = m
	}
	m[ym] = cents
}

func (s *Store) AddTx(tx *Transaction) {
	s.mu.Lock()
	uid := tx.UserID
	ym := tx.YearMonth
	if s.txByMonth[uid] == nil {
		s.txByMonth[uid] = make(map[int32][]*Transaction)
	}
	s.txByMonth[uid][ym] = append(s.txByMonth[uid][ym], tx)
	sort.Slice(s.txByMonth[uid][ym], func(i, j int) bool {
		return s.txByMonth[uid][ym][i].Date < s.txByMonth[uid][ym][j].Date
	})
	s.txByID[tx.ID] = tx
	s.recompute(uid, ym)
	s.mu.Unlock()
}

func (s *Store) GetTxByMonth(uid int64, ym int32, txType *TxType) []*Transaction {
	s.mu.RLock()
	txs := s.txByMonth[uid][ym]
	if txType == nil {
		out := make([]*Transaction, len(txs))
		copy(out, txs)
		s.mu.RUnlock()
		return out
	}
	out := make([]*Transaction, 0, len(txs))
	for _, t := range txs {
		if t.Type == *txType {
			out = append(out, t)
		}
	}
	s.mu.RUnlock()
	return out
}

func (s *Store) GetTx(id int64) *Transaction {
	s.mu.RLock()
	tx := s.txByID[id]
	s.mu.RUnlock()
	return tx
}

func (s *Store) UpdateTx(id int64, fn func(*Transaction) bool) (*Transaction, bool) {
	s.mu.Lock()
	tx := s.txByID[id]
	if tx == nil {
		s.mu.Unlock()
		return nil, false
	}
	oldYM := tx.YearMonth
	if !fn(tx) {
		s.mu.Unlock()
		return nil, false
	}
	tx.UpdatedAt = time.Now().UTC()
	if tx.YearMonth != oldYM {
		list := s.txByMonth[tx.UserID][oldYM]
		for i, t := range list {
			if t.ID == id {
				s.txByMonth[tx.UserID][oldYM] = append(list[:i], list[i+1:]...)
				break
			}
		}
		s.recompute(tx.UserID, oldYM)
		if s.txByMonth[tx.UserID] == nil {
			s.txByMonth[tx.UserID] = make(map[int32][]*Transaction)
		}
		s.txByMonth[tx.UserID][tx.YearMonth] = append(s.txByMonth[tx.UserID][tx.YearMonth], tx)
		sort.Slice(s.txByMonth[tx.UserID][tx.YearMonth], func(i, j int) bool {
			return s.txByMonth[tx.UserID][tx.YearMonth][i].Date < s.txByMonth[tx.UserID][tx.YearMonth][j].Date
		})
	}
	s.recompute(tx.UserID, tx.YearMonth)
	s.mu.Unlock()
	return tx, true
}

func (s *Store) DeleteTx(id int64) (*Transaction, bool) {
	s.mu.Lock()
	tx := s.txByID[id]
	if tx == nil {
		s.mu.Unlock()
		return nil, false
	}
	delete(s.txByID, id)
	list := s.txByMonth[tx.UserID][tx.YearMonth]
	for i, t := range list {
		if t.ID == id {
			s.txByMonth[tx.UserID][tx.YearMonth] = append(list[:i], list[i+1:]...)
			break
		}
	}
	s.recompute(tx.UserID, tx.YearMonth)
	s.mu.Unlock()
	return tx, true
}

func (s *Store) GetSummary(uid int64, ym int32) *MonthSummary {
	s.mu.RLock()
	var ms *MonthSummary
	if m := s.summaries[uid]; m != nil {
		ms = m[ym]
	}
	s.mu.RUnlock()
	return ms
}

func (s *Store) GetCategories(uid int64, ym int32) *MonthSummary {
	return s.GetSummary(uid, ym)
}

func (s *Store) NextTxID() int64 {
	return s.nextTxID.Add(1)
}

func (s *Store) NextAcctID() int64 {
	return s.nextAcctID.Add(1)
}

func (s *Store) SetIDCounters(maxTxID, maxAcctID int64) {
	s.nextTxID.Store(maxTxID)
	s.nextAcctID.Store(maxAcctID)
}

// Account operations

func (s *Store) AddAccount(a *Account) {
	s.mu.Lock()
	s.accounts[a.UserID] = append(s.accounts[a.UserID], a)
	s.accountIdx[a.ID] = a
	s.mu.Unlock()
}

func (s *Store) GetAccounts(uid int64) []*Account {
	s.mu.RLock()
	accs := s.accounts[uid]
	out := make([]*Account, len(accs))
	copy(out, accs)
	s.mu.RUnlock()
	return out
}

func (s *Store) GetAccount(id int64) *Account {
	s.mu.RLock()
	a := s.accountIdx[id]
	s.mu.RUnlock()
	return a
}

func (s *Store) UpdateAccount(id int64, fn func(*Account)) (*Account, bool) {
	s.mu.Lock()
	a := s.accountIdx[id]
	if a == nil {
		s.mu.Unlock()
		return nil, false
	}
	fn(a)
	s.mu.Unlock()
	return a, true
}

func (s *Store) DeleteAccount(id int64) (*Account, bool) {
	s.mu.Lock()
	a := s.accountIdx[id]
	if a == nil {
		s.mu.Unlock()
		return nil, false
	}
	delete(s.accountIdx, id)
	list := s.accounts[a.UserID]
	for i, ac := range list {
		if ac.ID == id {
			s.accounts[a.UserID] = append(list[:i], list[i+1:]...)
			break
		}
	}
	s.mu.Unlock()
	return a, true
}

// DuplicateMonth copies transactions from src to dst month, advances installments,
// sets carry-over from src final balance, recomputes dst summary.
func (s *Store) DuplicateMonth(uid int64, src, dst int32) []*Transaction {
	s.mu.Lock()
	srcTxs := s.txByMonth[uid][src]
	var created []*Transaction
	for _, t := range srcTxs {
		if t.InstallmentCur != nil && t.InstallmentTot != nil && *t.InstallmentCur >= *t.InstallmentTot {
			continue
		}
		nt := &Transaction{
			ID:             s.nextTxID.Add(1),
			UserID:         uid,
			AccountID:      t.AccountID,
			Type:           t.Type,
			Status:         StatusPending,
			Date:           advanceMonth(t.Date),
			Description:    t.Description,
			Category:       t.Category,
			AmountCents:    t.AmountCents,
			Currency:       t.Currency,
			RecurrenceType: t.RecurrenceType,
			InstallmentCur: t.InstallmentCur,
			InstallmentTot: t.InstallmentTot,
			YearMonth:      dst,
			CreatedAt:      time.Now().UTC(),
			UpdatedAt:      time.Now().UTC(),
		}
		if nt.InstallmentCur != nil {
			v := *nt.InstallmentCur + 1
			nt.InstallmentCur = &v
		}
		if s.txByMonth[uid] == nil {
			s.txByMonth[uid] = make(map[int32][]*Transaction)
		}
		s.txByMonth[uid][dst] = append(s.txByMonth[uid][dst], nt)
		s.txByID[nt.ID] = nt
		created = append(created, nt)
	}
	sort.Slice(s.txByMonth[uid][dst], func(i, j int) bool {
		return s.txByMonth[uid][dst][i].Date < s.txByMonth[uid][dst][j].Date
	})
	// carry-over from source
	var srcFinal int64
	if sm := s.summaries[uid]; sm != nil {
		if ss := sm[src]; ss != nil {
			srcFinal = ss.FinalBalance
		}
	}
	s.setCarryOver(uid, dst, srcFinal)
	s.recompute(uid, dst)
	s.mu.Unlock()
	return created
}

func advanceMonth(date string) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	t = t.AddDate(0, 1, 0)
	return t.Format("2006-01-02")
}

// LoadTx adds a transaction during store initialization (no lock, called before serving).
func (s *Store) LoadTx(tx *Transaction) {
	uid := tx.UserID
	ym := tx.YearMonth
	if s.txByMonth[uid] == nil {
		s.txByMonth[uid] = make(map[int32][]*Transaction)
	}
	s.txByMonth[uid][ym] = append(s.txByMonth[uid][ym], tx)
	s.txByID[tx.ID] = tx
}

// LoadAccount adds an account during store initialization (no lock).
func (s *Store) LoadAccount(a *Account) {
	s.accounts[a.UserID] = append(s.accounts[a.UserID], a)
	s.accountIdx[a.ID] = a
}

// LoadCarryOver sets carry-over during initialization (no lock).
func (s *Store) LoadCarryOver(uid int64, ym int32, cents int64) {
	if s.carryOver[uid] == nil {
		s.carryOver[uid] = make(map[int32]int64)
	}
	s.carryOver[uid][ym] = cents
}

// RecomputeAll recomputes summaries for all loaded data (call after loading, no lock).
func (s *Store) RecomputeAll() {
	for uid, months := range s.txByMonth {
		for ym, txs := range months {
			sort.Slice(txs, func(i, j int) bool { return txs[i].Date < txs[j].Date })
			s.recompute(uid, ym)
		}
	}
}
