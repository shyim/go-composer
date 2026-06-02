// Package repository queries Composer "type": "composer" repositories (such as
// packagist.org or a private Satis/Packagist instance) over Composer's V2
// metadata protocol.
//
// A Client fetches a repository's packages.json root, resolves its metadata-url
// (substituting %package% and reading the stable and ~dev files), expands the
// minified "composer/2.0" delta format, and returns a package's versions with
// their requirements, dist/source and other metadata. It also exposes the
// search and security-advisory endpoints. A Set queries several repositories in
// order with first-match-wins resolution and can be built from a parsed
// composer.json. Credentials from an auth.json are applied per request origin.
package repository
