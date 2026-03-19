package main

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type persistOp struct {
	query string
	args  []any
}

type Persister struct {
	pool    *pgxpool.Pool
	ch      chan persistOp
	wg      sync.WaitGroup
	done    chan struct{}
	batch   []persistOp
	mu      sync.Mutex
	ticker  *time.Ticker
}

func NewPersister(pool *pgxpool.Pool) *Persister {
	p := &Persister{
		pool:   pool,
		ch:     make(chan persistOp, 4096),
		done:   make(chan struct{}),
		ticker: time.NewTicker(100 * time.Millisecond),
	}
	p.wg.Add(1)
	go p.run()
	return p
}

func (p *Persister) Enqueue(query string, args ...any) {
	p.ch <- persistOp{query: query, args: args}
}

func (p *Persister) run() {
	defer p.wg.Done()
	for {
		select {
		case op := <-p.ch:
			p.mu.Lock()
			p.batch = append(p.batch, op)
			p.mu.Unlock()
		case <-p.ticker.C:
			p.flush()
		case <-p.done:
			// drain channel
			for {
				select {
				case op := <-p.ch:
					p.batch = append(p.batch, op)
				default:
					p.flush()
					return
				}
			}
		}
	}
}

func (p *Persister) flush() {
	p.mu.Lock()
	if len(p.batch) == 0 {
		p.mu.Unlock()
		return
	}
	ops := p.batch
	p.batch = nil
	p.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		log.Printf("persist: begin tx error: %v", err)
		return
	}
	for _, op := range ops {
		if _, err := tx.Exec(ctx, op.query, op.args...); err != nil {
			log.Printf("persist: exec error: %v query=%s", err, op.query)
			tx.Rollback(ctx)
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		log.Printf("persist: commit error: %v", err)
	}
}

func (p *Persister) Stop() {
	p.ticker.Stop()
	close(p.done)
	p.wg.Wait()
}

func loadStore(ctx context.Context, pool *pgxpool.Pool, s *Store) error {
	// Load transactions
	rows, err := pool.Query(ctx, `SELECT id, user_id, account_id, type, status, date, description, category, amount_cents, currency, recurrence_type, installment_cur, installment_tot, year_month, created_at, updated_at FROM transactions ORDER BY date`)
	if err != nil {
		return err
	}
	var maxTxID int64
	for rows.Next() {
		t := &Transaction{}
		var dt time.Time
		err := rows.Scan(&t.ID, &t.UserID, &t.AccountID, &t.Type, &t.Status, &dt, &t.Description, &t.Category, &t.AmountCents, &t.Currency, &t.RecurrenceType, &t.InstallmentCur, &t.InstallmentTot, &t.YearMonth, &t.CreatedAt, &t.UpdatedAt)
		if err != nil {
			rows.Close()
			return err
		}
		t.Date = dt.Format("2006-01-02")
		s.LoadTx(t)
		if t.ID > maxTxID {
			maxTxID = t.ID
		}
	}
	rows.Close()

	// Load accounts
	rows, err = pool.Query(ctx, `SELECT id, user_id, name, balance_cents, currency, created_at FROM accounts ORDER BY id`)
	if err != nil {
		return err
	}
	var maxAcctID int64
	for rows.Next() {
		a := &Account{}
		err := rows.Scan(&a.ID, &a.UserID, &a.Name, &a.BalanceCents, &a.Currency, &a.CreatedAt)
		if err != nil {
			rows.Close()
			return err
		}
		s.LoadAccount(a)
		if a.ID > maxAcctID {
			maxAcctID = a.ID
		}
	}
	rows.Close()

	// Load carry-overs
	rows, err = pool.Query(ctx, `SELECT user_id, year_month, carry_over_cents FROM month_balances`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var uid int64
		var ym int32
		var cents int64
		if err := rows.Scan(&uid, &ym, &cents); err != nil {
			rows.Close()
			return err
		}
		s.LoadCarryOver(uid, ym, cents)
	}
	rows.Close()

	s.SetIDCounters(maxTxID, maxAcctID)
	s.RecomputeAll()
	return nil
}
