package repository

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	packagist "github.com/shyim/go-packagist"
)

// newTestRepo wires a Client to an httptest server.
func newTestRepo(t *testing.T, handler http.HandlerFunc, auth *packagist.ComposerAuth) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	repo := New(srv.URL, auth)
	repo.HTTPClient = srv.Client()
	return repo, srv
}

func TestGetPackageV2Minified(t *testing.T) {
	var gotPaths []string
	repo, _ := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		gotPaths = append(gotPaths, r.URL.Path)
		switch r.URL.Path {
		case "/packages.json":
			_, _ = w.Write([]byte(`{"metadata-url":"/p2/%package%.json","search":"/search.json?q=%query%"}`))
		case "/p2/acme/lib.json":
			_, _ = w.Write([]byte(`{
				"minified":"composer/2.0",
				"packages":{"acme/lib":[
					{"name":"acme/lib","version":"2.0.0","version_normalized":"2.0.0.0","type":"library","require":{"php":">=8.1"},"dist":{"type":"zip","url":"https://ex/2.0.0.zip"}},
					{"version":"1.0.0","version_normalized":"1.0.0.0","require":{"php":">=7.4"}}
				]}
			}`))
		case "/p2/acme/lib~dev.json":
			_, _ = w.Write([]byte(`{"packages":{"acme/lib":[
				{"name":"acme/lib","version":"dev-main","version_normalized":"dev-main","type":"library","require":{"php":">=8.1"}}
			]}}`))
		default:
			http.NotFound(w, r)
		}
	}, nil)

	meta, err := repo.GetPackage(context.Background(), "acme/lib")
	require.NoError(t, err)
	require.Equal(t, "acme/lib", meta.Name)
	require.Len(t, meta.Versions, 3) // 2 stable + 1 dev

	// Delta expansion: the second version inherits type from the first.
	v1 := meta.Version("1.0.0")
	require.NotNil(t, v1)
	assert.Equal(t, "library", v1.Type)
	assert.Equal(t, map[string]string{"php": ">=7.4"}, v1.Require)

	v2 := meta.Version("2.0.0")
	require.NotNil(t, v2)
	assert.Equal(t, "zip", v2.Dist.Type)
	assert.Equal(t, "https://ex/2.0.0.zip", v2.Dist.URL)

	dev := meta.Version("dev-main")
	require.NotNil(t, dev)

	assert.Contains(t, gotPaths, "/p2/acme/lib.json")
	assert.Contains(t, gotPaths, "/p2/acme/lib~dev.json")
}

func TestGetPackageNotFound(t *testing.T) {
	repo, _ := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/packages.json" {
			_, _ = w.Write([]byte(`{"metadata-url":"/p2/%package%.json"}`))
			return
		}
		http.NotFound(w, r)
	}, nil)

	_, err := repo.GetPackage(context.Background(), "missing/pkg")
	assert.ErrorIs(t, err, ErrPackageNotFound)
}

func TestGetPackageRespectsAvailableList(t *testing.T) {
	var fetchedMetadata bool
	repo, _ := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/packages.json":
			_, _ = w.Write([]byte(`{"metadata-url":"/p2/%package%.json","available-packages":["acme/known"]}`))
		default:
			fetchedMetadata = true
			http.NotFound(w, r)
		}
	}, nil)

	_, err := repo.GetPackage(context.Background(), "acme/unknown")
	assert.ErrorIs(t, err, ErrPackageNotFound)
	assert.False(t, fetchedMetadata, "should not hit metadata-url for a package outside available-packages")
}

func TestGetPackageInlinePartialPackages(t *testing.T) {
	repo, _ := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/packages.json", r.URL.Path)
		_, _ = w.Write([]byte(`{
			"metadata-url":"/p2/%package%.json",
			"packages":{"acme/inline":[{"name":"acme/inline","version":"1.2.3","version_normalized":"1.2.3.0"}]}
		}`))
	}, nil)

	meta, err := repo.GetPackage(context.Background(), "acme/inline")
	require.NoError(t, err)
	require.Len(t, meta.Versions, 1)
	assert.Equal(t, "1.2.3", meta.Versions[0].Version)
}

