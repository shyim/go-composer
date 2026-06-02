# Reading `composer.lock`

[← Back to README](../README.md)

`composer.ReadLock` parses a `composer.lock` into a `*composer.Lock`, exposing
the locked packages so you can inspect exactly what is installed.

```go
package main

import (
	"log"

	"github.com/shyim/go-composer"
)

func main() {
	lock, err := composer.ReadLock("composer.lock")
	if err != nil {
		log.Fatal(err)
	}

	for _, pkg := range lock.Packages {
		log.Printf("%s %s", pkg.Name, pkg.Version)
	}

	// require-dev packages are listed separately.
	for _, pkg := range lock.PackagesDev {
		log.Printf("[dev] %s %s", pkg.Name, pkg.Version)
	}
}
```

## Looking up a single package

```go
if pkg := lock.GetPackage("monolog/monolog"); pkg != nil {
	log.Println(pkg.Version)        // e.g. "3.10.0"
	log.Println(pkg.Require)        // map[string]string of its dependencies
	log.Println(pkg.Dist.URL)       // download URL
	log.Println(pkg.Source.Reference)
}
```

`GetPackage` searches `Packages` (not `PackagesDev`) and returns `nil` when the
package is not locked.

Each `LockPackage` carries `Name`, `Version`, `Type`, `Require`, `License`,
`Description`, `Homepage`, `Time`, and the `Dist` / `Source` references
(`LockPackageDist` / `LockPackageSource`).
