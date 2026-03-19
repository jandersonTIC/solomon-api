package main

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type appleKey struct {
	KID string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type appleKeySet struct {
	Keys []appleKey `json:"keys"`
}

type AppleAuth struct {
	mu     sync.RWMutex
	keys   map[string]*rsa.PublicKey
	expiry time.Time
}

func NewAppleAuth() *AppleAuth {
	return &AppleAuth{keys: make(map[string]*rsa.PublicKey)}
}

func (a *AppleAuth) GetKey(kid string) (*rsa.PublicKey, error) {
	a.mu.RLock()
	if time.Now().Before(a.expiry) {
		if k, ok := a.keys[kid]; ok {
			a.mu.RUnlock()
			return k, nil
		}
	}
	a.mu.RUnlock()

	if err := a.refresh(); err != nil {
		return nil, err
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	if k, ok := a.keys[kid]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("unknown kid: %s", kid)
}

func (a *AppleAuth) refresh() error {
	resp, err := http.Get("https://appleid.apple.com/auth/keys")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var ks appleKeySet
	if err := json.Unmarshal(body, &ks); err != nil {
		return err
	}
	keys := make(map[string]*rsa.PublicKey, len(ks.Keys))
	for _, k := range ks.Keys {
		nb, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		eb, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		n := new(big.Int).SetBytes(nb)
		e := int(new(big.Int).SetBytes(eb).Int64())
		keys[k.KID] = &rsa.PublicKey{N: n, E: e}
	}
	a.mu.Lock()
	a.keys = keys
	a.expiry = time.Now().Add(24 * time.Hour)
	a.mu.Unlock()
	return nil
}

type AuthHandler struct {
	pool      *pgxpool.Pool
	store     *Store
	apple     *AppleAuth
	jwtSecret []byte
}

type appleTokenRequest struct {
	IdentityToken string `json:"identity_token"`
}

type authResponse struct {
	Token string `json:"token"`
	User  *User  `json:"user"`
}

func (h *AuthHandler) HandleAppleAuth(w http.ResponseWriter, r *http.Request) {
	var req appleTokenRequest
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	parser := jwtlib.NewParser(jwtlib.WithoutClaimsValidation())
	token, _, err := parser.ParseUnverified(req.IdentityToken, jwtlib.MapClaims{})
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid token format")
		return
	}
	kid, _ := token.Header["kid"].(string)
	pubKey, err := h.apple.GetKey(kid)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "cannot verify token")
		return
	}
	verified, err := jwtlib.Parse(req.IdentityToken, func(t *jwtlib.Token) (any, error) {
		return pubKey, nil
	}, jwtlib.WithValidMethods([]string{"RS256"}))
	if err != nil || !verified.Valid {
		writeErr(w, http.StatusUnauthorized, "invalid apple token")
		return
	}
	claims, _ := verified.Claims.(jwtlib.MapClaims)
	sub, _ := claims["sub"].(string)
	email, _ := claims["email"].(string)
	if sub == "" {
		writeErr(w, http.StatusBadRequest, "missing sub")
		return
	}

	ctx := r.Context()
	user, err := h.upsertUser(ctx, sub, email)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "user error")
		return
	}

	sessionToken := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, jwtlib.MapClaims{
		"uid": float64(user.ID),
		"exp": float64(time.Now().Add(30 * 24 * time.Hour).Unix()),
	})
	signed, err := sessionToken.SignedString(h.jwtSecret)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "token error")
		return
	}
	writeJSON(w, http.StatusOK, authResponse{Token: signed, User: user})
}

func (h *AuthHandler) upsertUser(ctx context.Context, appleSub, email string) (*User, error) {
	var u User
	err := h.pool.QueryRow(ctx,
		`INSERT INTO users(apple_sub, email) VALUES($1, $2)
		 ON CONFLICT(apple_sub) DO UPDATE SET email=EXCLUDED.email
		 RETURNING id, apple_sub, email, name, created_at`,
		appleSub, email).Scan(&u.ID, &u.AppleSub, &u.Email, &u.Name, &u.CreatedAt)
	return &u, err
}
