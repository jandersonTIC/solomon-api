package main

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type ctxKey int

const ctxUserID ctxKey = 0

func userIDFrom(ctx context.Context) int64 {
	v, _ := ctx.Value(ctxUserID).(int64)
	return v
}

func authMiddleware(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if !strings.HasPrefix(h, "Bearer ") {
				writeErr(w, http.StatusUnauthorized, "missing token")
				return
			}
			tok, err := jwt.Parse(h[7:], func(t *jwt.Token) (any, error) {
				return secret, nil
			}, jwt.WithValidMethods([]string{"HS256"}))
			if err != nil || !tok.Valid {
				writeErr(w, http.StatusUnauthorized, "invalid token")
				return
			}
			claims, ok := tok.Claims.(jwt.MapClaims)
			if !ok {
				writeErr(w, http.StatusUnauthorized, "invalid claims")
				return
			}
			uid, ok := claims["uid"].(float64)
			if !ok {
				writeErr(w, http.StatusUnauthorized, "invalid uid")
				return
			}
			ctx := context.WithValue(r.Context(), ctxUserID, int64(uid))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func timingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		w.Header().Set("X-Response-Time", time.Since(start).String())
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
