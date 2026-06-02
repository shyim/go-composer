package repository

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestVersionLookup(t *testing.T) {
	meta := &Package{Versions: []Version{
		{Version: "1.0.0", VersionNormalized: "1.0.0.0"},
		{Version: "2.0.0", VersionNormalized: "2.0.0.0"},
	}}
	assert.Equal(t, "1.0.0", meta.Version("1.0.0").Version)
	assert.Equal(t, "2.0.0", meta.Version("2.0.0.0").Version) // by normalized
	assert.Nil(t, meta.Version("9.9.9"))
}
