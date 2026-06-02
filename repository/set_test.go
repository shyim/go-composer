package repository

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	packagist "github.com/shyim/go-packagist"
)

func TestSetFirstMatchWins(t *testing.T) {
	// First repo knows nothing; second repo provides the package.
	private, _ := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/packages.json" {
			_, _ = w.Write([]byte(`{"metadata-url":"/p2/%package%.json","available-packages":[]}`))
			return
		}
		http.NotFound(w, r)
	}, nil)

	primary, _ := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/packages.json":
			_, _ = w.Write([]byte(`{"metadata-url":"/p2/%package%.json"}`))
		case "/p2/acme/lib.json":
			_, _ = w.Write([]byte(`{"packages":{"acme/lib":[{"name":"acme/lib","version":"1.0.0","version_normalized":"1.0.0.0"}]}}`))
		default:
			http.NotFound(w, r)
		}
	}, nil)

	set := NewSet(private, primary)

	meta, repo, err := set.GetPackage(context.Background(), "acme/lib")
	require.NoError(t, err)
	assert.Equal(t, "acme/lib", meta.Name)
	assert.Same(t, primary, repo)
}

func TestSetNotFound(t *testing.T) {
	repo, _ := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/packages.json" {
			_, _ = w.Write([]byte(`{"metadata-url":"/p2/%package%.json"}`))
			return
		}
		http.NotFound(w, r)
	}, nil)

	set := NewSet(repo)
	_, _, err := set.GetPackage(context.Background(), "no/thing")
	assert.ErrorIs(t, err, ErrPackageNotFound)
}

func TestFromComposer(t *testing.T) {
	c := &packagist.ComposerJson{
		Repositories: packagist.ComposerJsonRepositories{
			{Type: "composer", URL: "https://repo.example.com"},
			{Type: "vcs", URL: "https://github.com/acme/lib"}, // skipped
			{Type: "path", URL: "../local"},                   // skipped
		},
	}

	set := FromComposer(c, nil, true)
	require.Len(t, set.Repositories, 2)
	assert.Equal(t, "https://repo.example.com", set.Repositories[0].URL())
	assert.Equal(t, PackagistURL, set.Repositories[1].URL())
}

func TestFromComposerNoPackagist(t *testing.T) {
	c := &packagist.ComposerJson{
		Repositories: packagist.ComposerJsonRepositories{
			{Type: "composer", URL: "https://repo.example.com"},
		},
	}

	set := FromComposer(c, nil, false)
	require.Len(t, set.Repositories, 1)
	assert.Equal(t, "https://repo.example.com", set.Repositories[0].URL())
}

func TestFromComposerDeduplicatesPackagist(t *testing.T) {
	c := &packagist.ComposerJson{
		Repositories: packagist.ComposerJsonRepositories{
			{Type: "composer", URL: "https://repo.packagist.org"},
		},
	}

	set := FromComposer(c, nil, true)
	require.Len(t, set.Repositories, 1, "packagist should not be appended twice")
}

func TestSetSearchAggregates(t *testing.T) {
	mk := func(name string) *Client {
		repo, _ := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/packages.json":
				_, _ = w.Write([]byte(`{"metadata-url":"/p2/%package%.json","search":"/s.json?q=%query%"}`))
			case "/s.json":
				_, _ = w.Write([]byte(`{"results":[{"name":"` + name + `"}]}`))
			default:
				http.NotFound(w, r)
			}
		}, nil)
		return repo
	}

	set := NewSet(mk("a/one"), mk("b/two"))
	results, err := set.Search(context.Background(), "x")
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "a/one", results[0].Name)
	assert.Equal(t, "b/two", results[1].Name)
}
