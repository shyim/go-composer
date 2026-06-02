# go-packagist

[![Go Reference](https://pkg.go.dev/badge/github.com/shyim/go-packagist.svg)](https://pkg.go.dev/github.com/shyim/go-packagist)

A small, zero-dependency Go library for reading and manipulating [Composer](https://getcomposer.org/) files.

## Features

- Read, modify and write `composer.json` while preserving the formatting Composer expects.
- Read `composer.lock` to inspect locked packages.
- Read and write `auth.json` for registry credentials.

The library only depends on the Go standard library.

## Installation

```sh
go get github.com/shyim/go-packagist
```

## Usage

### Read and modify `composer.json`

```go
package main

import (
	"log"

	"github.com/shyim/go-packagist"
)

func main() {
	composer, err := packagist.ReadComposerJson("composer.json")
	if err != nil {
		log.Fatal(err)
	}

	// Inspect or modify the parsed document, then write it back.
	if err := composer.Save(); err != nil {
		log.Fatal(err)
	}
}
```

### Work with the `extra` section

The `extra` section is exposed as `ExtraData` — a `map[string]any` with
dotted-path accessors for reading and manipulating nested values:

```go
composer, _ := packagist.ReadComposerJson("composer.json")

// Read by dotted path (comma-ok; second return reports presence/type match).
if class, ok := composer.Extra.GetString("shopware.plugin-class"); ok {
	log.Println("plugin class:", class)
}
enabled, _ := composer.Extra.GetBool("shopware.enabled")
priority, _ := composer.Extra.GetInt("shopware.priority")
bundles, _ := composer.Extra.GetStringSlice("shopware.bundles")
_ = enabled
_ = priority
_ = bundles

// Mutate nested values (intermediate objects are created as needed).
composer.Extra.Set("shopware.plugin-icon", "icon.png")
composer.Extra.Unset("shopware.deprecated")

// Plain map indexing still works, since ExtraData is a map underneath.
raw := composer.Extra["shopware"]
_ = raw
```

Available accessors: `Get`, `Has`, `GetString`, `GetBool`, `GetInt`,
`GetFloat`, `GetMap`, `GetSlice`, `GetStringSlice`, `Set`, and `Unset`.

### Read `composer.lock`

```go
lock, err := packagist.ReadComposerLock("composer.lock")
if err != nil {
	log.Fatal(err)
}

for _, pkg := range lock.Packages {
	log.Printf("%s %s", pkg.Name, pkg.Version)
}
```

### Read `auth.json`

```go
auth, err := packagist.ReadComposerAuth("auth.json")
if err != nil {
	log.Fatal(err)
}

_ = auth
```

## Documentation

Full API documentation is available on [pkg.go.dev](https://pkg.go.dev/github.com/shyim/go-packagist).

## License

[MIT](./LICENSE)
