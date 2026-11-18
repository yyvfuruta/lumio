package httpapi

import (
	"net/http"
	"time"

	"github.com/yyvfuruta/lumio/internal/core"
)

type timelineMonth = core.Month

func photoRange(from, until string) (*time.Time, *time.Time, error) {
	return core.PhotoRange(from, until)
}
func (a *API) timeline(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	months, err := a.library.Timeline(r.Context(), q.Get("tz"), q.Get("q"), q.Get("favorite") == "true")
	if err != nil {
		coreProblem(w, err)
		return
	}
	jsonResponse(w, 200, map[string]any{"months": months})
}
