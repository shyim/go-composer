# Generating a CycloneDX SBOM

[← Back to README](../README.md)

The `sbom` package is its own Go module (`github.com/shyim/go-composer/sbom`)
and turns a parsed `composer.lock` into a [CycloneDX](https://cyclonedx.org/)
1.7 JSON Software Bill of Materials. Its only dependency is
`github.com/shyim/go-composer` (stdlib lock types); install with:

```sh
go get github.com/shyim/go-composer/sbom@sbom/vX.Y.Z
```

In this monorepo, a root `go.work` lists both modules so the local parent is
used automatically while developing. Consumers do not need `go.work` — after
the first root release they resolve a real versioned require (for example
`github.com/shyim/go-composer v0.1.0`). Nested releases use directory-prefixed
tags: `sbom/v0.1.0` for this module, `v0.1.0` for the root.

## Quick start

```go
package main

import (
	"log"
	"os"

	"github.com/shyim/go-composer"
	"github.com/shyim/go-composer/sbom"
)

func main() {
	lock, err := composer.ReadLock("composer.lock")
	if err != nil {
		log.Fatal(err)
	}
	c, _ := composer.ReadJson("composer.json") // optional, for root name/version

	bom, err := sbom.Generate(lock, sbom.Options{
		ApplicationName:        c.Name,
		ApplicationVersion:     c.Version,
		ToolName:               "my-cli",
		ToolGroup:              "acme",
		ToolVersion:            "1.2.3",
		IncludeDevDependencies: false,
	})
	if err != nil {
		log.Fatal(err)
	}

	data, err := sbom.Marshal(bom)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("sbom.cdx.json", data, 0o644); err != nil {
		log.Fatal(err)
	}
}
```

## What ends up in the BOM

| CycloneDX field | Source |
|---|---|
| `metadata.component` | root application (`Options.ApplicationName` / `ApplicationVersion`) |
| `metadata.tools.components` | optional tool identity (`ToolName` / `ToolGroup` / `ToolVersion`) |
| `components[]` | each locked composer package (purl `pkg:composer/…`) |
| `components[].hashes` | `dist.shasum` as `SHA-1` when present |
| `components[].externalReferences` | homepage (`website`), source URL (`vcs`), dist URL (`distribution`) |
| `components[].licenses` | lock `license` values (see [Licenses](#licenses)) |
| `dependencies[]` | root → every package; each package → its non-platform `require`s |

Platform requirements (`php`, `ext-*`, `lib-*`, `composer-*`, `hhvm`) are
skipped from dependency edges — they are not Composer packages.

`packages-dev` is ignored unless `IncludeDevDependencies: true`.

## Licenses

Composer license strings are either SPDX identifiers (`MIT`) or free text
(`proprietary`). CycloneDX requires the former as `license.id` and the latter
as `license.name`.

By default every license is emitted as `license.name`, which is always valid
CycloneDX. To emit recognized SPDX identifiers as `license.id` instead, set
`Options.SPDX` — for example with
[`github.com/shyim/go-spdx`](https://github.com/shyim/go-spdx):

```go
import "github.com/shyim/go-spdx"

s, err := spdx.NewSpdxLicenses()
if err != nil {
	log.Fatal(err)
}

bom, err := sbom.Generate(lock, sbom.Options{
	ApplicationName: "acme/app",
	SPDX: func(license string) bool {
		ok, _ := s.Validate(license)
		return ok
	},
})
```

`SPDX` is optional so this package stays zero-dependency; SPDX databases are
large and not every consumer needs them.

## Options reference

| Field | Default | Purpose |
|---|---|---|
| `ApplicationName` | `"application"` | Root metadata component name |
| `ApplicationVersion` | empty | Root component version |
| `ToolName` | empty | When set, adds a `tools.components` entry |
| `ToolGroup` | empty | Group for the tool component |
| `ToolVersion` | empty | Version for the tool component |
| `ToolType` | `"application"` | CycloneDX type for the tool |
| `IncludeDevDependencies` | `false` | Also include `packages-dev` |
| `SPDX` | `nil` | Predicate: true → `license.id`, false → `license.name` |
