package repository_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shyim/go-composer/repository"
)

// testProvider is an in-memory Provider implementing every capability
// interface, used to exercise the handler.
type testProvider struct {
	packages   map[string]*repository.Package
	searches   []repository.SearchResult
	advisories map[string][]repository.SecurityAdvisory
}

func (p *testProvider) Package(_ context.Context, name string) (*repository.Package, error) {
	pkg, ok := p.packages[strings.ToLower(name)]
	if !ok {
		return nil, repository.ErrPackageNotFound
	}
	return pkg, nil
}

func (p *testProvider) PackageNames(context.Context) ([]string, error) {
	names := make([]string, 0, len(p.packages))
	for name := range p.packages {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func (p *testProvider) Search(_ context.Context, query string) ([]repository.SearchResult, error) {
	var out []repository.SearchResult
	for _, r := range p.searches {
		if strings.Contains(r.Name, query) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (p *testProvider) SecurityAdvisories(_ context.Context, names []string) (map[string][]repository.SecurityAdvisory, error) {
	out := map[string][]repository.SecurityAdvisory{}
	for _, name := range names {
		if adv, ok := p.advisories[name]; ok {
			out[name] = adv
		}
	}
	return out, nil
}

func sampleProvider() *testProvider {
	return &testProvider{
		packages: map[string]*repository.Package{
			"acme/lib": {
				Name: "acme/lib",
				Versions: []repository.Version{
					{Name: "acme/lib", Version: "2.0.0", VersionNormalized: "2.0.0.0", Type: "library", Require: map[string]string{"php": ">=8.1"}},
					{Name: "acme/lib", Version: "1.0.0", VersionNormalized: "1.0.0.0", Type: "library", Require: map[string]string{"php": ">=7.4"}},
					{Name: "acme/lib", Version: "dev-main", VersionNormalized: "dev-main", Type: "library", Require: map[string]string{"php": ">=8.1"}},
				},
			},
		},
		searches: []repository.SearchResult{
			{Name: "acme/lib", Description: "a lib", Downloads: 42},
		},
		advisories: map[string][]repository.SecurityAdvisory{
			"acme/lib": {{AdvisoryID: "PKSA-1", PackageName: "acme/lib", AffectedVersions: "<1.0.1", CVE: "CVE-1"}},
		},
	}
}

// newClient stands up the handler on an httptest server and returns a
// repository.Client pointed at it.
func newClient(t *testing.T, p repository.Provider, opts ...repository.HandlerOption) *repository.Client {
	t.Helper()
	srv := httptest.NewServer(repository.NewHandler(p, opts...))
	t.Cleanup(srv.Close)
	c := repository.New(srv.URL, nil)
	c.HTTPClient = srv.Client()
	return c
}

// TestRoundTrip serves with the handler and reads back with the client,
// covering the stable/dev split, minification, and the shared wire types.
func TestRoundTrip(t *testing.T) {
	c := newClient(t, sampleProvider())

	pkg, err := c.GetPackage(context.Background(), "acme/lib")
	require.NoError(t, err)
	require.Equal(t, "acme/lib", pkg.Name)
	require.Len(t, pkg.Versions, 3, "2 stable + 1 dev merged by the client")

	v1 := pkg.Version("1.0.0")
	require.NotNil(t, v1)
	assert.Equal(t, map[string]string{"php": ">=7.4"}, v1.Require)
	assert.Equal(t, "library", v1.Type)

	require.NotNil(t, pkg.Version("2.0.0"))
	require.NotNil(t, pkg.Version("dev-main"))
}

func TestStableAndDevFilesAreSeparated(t *testing.T) {
	srv := httptest.NewServer(repository.NewHandler(sampleProvider()))
	t.Cleanup(srv.Close)

	get := func(path string) repository.Metadata {
		resp, err := http.Get(srv.URL + path)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var meta repository.Metadata
		require.NoError(t, decodeJSON(resp, &meta))
		return meta
	}

	stable := get("/p2/acme/lib.json")
	versions, err := repository.DecodePackageVersions(stable.Packages["acme/lib"], stable.Minified == "composer/2.0")
	require.NoError(t, err)
	require.Len(t, versions, 2)
	for _, v := range versions {
		assert.NotEqual(t, "dev-main", v.Version, "stable file must not contain dev versions")
	}

	dev := get("/p2/acme/lib~dev.json")
	devVersions, err := repository.DecodePackageVersions(dev.Packages["acme/lib"], dev.Minified == "composer/2.0")
	require.NoError(t, err)
	require.Len(t, devVersions, 1)
	assert.Equal(t, "dev-main", devVersions[0].Version)
}

func TestPackagesJSONAdvertisesCapabilities(t *testing.T) {
	t.Run("full provider", func(t *testing.T) {
		root := fetchRoot(t, sampleProvider())
		assert.Equal(t, "/p2/%package%.json", root.MetadataURL)
		assert.Equal(t, []string{"acme/lib"}, root.AvailablePackages)
		assert.NotEmpty(t, root.SearchURL)
		require.NotNil(t, root.SecurityAdvisories)
		assert.NotEmpty(t, root.SecurityAdvisories.APIURL)
	})

	t.Run("minimal provider", func(t *testing.T) {
		// bareProvider hides the optional capability interfaces.
		root := fetchRoot(t, bareProvider{sampleProvider()})
		assert.Equal(t, "/p2/%package%.json", root.MetadataURL)
		assert.Empty(t, root.AvailablePackages)
		assert.Empty(t, root.SearchURL)
		assert.Nil(t, root.SecurityAdvisories)
	})
}

// bareProvider hides any optional capability interfaces of the embedded value.
type bareProvider struct{ p repository.Provider }

func (b bareProvider) Package(ctx context.Context, name string) (*repository.Package, error) {
	return b.p.Package(ctx, name)
}

func TestSearchEndpoint(t *testing.T) {
	c := newClient(t, sampleProvider())
	results, err := c.Search(context.Background(), "acme")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "acme/lib", results[0].Name)
	assert.Equal(t, 42, results[0].Downloads)
}

func TestAdvisoriesEndpoint(t *testing.T) {
	c := newClient(t, sampleProvider())
	adv, err := c.GetSecurityAdvisories(context.Background(), []string{"acme/lib"})
	require.NoError(t, err)
	require.Len(t, adv["acme/lib"], 1)
	assert.Equal(t, "CVE-1", adv["acme/lib"][0].CVE)
}

func TestPackageNotFound(t *testing.T) {
	c := newClient(t, sampleProvider())
	_, err := c.GetPackage(context.Background(), "acme/unknown")
	assert.ErrorIs(t, err, repository.ErrPackageNotFound)
}

func TestSearchEndpointNotWiredWithoutCapability(t *testing.T) {
	p := bareProvider{sampleProvider()}
	srv := httptest.NewServer(repository.NewHandler(p))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/search.json?q=acme")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestAdvisoriesRequiresPost(t *testing.T) {
	srv := httptest.NewServer(repository.NewHandler(sampleProvider()))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/security-advisories.json")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

func TestMalformedMetadataPath(t *testing.T) {
	srv := httptest.NewServer(repository.NewHandler(sampleProvider()))
	t.Cleanup(srv.Close)

	for _, path := range []string{"/p2/noslash.json", "/p2/a/b/c.json", "/p2/acme/lib.txt"} {
		resp, err := http.Get(srv.URL + path)
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, path)
	}
}

func TestAdvisoriesFormEncoding(t *testing.T) {
	srv := httptest.NewServer(repository.NewHandler(sampleProvider()))
	t.Cleanup(srv.Close)

	resp, err := http.PostForm(srv.URL+"/security-advisories.json", url.Values{"packages[]": {"acme/lib"}})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Advisories map[string][]repository.SecurityAdvisory `json:"advisories"`
	}
	require.NoError(t, decodeJSON(resp, &body))
	require.Len(t, body.Advisories["acme/lib"], 1)
}

// --- test helpers ---

func decodeJSON(resp *http.Response, v any) error {
	return json.NewDecoder(resp.Body).Decode(v)
}

func fetchRoot(t *testing.T, p repository.Provider) repository.RootFile {
	t.Helper()
	srv := httptest.NewServer(repository.NewHandler(p))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/packages.json")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var root repository.RootFile
	require.NoError(t, decodeJSON(resp, &root))
	return root
}
