package repository

import (
	"context"
	"github.com/shyim/go-composer"
	"github.com/shyim/go-composer/internal/testassert"
	"net/http"
	"testing"
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
	testassert.RequireNoError(t, err)
	testassert.Equal(t, "acme/lib", meta.Name)
	testassert.Same(t, primary, repo)
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
	testassert.ErrorIs(t, err, ErrPackageNotFound)
}

func TestFromComposer(t *testing.T) {
	c := &composer.Json{
		Repositories: composer.Repositories{
			{Type: "composer", URL: "https://repo.example.com"},
			{Type: "vcs", URL: "https://github.com/acme/lib"}, // skipped
			{Type: "path", URL: "../local"},                   // skipped
		},
	}

	set := FromComposer(c, nil, true)
	testassert.RequireLen(t, set.Repositories, 2)
	testassert.Equal(t, "https://repo.example.com", set.Repositories[0].URL())
	testassert.Equal(t, PackagistURL, set.Repositories[1].URL())
}

func TestFromComposerNoPackagist(t *testing.T) {
	c := &composer.Json{
		Repositories: composer.Repositories{
			{Type: "composer", URL: "https://repo.example.com"},
		},
	}

	set := FromComposer(c, nil, false)
	testassert.RequireLen(t, set.Repositories, 1)
	testassert.Equal(t, "https://repo.example.com", set.Repositories[0].URL())
}

func TestFromComposerDeduplicatesPackagist(t *testing.T) {
	c := &composer.Json{
		Repositories: composer.Repositories{
			{Type: "composer", URL: "https://repo.packagist.org"},
		},
	}

	set := FromComposer(c, nil, true)
	testassert.RequireLen(t, set.Repositories, 1, "packagist should not be appended twice")
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
	testassert.RequireNoError(t, err)
	testassert.RequireLen(t, results, 2)
	testassert.Equal(t, "a/one", results[0].Name)
	testassert.Equal(t, "b/two", results[1].Name)
}

func TestSetHasRepositoryAndURLs(t *testing.T) {
	set := NewSet(
		New("https://repo.example.com", nil),
		New("https://repo.packagist.org", nil),
	)

	testassert.True(t, set.HasRepository("https://repo.example.com"))
	testassert.True(t, set.HasRepository("https://repo.example.com/"))
	testassert.True(t, set.Has("https://packagist.org"))
	testassert.True(t, set.Has("https://repo.packagist.org/"))
	testassert.False(t, set.HasRepository("https://unknown.example.com"))

	urls := set.URLs()
	testassert.RequireLen(t, urls, 2)
	testassert.Equal(t, "https://repo.example.com", urls[0])
	testassert.Equal(t, "https://repo.packagist.org", urls[1])
}

func TestSetAddAndAddRepository(t *testing.T) {
	set := NewSet(New("https://repo.example.com", nil))

	// AddRepository does not add duplicate
	set.AddRepository(New("https://repo.example.com/", nil))
	testassert.RequireLen(t, set.Repositories, 1)

	// AddRepository adds new repo if missing
	set.AddRepository(New("https://new.example.com", nil))
	testassert.RequireLen(t, set.Repositories, 2)
	testassert.True(t, set.HasRepository("https://new.example.com"))

	// Add appends unconditionally
	set.Add(New("https://forced.example.com", nil))
	testassert.RequireLen(t, set.Repositories, 3)
	testassert.True(t, set.HasRepository("https://forced.example.com"))
}

