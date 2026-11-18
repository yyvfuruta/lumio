package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestCSRF(t *testing.T) {
	for _, tc := range []struct {
		header, origin, site string
		want                 bool
	}{
		{"", "", "", false}, {"1", "", "", true}, {"1", "https://photos.test", "same-origin", true}, {"1", "https://evil.test", "", false}, {"1", "null", "", false}, {"1", "https://photos.test", "cross-site", false},
	} {
		r := httptest.NewRequest("POST", "https://photos.test/api/v1/photos", nil)
		r.Header.Set("X-Lumio-Request", tc.header)
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Sec-Fetch-Site", tc.site)
		if safeMutation(r) != tc.want {
			t.Fatalf("case %+v", tc)
		}
	}
}
