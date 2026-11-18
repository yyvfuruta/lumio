package httpapi

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/yyvfuruta/lumio/internal/core"
)

const sessionCookie = "lumio_session"

func (a *API) authenticated(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	return err == nil && a.accounts.Authenticated(r.Context(), c.Value)
}

func (a *API) protect(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.authenticated(r) {
			problem(w, http.StatusUnauthorized, "sign in to access your photos")
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

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	value, expires, err := a.accounts.Login(r.Context(), input.Password)
	if err != nil {
		switch {
		case errors.Is(err, core.ErrThrottled):
			w.Header().Set("Retry-After", "60")
			problem(w, 429, err.Error())
		case errors.Is(err, core.ErrUnauthorized):
			problem(w, 401, err.Error())
		default:
			internal(w, err)
		}
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: value, Path: "/", HttpOnly: true, Secure: a.cfg.SecureCookie, SameSite: http.SameSiteStrictMode, Expires: expires, MaxAge: 7 * 24 * 3600})
	w.Header().Set("Location", "/api/v1/sessions/current")
	jsonResponse(w, http.StatusCreated, map[string]bool{"authenticated": true})
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if err := a.accounts.Logout(r.Context(), c.Value); err != nil {
			internal(w, err)
			return
		}
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: a.cfg.SecureCookie, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}
