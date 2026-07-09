package repository_test

import (
	"context"
	"encoding/json"
	"github.com/shyim/go-composer/internal/testassert"
	"github.com/shyim/go-composer/repository"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
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
	testassert.RequireNoError(t, err)
	testassert.RequireEqual(t, "acme/lib", pkg.Name)
	testassert.RequireLen(t, pkg.Versions, 3, "2 stable + 1 dev merged by the client")

	v1 := pkg.Version("1.0.0")
	testassert.RequireNotNil(t, v1)
	testassert.Equal(t, map[string]string{"php": ">=7.4"}, v1.Require)
	testassert.Equal(t, "library", v1.Type)

	testassert.RequireNotNil(t, pkg.Version("2.0.0"))
	testassert.RequireNotNil(t, pkg.Version("dev-main"))
}

func TestStableAndDevFilesAreSeparated(t *testing.T) {
	srv := httptest.NewServer(repository.NewHandler(sampleProvider()))
	t.Cleanup(srv.Close)

	get := func(path string) repository.Metadata {
		resp, err := http.Get(srv.URL + path)
		testassert.RequireNoError(t, err)
		defer resp.Body.Close()
		testassert.RequireEqual(t, http.StatusOK, resp.StatusCode)
		var meta repository.Metadata
		testassert.RequireNoError(t, decodeJSON(resp, &meta))
		return meta
	}

	stable := get("/p2/acme/lib.json")
	versions, err := repository.DecodePackageVersions(stable.Packages["acme/lib"], stable.Minified == "composer/2.0")
	testassert.RequireNoError(t, err)
	testassert.RequireLen(t, versions, 2)
	for _, v := range versions {
		testassert.NotEqual(t, "dev-main", v.Version, "stable file must not contain dev versions")
	}

	dev := get("/p2/acme/lib~dev.json")
	devVersions, err := repository.DecodePackageVersions(dev.Packages["acme/lib"], dev.Minified == "composer/2.0")
	testassert.RequireNoError(t, err)
	testassert.RequireLen(t, devVersions, 1)
	testassert.Equal(t, "dev-main", devVersions[0].Version)
}

