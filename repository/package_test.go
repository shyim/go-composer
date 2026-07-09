package repository

import (
	"context"
	"github.com/shyim/go-composer/internal/testassert"
	"net/http"
	"testing"
)

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
	testassert.RequireNoError(t, err)
	testassert.RequireEqual(t, "acme/lib", meta.Name)
	testassert.RequireLen(t, meta.Versions, 3) // 2 stable + 1 dev

	// Delta expansion: the second version inherits type from the first.
	v1 := meta.Version("1.0.0")
	testassert.RequireNotNil(t, v1)
	testassert.Equal(t, "library", v1.Type)
	testassert.Equal(t, map[string]string{"php": ">=7.4"}, v1.Require)

	v2 := meta.Version("2.0.0")
	testassert.RequireNotNil(t, v2)
	testassert.Equal(t, "zip", v2.Dist.Type)
	testassert.Equal(t, "https://ex/2.0.0.zip", v2.Dist.URL)

	dev := meta.Version("dev-main")
	testassert.RequireNotNil(t, dev)

	testassert.Contains(t, gotPaths, "/p2/acme/lib.json")
	testassert.Contains(t, gotPaths, "/p2/acme/lib~dev.json")
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
	testassert.ErrorIs(t, err, ErrPackageNotFound)
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
	testassert.ErrorIs(t, err, ErrPackageNotFound)
	testassert.False(t, fetchedMetadata, "should not hit metadata-url for a package outside available-packages")
}

func TestGetPackageInlinePartialPackages(t *testing.T) {
	repo, _ := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		testassert.RequireEqual(t, "/packages.json", r.URL.Path)
		_, _ = w.Write([]byte(`{
			"metadata-url":"/p2/%package%.json",
			"packages":{"acme/inline":[{"name":"acme/inline","version":"1.2.3","version_normalized":"1.2.3.0"}]}
		}`))
	}, nil)

	meta, err := repo.GetPackage(context.Background(), "acme/inline")
	testassert.RequireNoError(t, err)
	testassert.RequireLen(t, meta.Versions, 1)
	testassert.Equal(t, "1.2.3", meta.Versions[0].Version)
}

func TestVersionLookup(t *testing.T) {
	meta := &Package{Versions: []Version{
		{Version: "1.0.0", VersionNormalized: "1.0.0.0"},
		{Version: "2.0.0", VersionNormalized: "2.0.0.0"},
	}}
	testassert.Equal(t, "1.0.0", meta.Version("1.0.0").Version)
	testassert.Equal(t, "2.0.0", meta.Version("2.0.0.0").Version) // by normalized
	testassert.Nil(t, meta.Version("9.9.9"))
}
