// Package packagist reads and manipulates Composer files and parses PHP
// version constraints.
//
// It provides helpers for working with composer.json, composer.lock, and
// auth.json: loading them from disk, modifying their contents, and writing
// them back while preserving the formatting Composer expects. In addition,
// it parses and evaluates PHP-style version constraints so that package
// requirements can be inspected and compared programmatically.
package packagist
