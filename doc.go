// Package packagist reads and manipulates Composer files.
//
// It provides helpers for working with composer.json, composer.lock, and
// auth.json: loading them from disk, modifying their contents, and writing
// them back while preserving the formatting Composer expects.
//
// It can also query Composer "type": "composer" repositories (such as
// packagist.org or a private Satis/Packagist instance) over the V2 metadata
// protocol via ComposerRepository, returning a package's versions and their
// requirements, dist/source, and other metadata. RepositorySet queries several
// repositories in order — including those declared in a composer.json — with
// first-match-wins resolution, and credentials from an auth.json are applied
// per request origin.
package packagist
