package core

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"time"
)

var ErrUnauthorized = errors.New("incorrect password")
var ErrThrottled = errors.New("too many attempts; wait one minute")

type Sessions interface {
	Attempt(context.Context) (int, error)
	Valid(context.Context, []byte, []byte) (bool, error)
	SaveSession(context.Context, []byte, []byte, time.Time) error
	Revoke(context.Context, []byte) error
}
type Accounts struct {
	sessions    Sessions
	passwordTag [32]byte
}

func NewAccounts(sessions Sessions, password string) *Accounts {
	return &Accounts{sessions: sessions, passwordTag: sha256.Sum256([]byte(password))}
}
func (a *Accounts) Authenticated(ctx context.Context, token string) bool {
	if len(token) != 64 {
		return false
	}
	hash := sha256.Sum256([]byte(token))
	ok, err := a.sessions.Valid(ctx, hash[:], a.passwordTag[:])
	return err == nil && ok
}
func (a *Accounts) Login(ctx context.Context, password string) (string, time.Time, error) {
	attempts, err := a.sessions.Attempt(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	if attempts > 20 {
		return "", time.Time{}, ErrThrottled
	}
	hash := sha256.Sum256([]byte(password))
	if subtle.ConstantTimeCompare(hash[:], a.passwordTag[:]) != 1 {
		return "", time.Time{}, ErrUnauthorized
	}
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", time.Time{}, err
	}
	value := hex.EncodeToString(token[:])
	digest := sha256.Sum256([]byte(value))
	expires := time.Now().Add(7 * 24 * time.Hour)
	if err := a.sessions.SaveSession(ctx, digest[:], a.passwordTag[:], expires); err != nil {
		return "", time.Time{}, err
	}
	return value, expires, nil
}
func (a *Accounts) Logout(ctx context.Context, token string) error {
	hash := sha256.Sum256([]byte(token))
	return a.sessions.Revoke(ctx, hash[:])
}
