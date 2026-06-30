package platform

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	cookieName    = "pk_session"
	sessionTTL    = 24 * time.Hour
	loginFailDelay = 200 * time.Millisecond
)

// sessionStore is an in-memory token→expiry map with sliding renewal.
type sessionStore struct {
	mu     sync.Mutex
	tokens map[string]time.Time
}

func newSessionStore() *sessionStore {
	ss := &sessionStore{tokens: map[string]time.Time{}}
	go ss.reapLoop()
	return ss
}

func (ss *sessionStore) issue() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("platform/auth: rand: %v", err))
	}
	tok := hex.EncodeToString(b)
	ss.mu.Lock()
	ss.tokens[tok] = time.Now().Add(sessionTTL)
	ss.mu.Unlock()
	return tok
}

func (ss *sessionStore) validate(tok string) bool {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	exp, ok := ss.tokens[tok]
	if !ok || time.Now().After(exp) {
		delete(ss.tokens, tok)
		return false
	}
	ss.tokens[tok] = time.Now().Add(sessionTTL) // sliding
	return true
}

func (ss *sessionStore) revoke(tok string) {
	ss.mu.Lock()
	delete(ss.tokens, tok)
	ss.mu.Unlock()
}

func (ss *sessionStore) reapLoop() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		ss.mu.Lock()
		now := time.Now()
		for tok, exp := range ss.tokens {
			if now.After(exp) {
				delete(ss.tokens, tok)
			}
		}
		ss.mu.Unlock()
	}
}

// authState holds the admin email, hashed password and session store for the
// platform server.
type authState struct {
	email    string
	hash     []byte
	sessions *sessionStore
}

// newAuthState reads POCKETKNIFE_ADMIN_EMAIL and POCKETKNIFE_ADMIN_PASSWORD;
// if the password is absent, generates and prints one alongside the email.
func newAuthState() (*authState, error) {
	email := os.Getenv("POCKETKNIFE_ADMIN_EMAIL")
	if email == "" {
		email = "admin@pocketknife.local"
	}
	pw := os.Getenv("POCKETKNIFE_ADMIN_PASSWORD")
	if pw == "" {
		pw = randomPassword(16)
		fmt.Printf("\n╔══════════════════════════════════════════╗\n")
		fmt.Printf("║  POCKETKNIFE ADMIN EMAIL:    %-12s ║\n", email)
		fmt.Printf("║  POCKETKNIFE ADMIN PASSWORD: %-12s ║\n", pw)
		fmt.Printf("╚══════════════════════════════════════════╝\n\n")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash admin password: %w", err)
	}
	return &authState{email: email, hash: h, sessions: newSessionStore()}, nil
}

func randomPassword(n int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		b[i] = chars[idx.Int64()]
	}
	return string(b)
}

// handleLogin implements POST /platform/auth/login.
func (a *authState) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	emailOK := strings.EqualFold(strings.TrimSpace(body.Email), a.email)
	passErr := bcrypt.CompareHashAndPassword(a.hash, []byte(body.Password))
	if !emailOK || passErr != nil {
		time.Sleep(loginFailDelay)
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
		return
	}
	tok := a.sessions.issue()
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    tok,
		Path:     "/platform",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleLogout implements POST /platform/auth/logout.
func (a *authState) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if c, err := r.Cookie(cookieName); err == nil {
		a.sessions.revoke(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Path:     "/platform",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// authMiddleware wraps a handler, requiring a valid pk_session cookie except on
// the two auth endpoints themselves.
func (a *authState) authMiddleware(next http.Handler) http.Handler {
	exempt := map[string]bool{
		"/platform/auth/login":  true,
		"/platform/auth/logout": true,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Trim trailing slashes for comparison.
		path := strings.TrimRight(r.URL.Path, "/")
		if exempt[path] {
			next.ServeHTTP(w, r)
			return
		}
		c, err := r.Cookie(cookieName)
		if err != nil || !a.sessions.validate(c.Value) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}
