package repository

import (
	"context"
	"errors"
	"strings"

	"github.com/shyim/go-composer"
)

// PackagistURL is the canonical public Composer repository, queried by default
// unless a composer.json opts out of it.
const PackagistURL = "https://repo.packagist.org"

// Set aggregates multiple Composer repositories and queries them in declared
// order, mirroring Composer's "first repository that provides the package wins"
// resolution.
type Set struct {
	Repositories []*Client
}

// NewSet builds a set from the given repositories, queried in order.
func NewSet(repos ...*Client) *Set {
	return &Set{Repositories: repos}
}

// Add appends the given repository clients to the set.
func (s *Set) Add(repos ...*Client) {
	for _, repo := range repos {
		if repo != nil {
			s.Repositories = append(s.Repositories, repo)
		}
	}
}

// AddRepository appends a repository client to the set unless a repository
// with the same URL is already registered.
func (s *Set) AddRepository(repo *Client) {
	if repo == nil || s.HasRepository(repo.URL()) {
		return
	}
	s.Repositories = append(s.Repositories, repo)
}

// HasRepository reports whether a repository with the given URL is registered
// in the set.
func (s *Set) HasRepository(rawURL string) bool {
	if rawURL == "" {
		return false
	}
	target := strings.TrimRight(rawURL, "/")
	targetIsPackagist := isPackagistURL(rawURL)

	for _, repo := range s.Repositories {
		if repo == nil {
			continue
		}
		repoURL := strings.TrimRight(repo.URL(), "/")
		if strings.EqualFold(repoURL, target) {
			return true
		}
		if targetIsPackagist && isPackagistURL(repo.URL()) {
			return true
		}
	}
	return false
}

// Has is an alias for HasRepository.
func (s *Set) Has(rawURL string) bool {
	return s.HasRepository(rawURL)
}

// URLs returns the canonical base URLs of all registered repositories in order.
func (s *Set) URLs() []string {
	urls := make([]string, 0, len(s.Repositories))
	for _, repo := range s.Repositories {
		if repo != nil {
			urls = append(urls, repo.URL())
		}
	}
	return urls
}

// FromComposer builds a set from the "repositories" declared in a parsed
// composer.json. Only "composer" type repositories are included, in declaration
// order; repositories of other types (vcs, path, package, ...) are skipped as
// they are not queryable via the metadata protocol. The public packagist.org
// repository is appended last unless includePackagist is false or a repository
// with that URL is already configured. auth may be nil.
//
// Note: composer.json's `{"packagist.org": false}` opt-out cannot currently be
// represented by the repositories parser, so callers that honor it should pass
// includePackagist=false.
func FromComposer(c *composer.Json, auth *composer.Auth, includePackagist bool) *Set {
	set := &Set{}
	hasPackagist := false

	for _, repo := range c.Repositories {
		if repo.Type != "composer" || repo.URL == "" {
			continue
		}
		if isPackagistURL(repo.URL) {
			hasPackagist = true
		}
		set.Repositories = append(set.Repositories, New(repo.URL, auth))
	}

	if includePackagist && !hasPackagist {
		set.Repositories = append(set.Repositories, New(PackagistURL, auth))
	}

	return set
}

func isPackagistURL(rawURL string) bool {
	for _, host := range []string{"repo.packagist.org", "packagist.org"} {
		if rawURL == "https://"+host || rawURL == "http://"+host ||
			rawURL == "https://"+host+"/" || rawURL == "http://"+host+"/" {
			return true
		}
	}
	return false
}

// GetPackage queries each repository in order and returns the first match,
// along with the repository that provided it. It returns ErrPackageNotFound
// only when no repository knows the package.
func (s *Set) GetPackage(ctx context.Context, name string) (*Package, *Client, error) {
	for _, repo := range s.Repositories {
		pkg, err := repo.GetPackage(ctx, name)
		if err != nil {
			if errors.Is(err, ErrPackageNotFound) {
				continue
			}
			return nil, nil, err
		}
		return pkg, repo, nil
	}
	return nil, nil, ErrPackageNotFound
}

// Search queries every repository and concatenates their results in repository
// order. Errors from individual repositories abort the search.
func (s *Set) Search(ctx context.Context, query string) ([]SearchResult, error) {
	var results []SearchResult
	for _, repo := range s.Repositories {
		res, err := repo.Search(ctx, query)
		if err != nil {
			return nil, err
		}
		results = append(results, res...)
	}
	return results, nil
}

// GetSecurityAdvisories queries every repository and merges their advisories
// by package name into an Advisories value. When several repositories report
// advisories for the same package, the lists are concatenated in repository
// order (duplicates are not deduplicated — callers that need uniqueness can
// key on AdvisoryID). Errors from individual repositories abort the query.
// Repositories without advisory support contribute nothing.
func (s *Set) GetSecurityAdvisories(ctx context.Context, packages []string) (Advisories, error) {
	result := Advisories{}
	if len(packages) == 0 {
		return result, nil
	}
	for _, repo := range s.Repositories {
		adv, err := repo.GetSecurityAdvisories(ctx, packages)
		if err != nil {
			return nil, err
		}
		for name, list := range adv {
			if len(list) == 0 {
				continue
			}
			result[name] = append(result[name], list...)
		}
	}
	return result, nil
}
