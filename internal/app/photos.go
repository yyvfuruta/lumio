package app

import (
	"context"
	"io"

	"github.com/yyvfuruta/lumio/internal/core"
)

type Photo = core.Photo
type timelineMonth = core.Month

func validID(id string) bool { return core.ValidID(id) }
func (a *App) addPhoto(ctx context.Context, name string, src io.Reader) (Photo, bool, error) {
	return a.library.Upload(ctx, name, src, a.cfg.MaxUploadBytes)
}
func (a *App) collectOne(ctx context.Context) (bool, error) { return a.library.Collect(ctx) }
