package app

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"net/url"
	"time"
)

const sessionCookie = "lumio_session"

func (a *App) authenticated(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil || len(c.Value) != 64 {
		return false
	}
	hash := sha256.Sum256([]byte(c.Value))
	var exists bool
	err = a.db.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM lumio_sessions WHERE token_hash=$1 AND password_tag=$2 AND expires_at>now())", hash[:], a.passwordTag[:]).Scan(&exists)
	return err == nil && exists
}

func (a *App) protect(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.authenticated(r) {
			problem(w, http.StatusUnauthorized, "entre para acessar suas fotos")
			return
		}
		next(w, r)
	}
}

// Browsers cannot send the custom header cross-origin without a CORS preflight.
// We don't allow CORS, and also validate Origin and Fetch Metadata when present.
func safeMutation(r *http.Request) bool {
	if r.Header.Get("X-Lumio-Request") != "1" {
		return false
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Host != r.Host || (u.Scheme != "http" && u.Scheme != "https") {
			return false
		}
	}
	return true
}

func (a *App) login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	var attempts int
	err := a.db.QueryRow(r.Context(), `INSERT INTO lumio_login_limits(minute,attempts) VALUES(date_trunc('minute',now()),1)
	 ON CONFLICT(minute) DO UPDATE SET attempts=lumio_login_limits.attempts+1 RETURNING attempts`).Scan(&attempts)
	if err != nil {
		internal(w, err)
		return
	}
	if attempts > 20 {
		w.Header().Set("Retry-After", "60")
		problem(w, 429, "muitas tentativas; aguarde um minuto")
		return
	}
	hash := sha256.Sum256([]byte(input.Password))
	if subtle.ConstantTimeCompare(hash[:], a.passwordTag[:]) != 1 {
		problem(w, 401, "senha incorreta")
		return
	}
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		internal(w, err)
		return
	}
	value := hex.EncodeToString(token[:])
	tokenHash := sha256.Sum256([]byte(value))
	expires := time.Now().Add(7 * 24 * time.Hour)
	_, err = a.db.Exec(r.Context(), "INSERT INTO lumio_sessions(token_hash,password_tag,expires_at) VALUES($1,$2,$3)", tokenHash[:], a.passwordTag[:], expires)
	if err != nil {
		internal(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: value, Path: "/", HttpOnly: true, Secure: a.cfg.SecureCookie, SameSite: http.SameSiteStrictMode, Expires: expires, MaxAge: 7 * 24 * 3600})
	jsonResponse(w, 200, map[string]bool{"authenticated": true})
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		hash := sha256.Sum256([]byte(c.Value))
		if _, err := a.db.Exec(r.Context(), "DELETE FROM lumio_sessions WHERE token_hash=$1", hash[:]); err != nil {
			internal(w, err)
			return
		}
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: a.cfg.SecureCookie, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}
