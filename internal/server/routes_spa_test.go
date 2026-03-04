package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSPARoutesDoNotRedirect(t *testing.T) {
	s := &Server{opts: Options{Dev: true}}
	h := s.routes()
	ts := httptest.NewServer(h)
	defer ts.Close()

	paths := []string{
		"/",
		"/index.html",
		"/apps/test",
		"/apps/test/deep/link",
	}

	client := ts.Client()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		t.Fatalf("unexpected redirect for %q to %q", via[0].URL.Path, req.URL.Path)
		return nil
	}

	for _, p := range paths {
		resp, err := client.Get(ts.URL + p)
		if err != nil {
			t.Fatalf("GET %s failed: %v", p, err)
		}
		resp.Body.Close()
	}
}

