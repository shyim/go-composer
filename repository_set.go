package packagist

import (
	"context"
	"errors"
)

// PackagistURL is the canonical public Composer repository, queried by default
// unless a composer.json opts out of it.
const PackagistURL = "https://repo.packagist.org"

// RepositorySet aggregates multiple Composer repositories and queries them in
// declared order, mirroring Composer's "first repository that provides the
// package wins" resolution.
type RepositorySet struct {
	Repositories []*ComposerRepository
}

// NewRepositorySet builds a set from the given repositories, queried in order.
func NewRepositorySet(repos ...*ComposerRepository) *RepositorySet {
	return &RepositorySet{Repositories: repos}
}

// NewRepositorySetFromComposer builds a set from the "repositories" declared in
// a parsed composer.json. Only "composer" type repositories are included, in
// declaration order; repositories of other types (vcs, path, package, ...) are
// skipped as they are not queryable via the metadata protocol. The public
// packagist.org repository is appended last unless includePackagist is false or
// a repository with that URL is already configured. auth may be nil.
//
// Note: composer.json's `{"packagist.org": false}` opt-out cannot currently be
// represented by the repositories parser, so callers that honor it should pass
// includePackagist=false.
func NewRepositorySetFromComposer(c *ComposerJson, auth *ComposerAuth, includePackagist bool) *RepositorySet {
	set := &RepositorySet{}
	hasPackagist := false

	for _, repo := range c.Repositories {
		if repo.Type != "composer" || repo.URL == "" {
			continue
		}
		if isPackagistURL(repo.URL) {
			hasPackagist = true
		}
		set.Repositories = append(set.Repositories, NewComposerRepository(repo.URL, auth))
	}

	if includePackagist && !hasPackagist {
		set.Repositories = append(set.Repositories, NewComposerRepository(PackagistURL, auth))
	}

	return set
}

func isPackagistURL(url string) bool {
	for _, host := range []string{"repo.packagist.org", "packagist.org"} {
		if url == "https://"+host || url == "http://"+host ||
			url == "https://"+host+"/" || url == "http://"+host+"/" {
			return true
		}
	}
	return false
}

// GetPackage queries each repository in order and returns the first match,
// along with the repository that provided it. It returns ErrPackageNotFound
// only when no repository knows the package.
func (s *RepositorySet) GetPackage(ctx context.Context, name string) (*PackageMetadata, *ComposerRepository, error) {
	for _, repo := range s.Repositories {
		meta, err := repo.GetPackage(ctx, name)
		if err != nil {
			if errors.Is(err, ErrPackageNotFound) {
				continue
			}
			return nil, nil, err
		}
		return meta, repo, nil
	}
	return nil, nil, ErrPackageNotFound
}

// Search queries every repository and concatenates their results in repository
// order. Errors from individual repositories abort the search.
func (s *RepositorySet) Search(ctx context.Context, query string) ([]SearchResult, error) {
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
