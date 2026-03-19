package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	port := env("PORT", "8080")
	dbURL := env("DATABASE_URL", "postgres://solomon:solomon@localhost:5432/solomon?sslmode=disable")
	jwtSecret := []byte(env("JWT_SECRET", "dev-secret-change-in-prod"))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	pool, err := pgxpool.New(ctx, dbURL)
	cancel()
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	if err := runMigrations(ctx, pool); err != nil {
		log.Fatalf("migrations: %v", err)
	}
	cancel()

	store := NewStore()
	ctx, cancel = context.WithTimeout(context.Background(), 60*time.Second)
	if err := loadStore(ctx, pool, store); err != nil {
		log.Fatalf("load store: %v", err)
	}
	cancel()
	log.Printf("store loaded: %d transactions, %d accounts", len(store.txByID), len(store.accountIdx))

	persister := NewPersister(pool)
	defer persister.Stop()

	mux := buildRouter(store, persister, pool, jwtSecret)

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("listening on :%s", port)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}

func buildRouter(store *Store, persister *Persister, pool *pgxpool.Pool, jwtSecret []byte) http.Handler {
	auth := authMiddleware(jwtSecret)
	txH := &TxHandler{store: store, persist: persister}
	monthH := &MonthHandler{store: store, persist: persister}
	acctH := &AccountHandler{store: store, persist: persister}
	appleH := &AuthHandler{pool: pool, store: store, apple: NewAppleAuth(), jwtSecret: jwtSecret}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"alive"}`))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if pool != nil {
			if err := pool.Ping(ctx); err != nil {
				writeErr(w, http.StatusServiceUnavailable, "db unreachable")
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ready"}`))
	})

	mux.HandleFunc("POST /v1/auth/apple", appleH.HandleAppleAuth)

	protected := http.NewServeMux()
	protected.HandleFunc("GET /v1/transactions", txH.List)
	protected.HandleFunc("POST /v1/transactions", txH.Create)
	protected.HandleFunc("PUT /v1/transactions/{id}", txH.Update)
	protected.HandleFunc("DELETE /v1/transactions/{id}", txH.Delete)
	protected.HandleFunc("PATCH /v1/transactions/{id}/status", txH.PatchStatus)

	protected.HandleFunc("GET /v1/months/{ym}/summary", monthH.Summary)
	protected.HandleFunc("POST /v1/months/{ym}/duplicate", monthH.Duplicate)
	protected.HandleFunc("GET /v1/months/{ym}/categories", monthH.Categories)

	protected.HandleFunc("GET /v1/accounts", acctH.List)
	protected.HandleFunc("POST /v1/accounts", acctH.Create)
	protected.HandleFunc("PUT /v1/accounts/{id}", acctH.Update)
	protected.HandleFunc("DELETE /v1/accounts/{id}", acctH.Delete)

	mux.Handle("/v1/", auth(protected))

	return corsMiddleware(timingMiddleware(mux))
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
