# go-packagist

[![Go Reference](https://pkg.go.dev/badge/github.com/shyim/go-packagist.svg)](https://pkg.go.dev/github.com/shyim/go-packagist)

A small, dependency-light Go library for reading and manipulating [Composer](https://getcomposer.org/) files and parsing PHP-style version constraints.

## Features

- Read, modify and write `composer.json` while preserving the formatting Composer expects.
- Read `composer.lock` to inspect locked packages.
- Read and write `auth.json` for registry credentials.
- Parse and evaluate PHP-style version constraints (e.g. `^1.2`, `>=2.0 <3.0`).

The library only depends on the Go standard library and [`github.com/shyim/go-version`](https://github.com/shyim/go-version).

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
	if err := composer.Save("composer.json"); err != nil {
		log.Fatal(err)
	}
}
```

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

### Parse PHP version constraints

```go
constraint, err := packagist.NewPHPConstraint("^1.2")
if err != nil {
	log.Fatal(err)
}

ok := constraint.Matches("1.4.0")
log.Printf("matches: %v", ok)
```

## Documentation

Full API documentation is available on [pkg.go.dev](https://pkg.go.dev/github.com/shyim/go-packagist).

## License

[MIT](./LICENSE)
