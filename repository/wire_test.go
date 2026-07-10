package repository

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/shyim/go-composer/internal/testassert"
)

func TestRootFilePackagesEmptyArray(t *testing.T) {
	// packagist.org ships "packages": [] as a V1 stub next to metadata-url.
	var root RootFile
	err := json.Unmarshal([]byte(`{
		"packages": [],
		"metadata-url": "/p2/%package%.json",
		"search": "/search.json?q=%query%"
	}`), &root)
	testassert.RequireNoError(t, err)
	testassert.Nil(t, root.Packages)
	testassert.Equal(t, "/p2/%package%.json", root.MetadataURL)
}

func TestRootFilePackagesObject(t *testing.T) {
	var root RootFile
	err := json.Unmarshal([]byte(`{
		"packages": {
			"acme/lib": [{"name":"acme/lib","version":"1.0.0"}]
		}
	}`), &root)
	testassert.RequireNoError(t, err)
	testassert.RequireNotNil(t, root.Packages)
	testassert.RequireLen(t, root.Packages, 1)
	_, ok := root.Packages["acme/lib"]
	testassert.True(t, ok)
}

func TestRootFilePackagesEmptyObject(t *testing.T) {
	var root RootFile
	err := json.Unmarshal([]byte(`{"packages":{}}`), &root)
	testassert.RequireNoError(t, err)
	testassert.RequireNotNil(t, root.Packages)
	testassert.Empty(t, root.Packages)
}

func TestRootFilePackagesNonEmptyArrayRejected(t *testing.T) {
	var root RootFile
	err := json.Unmarshal([]byte(`{"packages":[{"name":"x"}]}`), &root)
	testassert.Error(t, err)
}

func TestGetPackageAgainstPackagistStyleRoot(t *testing.T) {
	repo, _ := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/packages.json":
			// Real packagist.org shape: empty packages array + metadata-url.
			_, _ = w.Write([]byte(`{"packages":[],"metadata-url":"/p2/%package%.json"}`))
		case "/p2/monolog/monolog.json":
			_, _ = w.Write([]byte(`{"packages":{"monolog/monolog":[{"name":"monolog/monolog","version":"3.0.0","version_normalized":"3.0.0.0"}]}}`))
		case "/p2/monolog/monolog~dev.json":
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}, nil)

	pkg, err := repo.GetPackage(context.Background(), "monolog/monolog")
	testassert.RequireNoError(t, err)
	testassert.Equal(t, "monolog/monolog", pkg.Name)
	testassert.RequireLen(t, pkg.Versions, 1)
	testassert.Equal(t, "3.0.0", pkg.Versions[0].Version)

	_, err = repo.GetPackages(context.Background())
	testassert.ErrorIs(t, err, ErrListingNotSupported)
}
