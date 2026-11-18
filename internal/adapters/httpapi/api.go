// Package httpapi adapts HTTP requests and resource representations to core use cases.
package httpapi

import (
	"context"
	"io"
	"net/http"

	"github.com/yyvfuruta/lumio/internal/core"
)

type Config struct {
	StorageURL     string
	SecureCookie   bool
	MaxUploadBytes int64
}
type API struct {
	cfg        Config
	library    *core.Library
	accounts   *core.Accounts
	ready      func(context.Context) error
	uploadGate chan struct{}
}

// New wires only use cases and a readiness probe; it never opens infrastructure.
func New(cfg Config, library *core.Library, accounts *core.Accounts, ready func(context.Context) error) http.Handler {
	return (&API{cfg: cfg, library: library, accounts: accounts, ready: ready, uploadGate: make(chan struct{}, 1)}).Handler()
}

type Photo = core.Photo

func validID(id string) bool { return core.ValidID(id) }
func (a *API) addPhoto(ctx context.Context, name string, src io.Reader) (Photo, bool, error) {
	return a.library.Upload(ctx, name, src, a.cfg.MaxUploadBytes)
}