func TestSearch(t *testing.T) {
	repo, _ := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/packages.json":
			_, _ = w.Write([]byte(`{"metadata-url":"/p2/%package%.json","search":"/search.json?q=%query%&type=%type%"}`))
		case "/search.json":
			assert.Equal(t, "monolog", r.URL.Query().Get("q"))
			_, _ = w.Write([]byte(`{"results":[
				{"name":"monolog/monolog","description":"logging","downloads":1000},
				{"name":"virtual/thing","virtual":true}
			]}`))
		default:
			http.NotFound(w, r)
		}
	}, nil)

	results, err := repo.Search(context.Background(), "monolog")
	require.NoError(t, err)
	require.Len(t, results, 1) // virtual filtered out
	assert.Equal(t, "monolog/monolog", results[0].Name)
	assert.Equal(t, 1000, results[0].Downloads)
}

func TestGetSecurityAdvisoriesAPI(t *testing.T) {
	repo, _ := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/packages.json":
			_, _ = w.Write([]byte(`{"metadata-url":"/p2/%package%.json","security-advisories":{"api-url":"/advisories.json"}}`))
		case "/advisories.json":
			require.Equal(t, http.MethodPost, r.Method)
			require.NoError(t, r.ParseForm())
			assert.Contains(t, r.PostForm["packages[]"], "acme/lib")
			_, _ = w.Write([]byte(`{"advisories":{"acme/lib":[
				{"advisoryId":"PKSA-1","packageName":"acme/lib","affectedVersions":"<1.0.1","title":"XSS","cve":"CVE-1"}
			]}}`))
		default:
			http.NotFound(w, r)
		}
	}, nil)

	adv, err := repo.GetSecurityAdvisories(context.Background(), []string{"acme/lib"})
	require.NoError(t, err)
	require.Len(t, adv["acme/lib"], 1)
	assert.Equal(t, "CVE-1", adv["acme/lib"][0].CVE)
	assert.Equal(t, "<1.0.1", adv["acme/lib"][0].AffectedVersions)
}

func TestGetSecurityAdvisoriesNoneConfigured(t *testing.T) {
	repo, _ := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"metadata-url":"/p2/%package%.json"}`))
	}, nil)

	adv, err := repo.GetSecurityAdvisories(context.Background(), []string{"acme/lib"})
	require.NoError(t, err)
	assert.Empty(t, adv)
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
		repo.auth = &packagist.ComposerAuth{HTTPBasicAuth: map[string]packagist.ComposerAuthHttpBasic{host: {Username: "u", Password: "p"}}}
		repo.root = nil
		_, err := repo.loadRoot(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "Basic dTpw", gotAuth) // base64("u:p")
	})

	t.Run("bearer", func(t *testing.T) {
		repo.auth = &packagist.ComposerAuth{BearerAuth: map[string]string{host: "tok"}}
		repo.root = nil
		_, err := repo.loadRoot(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "Bearer tok", gotAuth)
	})

	t.Run("gitlab-token", func(t *testing.T) {
		repo.auth = &packagist.ComposerAuth{GitlabAuth: map[string]packagist.GitlabToken{host: {Token: "glpat"}}}
		repo.root = nil
		_, err := repo.loadRoot(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "glpat", gotPrivate)
	})
}

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

func mustHost(t *testing.T, rawURL string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	require.NoError(t, err)
	return u.Host
}

func TestVersionLookup(t *testing.T) {
	meta := &Package{Versions: []Version{
		{Version: "1.0.0", VersionNormalized: "1.0.0.0"},
		{Version: "2.0.0", VersionNormalized: "2.0.0.0"},
	}}
	assert.Equal(t, "1.0.0", meta.Version("1.0.0").Version)
	assert.Equal(t, "2.0.0", meta.Version("2.0.0.0").Version) // by normalized
	assert.Nil(t, meta.Version("9.9.9"))
}
