package repository

import (
	"context"
	"github.com/shyim/go-composer/internal/testassert"
	"net/http"
	"testing"
)

func TestSearch(t *testing.T) {
	repo, _ := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/packages.json":
			_, _ = w.Write([]byte(`{"metadata-url":"/p2/%package%.json","search":"/search.json?q=%query%&type=%type%"}`))
		case "/search.json":
			testassert.Equal(t, "monolog", r.URL.Query().Get("q"))
			_, _ = w.Write([]byte(`{"results":[
				{"name":"monolog/monolog","description":"logging","downloads":1000},
				{"name":"virtual/thing","virtual":true}
			]}`))
		default:
			http.NotFound(w, r)
		}
	}, nil)

	results, err := repo.Search(context.Background(), "monolog")
	testassert.RequireNoError(t, err)
	testassert.RequireLen(t, results, 1) // virtual filtered out
	testassert.Equal(t, "monolog/monolog", results[0].Name)
	testassert.Equal(t, 1000, results[0].Downloads)
}
