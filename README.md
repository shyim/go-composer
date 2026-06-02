# go-composer

[![Go Reference](https://pkg.go.dev/badge/github.com/shyim/go-composer.svg)](https://pkg.go.dev/github.com/shyim/go-composer)

A small, zero-dependency Go library for the [Composer](https://getcomposer.org/)
ecosystem: read and manipulate Composer files, and query or host a Composer
repository.

## Features

- Read, modify and write `composer.json` while preserving the formatting
  Composer expects and any fields the library does not model.
- Read `composer.lock` to inspect locked packages.
- Read and write `auth.json` for registry credentials.
- Query `composer`-type repositories (packagist.org, Satis, Private Packagist)
  over Composer's V2 protocol — versions, requirements, dist/source, search and
  security advisories.
- Serve your own repository over the same protocol and wire types, in the style
  of `net/http`.

The library only depends on the Go standard library.

## Installation

```sh
go get github.com/shyim/go-composer
```

## Quick start

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

	c.AddPackage("monolog/monolog", "^3.0")

	if err := c.Save(); err != nil {
		log.Fatal(err)
	}
}
```

## Guides

The library covers several use-cases; each has a focused guide:

- [Working with `composer.json`](docs/composer-json.md) — read, modify and write;
  dependencies, repositories and config; the `extra` section.
- [Reading `composer.lock`](docs/composer-lock.md) — inspect locked packages.
- [Working with `auth.json`](docs/auth-json.md) — registry credentials and the
  supported authentication methods.
- [Querying repositories](docs/repository-client.md) — the client: a single
  repository, multi-repository sets, search, security advisories and auth.
- [Serving a repository](docs/repository-server.md) — the server: the `Provider`
  interface and the optional search/advisory capabilities.

## Documentation

Full API documentation is available on [pkg.go.dev](https://pkg.go.dev/github.com/shyim/go-composer).

## License

[MIT](./LICENSE)
