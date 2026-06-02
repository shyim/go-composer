// Package packagist reads and manipulates Composer files.
//
// It provides helpers for working with composer.json, composer.lock, and
// auth.json: loading them from disk, modifying their contents, and writing
// them back while preserving the formatting Composer expects.
//
// The companion subpackage repository queries Composer "type": "composer"
// repositories (such as packagist.org or a private Satis/Packagist instance)
// over the V2 metadata protocol, returning a package's versions and their
// requirements, dist/source, and other metadata.
package packagist
