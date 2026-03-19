package main

import (
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {}

func buildRouter(store *Store, persister *Persister, pool *pgxpool.Pool, jwtSecret []byte) http.Handler {
	auth := authMiddleware(jwtSecret)
	txH := &TxHandler{store: store, persist: persister}
	monthH := &MonthHandler{store: store, persist: persister}
	acctH := &AccountHandler{store: store, persist: persister}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

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
