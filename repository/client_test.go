package repository

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shyim/go-composer"
)

func TestPackagesJSONURL(t *testing.T) {
	assert.Equal(t, "https://repo.packagist.org/packages.json",
		New("https://repo.packagist.org", nil).packagesJSONURL())
	assert.Equal(t, "https://repo.packagist.org/packages.json",
		New("https://repo.packagist.org/", nil).packagesJSONURL())
	assert.Equal(t, "https://example.com/custom.json",
		New("https://example.com/custom.json", nil).packagesJSONURL())
}

func TestMatchPackagePattern(t *testing.T) {
	assert.True(t, matchPackagePattern("acme/*", "acme/lib"))
	assert.True(t, matchPackagePattern("acme/*", "acme/"))
	assert.False(t, matchPackagePattern("acme/*", "other/lib"))
	assert.True(t, matchPackagePattern("acme/lib", "acme/lib"))
	assert.False(t, matchPackagePattern("acme/lib", "acme/library"))
	assert.True(t, matchPackagePattern("*/lib", "acme/lib"))
}

func TestApplyAuthHeaders(t *testing.T) {
	var gotAuth, gotPrivate string
	repo, srv := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPrivate = r.Header.Get("PRIVATE-TOKEN")
		_, _ = w.Write([]byte(`{}`))
	}, nil)

	host := mustHost(t, srv.URL)

	t.Run("http-basic", func(t *testing.T) {
		repo.auth = &composer.Auth{HTTPBasicAuth: map[string]composer.BasicAuth{host: {Username: "u", Password: "p"}}}
		repo.root = nil
		_, err := repo.loadRoot(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "Basic dTpw", gotAuth) // base64("u:p")
	})

	t.Run("bearer", func(t *testing.T) {
		repo.auth = &composer.Auth{BearerAuth: map[string]string{host: "tok"}}
		repo.root = nil
		_, err := repo.loadRoot(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "Bearer tok", gotAuth)
	})

	t.Run("gitlab-token", func(t *testing.T) {
		repo.auth = &composer.Auth{GitlabAuth: map[string]composer.GitlabToken{host: {Token: "glpat"}}}
		repo.root = nil
		_, err := repo.loadRoot(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "glpat", gotPrivate)
	})
}
