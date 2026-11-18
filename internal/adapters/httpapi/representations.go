package httpapi

import (
	"net/http"

	"github.com/yyvfuruta/lumio/internal/core"
)

type resourceLink struct {
	Href   string `json:"href"`
	Method string `json:"method"`
}
type photoRepresentation struct {
	core.Photo
	Links map[string]resourceLink `json:"links"`
}

func representPhoto(p core.Photo) photoRepresentation {
	base := "/api/v1/photos/" + p.ID
	return photoRepresentation{Photo: p, Links: map[string]resourceLink{
		"self": {Href: base, Method: "GET"}, "collection": {Href: "/api/v1/photos", Method: "GET"},
		"original": {Href: base + "/original", Method: "GET"}, "preview": {Href: base + "/preview", Method: "GET"},
		"thumbnail": {Href: base + "/thumbnail", Method: "GET"}, "favorite": {Href: base, Method: "PATCH"}, "delete": {Href: base, Method: "DELETE"},
	}}
}
func apiRoot(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, 200, map[string]any{"links": map[string]resourceLink{
		"self": {Href: "/api/v1", Method: "GET"}, "photos": {Href: "/api/v1/photos", Method: "GET"},
		"sessions": {Href: "/api/v1/sessions", Method: "POST"}, "session": {Href: "/api/v1/sessions/current", Method: "GET"},
		"logout": {Href: "/api/v1/sessions/current", Method: "DELETE"}, "status": {Href: "/api/v1/status", Method: "GET"},
		"timeline": {Href: "/api/v1/photos/timeline", Method: "GET"}, "archives": {Href: "/api/v1/photo-archives", Method: "POST"},
	}})
}
