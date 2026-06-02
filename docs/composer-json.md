# Working with `composer.json`

[← Back to README](../README.md)

`composer.ReadJson` parses a `composer.json` into a `*composer.Json`. Modify it
and call `Save` to write it back; the formatting Composer expects (two-space
indent) and the original top-level key order are preserved, and any keys the
library does not model are kept verbatim across a read/modify/write round-trip.

```go
package main

import (
	"log"

	"github.com/shyim/go-composer"
)

func main() {
	c, err := composer.ReadJson("composer.json")
	if err != nil {
		log.Fatal(err)
	}

	log.Println(c.Name, c.Description)

	if err := c.Save(); err != nil {
		log.Fatal(err)
	}
}
```

## Dependencies, repositories and config

```go
// require / require-dev
c.AddPackage("monolog/monolog", "^3.0")
c.AddPackageDev("phpunit/phpunit", "^11.0")
c.RemovePackage("old/package")

if c.HasPackage("monolog/monolog") {
	// ...
}

// repositories
c.AddRepository(composer.Repository{
	Type: "composer",
	URL:  "https://repo.example.com",
})
c.RemoveRepository("https://repo.example.com")

// config section
c.SetConfig("sort-packages", true)
c.EnableComposerPlugin("phpstan/extension-installer")
c.RemoveConfig("secure-http")
```

`AddPackage` / `AddPackageDev` add or update a constraint; `AddRepository` is a
no-op if a repository with the same URL already exists.

## Unknown and future fields

Top-level keys the library does not model are captured in
`Json.AdditionalFields` on read and merged back on write, so settings added by
future Composer versions survive untouched. The modeled `extra` section is
exposed separately as `Json.Extra` (see below).

## The `extra` section

`extra` is exposed as `ExtraData` — a `map[string]any` with dotted-path
accessors for reading and manipulating nested values:

```go
// Read by dotted path (comma-ok; the second return reports presence/type match).
if class, ok := c.Extra.GetString("shopware.plugin-class"); ok {
	log.Println("plugin class:", class)
}
enabled, _ := c.Extra.GetBool("shopware.enabled")
priority, _ := c.Extra.GetInt("shopware.priority")
bundles, _ := c.Extra.GetStringSlice("shopware.bundles")

// Mutate nested values (intermediate objects are created as needed).
c.Extra.Set("shopware.plugin-icon", "icon.png")
c.Extra.Unset("shopware.deprecated")

// Plain map indexing still works, since ExtraData is a map underneath.
raw := c.Extra["shopware"]
_ = raw
```

Available accessors: `Get`, `Has`, `GetString`, `GetBool`, `GetInt`,
`GetFloat`, `GetMap`, `GetSlice`, `GetStringSlice`, `Set`, and `Unset`.
