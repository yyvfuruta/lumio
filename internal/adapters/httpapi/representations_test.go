package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAPIDiscovery(t *testing.T) {
	for _, path := range []string{"/api/v1", "/api/v1/"} {
		w := httptest.NewRecorder()
		(&API{}).Handler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Link") == "" {
			t.Fatal("missing discovery", w.Code)
		}
		var root struct {
			Links map[string]resourceLink `json:"links"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &root); err != nil {
			t.Fatal(err)
		}
		if root.Links["photos"].Href != "/api/v1/photos" || root.Links["sessions"].Method != "POST" {
			t.Fatal("missing resource controls")
		}
	}
}
func TestPhotoResourceControls(t *testing.T) {
	id := strings.Repeat("a", 64)
	raw, err := json.Marshal(representPhoto(Photo{ID: id, Name: "phone.jpg"}))
	if err != nil {
		t.Fatal(err)
	}
	var resource struct {
		ID    string                  `json:"id"`
		Links map[string]resourceLink `json:"links"`
	}
	if err = json.Unmarshal(raw, &resource); err != nil {
		t.Fatal(err)
	}
	if resource.ID != id || resource.Links["original"].Href != "/api/v1/photos/"+id+"/original" || resource.Links["favorite"].Method != "PATCH" || resource.Links["delete"].Method != "DELETE" {
		t.Fatal("missing photo controls")
	}
}
