package repository

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/shyim/go-composer"
)

// newTestRepo wires a Client to an httptest server.
func newTestRepo(t *testing.T, handler http.HandlerFunc, auth *composer.Auth) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	repo := New(srv.URL, auth)
	repo.HTTPClient = srv.Client()
	return repo, srv
}

func mustHost(t *testing.T, rawURL string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	require.NoError(t, err)
	return u.Host
}