func TestPackagesJSONAdvertisesCapabilities(t *testing.T) {
	t.Run("full provider", func(t *testing.T) {
		root := fetchRoot(t, sampleProvider())
		testassert.Equal(t, "/p2/%package%.json", root.MetadataURL)
		testassert.Equal(t, []string{"acme/lib"}, root.AvailablePackages)
		testassert.NotEmpty(t, root.SearchURL)
		testassert.RequireNotNil(t, root.SecurityAdvisories)
		testassert.True(t, root.SecurityAdvisories.Metadata)
		testassert.Equal(t, "/security-advisories.json", root.SecurityAdvisories.APIURL)
	})

	t.Run("minimal provider", func(t *testing.T) {
		// bareProvider hides the optional capability interfaces.
		root := fetchRoot(t, bareProvider{sampleProvider()})
		testassert.Equal(t, "/p2/%package%.json", root.MetadataURL)
		testassert.Empty(t, root.AvailablePackages)
		testassert.Empty(t, root.SearchURL)
		testassert.Nil(t, root.SecurityAdvisories)
	})

	t.Run("api-only advisories", func(t *testing.T) {
		root := fetchRootOpts(t, sampleProvider(), repository.WithAdvisories(false, true))
		testassert.RequireNotNil(t, root.SecurityAdvisories)
		testassert.False(t, root.SecurityAdvisories.Metadata)
		testassert.Equal(t, "/security-advisories.json", root.SecurityAdvisories.APIURL)
	})

	t.Run("metadata-only advisories", func(t *testing.T) {
		root := fetchRootOpts(t, sampleProvider(), repository.WithAdvisories(true, false))
		testassert.RequireNotNil(t, root.SecurityAdvisories)
		testassert.True(t, root.SecurityAdvisories.Metadata)
		testassert.Empty(t, root.SecurityAdvisories.APIURL)
	})

	t.Run("advisories fully disabled", func(t *testing.T) {
		root := fetchRootOpts(t, sampleProvider(), repository.WithAdvisories(false, false))
		testassert.Nil(t, root.SecurityAdvisories)
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
	testassert.RequireNoError(t, err)
	testassert.RequireLen(t, results, 1)
	testassert.Equal(t, "acme/lib", results[0].Name)
	testassert.Equal(t, 42, results[0].Downloads)
}

func TestAdvisoriesEndpoint(t *testing.T) {
	c := newClient(t, sampleProvider())
	adv, err := c.GetSecurityAdvisories(context.Background(), []string{"acme/lib"})
	testassert.RequireNoError(t, err)
	testassert.RequireLen(t, adv["acme/lib"], 1)
	testassert.Equal(t, "CVE-1", adv["acme/lib"][0].CVE)
	testassert.Equal(t, "acme/lib", adv["acme/lib"][0].PackageName)
}

func TestMetadataEmbedsAdvisories(t *testing.T) {
	srv := httptest.NewServer(repository.NewHandler(sampleProvider()))
	t.Cleanup(srv.Close)

	// Stable metadata carries the advisory list.
	resp, err := http.Get(srv.URL + "/p2/acme/lib.json")
	testassert.RequireNoError(t, err)
	defer resp.Body.Close()
	testassert.RequireEqual(t, http.StatusOK, resp.StatusCode)

	var meta repository.Metadata
	testassert.RequireNoError(t, decodeJSON(resp, &meta))
	testassert.RequireLen(t, meta.SecurityAdvisories, 1)
	testassert.Equal(t, "PKSA-1", meta.SecurityAdvisories[0].AdvisoryID)
	testassert.Equal(t, "<1.0.1", meta.SecurityAdvisories[0].AffectedVersions)
	testassert.Equal(t, "acme/lib", meta.SecurityAdvisories[0].PackageName)

	// Dev metadata must not embed advisories.
	devResp, err := http.Get(srv.URL + "/p2/acme/lib~dev.json")
	testassert.RequireNoError(t, err)
	defer devResp.Body.Close()
	testassert.RequireEqual(t, http.StatusOK, devResp.StatusCode)
	var devMeta repository.Metadata
	testassert.RequireNoError(t, decodeJSON(devResp, &devMeta))
	testassert.Empty(t, devMeta.SecurityAdvisories)
}

func TestMetadataOnlyAdvisoriesClientRoundTrip(t *testing.T) {
	// Serve only via embedded metadata (no API endpoint) and let the client
	// fall back to the p2 file.
	c := newClient(t, sampleProvider(), repository.WithAdvisories(true, false))

	// Endpoint must not be wired.
	root := fetchRootOpts(t, sampleProvider(), repository.WithAdvisories(true, false))
	testassert.RequireNotNil(t, root.SecurityAdvisories)
	testassert.True(t, root.SecurityAdvisories.Metadata)
	testassert.Empty(t, root.SecurityAdvisories.APIURL)

	adv, err := c.GetSecurityAdvisories(context.Background(), []string{"acme/lib"})
	testassert.RequireNoError(t, err)
	testassert.RequireLen(t, adv["acme/lib"], 1)
	testassert.Equal(t, "PKSA-1", adv["acme/lib"][0].AdvisoryID)
	testassert.Equal(t, "CVE-1", adv["acme/lib"][0].CVE)
}

func TestWithAdvisoriesDisablesAPIEndpoint(t *testing.T) {
	srv := httptest.NewServer(repository.NewHandler(sampleProvider(), repository.WithAdvisories(true, false)))
	t.Cleanup(srv.Close)

	resp, err := http.PostForm(srv.URL+"/security-advisories.json", url.Values{"packages[]": {"acme/lib"}})
	testassert.RequireNoError(t, err)
	defer resp.Body.Close()
	testassert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestWithAdvisoriesDisablesMetadataEmbed(t *testing.T) {
	srv := httptest.NewServer(repository.NewHandler(sampleProvider(), repository.WithAdvisories(false, true)))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/p2/acme/lib.json")
	testassert.RequireNoError(t, err)
	defer resp.Body.Close()
	testassert.RequireEqual(t, http.StatusOK, resp.StatusCode)

	var meta repository.Metadata
	testassert.RequireNoError(t, decodeJSON(resp, &meta))
	testassert.Empty(t, meta.SecurityAdvisories)
}

func TestPackageNotFound(t *testing.T) {
	c := newClient(t, sampleProvider())
	_, err := c.GetPackage(context.Background(), "acme/unknown")
	testassert.ErrorIs(t, err, repository.ErrPackageNotFound)
}

func TestSearchEndpointNotWiredWithoutCapability(t *testing.T) {
	p := bareProvider{sampleProvider()}
	srv := httptest.NewServer(repository.NewHandler(p))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/search.json?q=acme")
	testassert.RequireNoError(t, err)
	defer resp.Body.Close()
	testassert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestAdvisoriesRequiresPost(t *testing.T) {
	srv := httptest.NewServer(repository.NewHandler(sampleProvider()))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/security-advisories.json")
	testassert.RequireNoError(t, err)
	defer resp.Body.Close()
	testassert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

func TestMalformedMetadataPath(t *testing.T) {
	srv := httptest.NewServer(repository.NewHandler(sampleProvider()))
	t.Cleanup(srv.Close)

	for _, path := range []string{"/p2/noslash.json", "/p2/a/b/c.json", "/p2/acme/lib.txt"} {
		resp, err := http.Get(srv.URL + path)
		testassert.RequireNoError(t, err)
		resp.Body.Close()
		testassert.Equal(t, http.StatusNotFound, resp.StatusCode, path)
	}
}

func TestAdvisoriesFormEncoding(t *testing.T) {
	srv := httptest.NewServer(repository.NewHandler(sampleProvider()))
	t.Cleanup(srv.Close)

	resp, err := http.PostForm(srv.URL+"/security-advisories.json", url.Values{"packages[]": {"acme/lib"}})
	testassert.RequireNoError(t, err)
	defer resp.Body.Close()
	testassert.RequireEqual(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Advisories map[string][]repository.SecurityAdvisory `json:"advisories"`
	}
	testassert.RequireNoError(t, decodeJSON(resp, &body))
	testassert.RequireLen(t, body.Advisories["acme/lib"], 1)
}

// --- test helpers ---

func decodeJSON(resp *http.Response, v any) error {
	return json.NewDecoder(resp.Body).Decode(v)
}

func fetchRoot(t *testing.T, p repository.Provider) repository.RootFile {
	t.Helper()
	return fetchRootOpts(t, p)
}

func fetchRootOpts(t *testing.T, p repository.Provider, opts ...repository.HandlerOption) repository.RootFile {
	t.Helper()
	srv := httptest.NewServer(repository.NewHandler(p, opts...))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/packages.json")
	testassert.RequireNoError(t, err)
	defer resp.Body.Close()
	testassert.RequireEqual(t, http.StatusOK, resp.StatusCode)

	var root repository.RootFile
	testassert.RequireNoError(t, decodeJSON(resp, &root))
	return root
}
