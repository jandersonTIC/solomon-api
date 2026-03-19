package main

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func newTestServer(t *testing.T) (*Store, http.Handler, []byte) {
	t.Helper()
	store := NewStore()
	store.SetIDCounters(0, 0)
	secret := []byte("test-secret")
	persist := &Persister{ch: make(chan persistOp, 4096)}
	handler := buildRouter(store, persist, nil, secret)
	return store, handler, secret
}

func testToken(t *testing.T, uid int64, secret []byte) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"uid": float64(uid),
		"exp": float64(time.Now().Add(time.Hour).Unix()),
	})
	s, err := token.SignedString(secret)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func fmtID(id int64) string {
	return fmt.Sprintf("%d", id)
}
