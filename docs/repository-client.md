# Querying repositories (client)

[← Back to README](../README.md)

The `repository` package (`github.com/shyim/go-composer/repository`) speaks
Composer's V2 metadata protocol. A `Client` queries a single repository; a `Set`
queries several in order. It works against packagist.org and any private
Satis / Private Packagist / Artifactory instance.

## A single repository

```go
import (
	"context"

	"github.com/shyim/go-composer/repository"
)

ctx := context.Background()
repo := repository.New("https://repo.packagist.org", nil) // nil = no auth

pkg, err := repo.GetPackage(ctx, "monolog/monolog")
if err != nil {
	// errors.Is(err, repository.ErrPackageNotFound) when the repo lacks it
	log.Fatal(err)
}

for _, v := range pkg.Versions {
	log.Printf("%s requires %v", v.Version, v.Require)
}
```

`GetPackage` returns every known version (stable and `dev`). Each `Version`
mirrors that release's `composer.json` plus `version_normalized`, `dist` and
`source`.

### Inline package catalogs

Some repositories (Satis full dumps, Shopware's `packages.shopware.com`, …)
ship their **entire** catalog inside the root `packages.json` under
`"packages"`, keyed by package name. Values may be either:

- an **array** of version objects (Composer V2 partial-packages style), or
- a **map** version → package object (Composer V1 packages.json style).

Both forms are decoded transparently. When a package is present inline,
`GetPackage` serves it from the root file and does not hit `metadata-url`.

To list everything at once (Shopware-store-style "give me all packages I can
buy"):

```go
// Bearer auth for packages.shopware.com (from auth.json or hand-built).
auth := &composer.Auth{BearerAuth: map[string]string{
    "packages.shopware.com": token,
}}
repo := repository.New("https://packages.shopware.com", auth)

all, err := repo.GetPackages(ctx) // map[name]*Package for every inline entry
if err != nil {
    log.Fatal(err)
}
for name, pkg := range all {
    log.Printf("%s has %d versions", name, len(pkg.Versions))
}

// Or just the names:
names, _ := repo.PackageNames(ctx)
```

`PackageNames` prefers `available-packages` when advertised; otherwise it
returns the keys of the inline `packages` map. Lazy repositories with only a
`metadata-url` (no `available-packages`, no inline `packages`) return
`repository.ErrListingNotSupported` from both `PackageNames` and `GetPackages`
— ask for packages by name via `GetPackage` instead. An explicit empty
`"packages": {}` is still listable and yields an empty result without error.

### Picking a version

```go
if v := pkg.Version("3.10.0"); v != nil { // matches version or version_normalized
	log.Println(v.Dist.URL)
}
```

`Version` does an exact match — **no constraint solving is performed**. Resolving
a constraint like `^3.0` to a concrete version is left to the caller, so the
library stays dependency-free.

## Multiple repositories

A `Set` queries repositories in order and returns the first match, mirroring
Composer's "first repository that provides the package wins" rule.

```go
c, _ := composer.ReadJson("composer.json")
auth, _ := composer.ReadAuth("auth.json")

// Build from a composer.json's "repositories"; packagist.org is appended last
// unless includePackagist is false or already configured.
set := repository.FromComposer(c, auth, true)

pkg, source, err := set.GetPackage(ctx, "acme/private-package")
if err != nil {
	log.Fatal(err)
}
log.Printf("found %s in %s", pkg.Name, source.URL())
```

You can also assemble one explicitly:

```go
set := repository.NewSet(
	repository.New("https://repo.example.com", auth),
	repository.New(repository.PackagistURL, nil),
)
```

> Note: composer.json's `{"packagist.org": false}` opt-out cannot currently be
> represented by the repositories parser. Pass `includePackagist=false` to
> `FromComposer` if you need to honor it.

You can inspect registered repositories or add additional ones dynamically:

```go
// Check if a repository URL is already registered
if !set.HasRepository("https://repo.example.com") {
    // Add only if missing
    set.AddRepository(repository.New("https://repo.example.com", auth))
}

// Or append repository clients unconditionally
set.Add(repository.New("https://other.example.com", auth))

// List all registered repository base URLs
urls := set.URLs()
```

## Search and security advisories

```go
results, _ := set.Search(ctx, "logger") // aggregates across the set

adv, _ := repo.GetSecurityAdvisories(ctx, []string{"monolog/monolog"})
// Set also aggregates: set.GetSecurityAdvisories(ctx, names)
```

Both are no-ops (empty result, no error) against repositories that do not
advertise the corresponding endpoint.

`GetSecurityAdvisories` returns an [`Advisories`](https://pkg.go.dev/github.com/shyim/go-composer/repository#Advisories)
value — the known advisories keyed by package name, unfiltered by version.
Filter on that result for a concrete install (Composer `audit`-style). Version
matching is dependency-free: pass a `ConstraintCheck` implemented with whatever
semver engine you use, e.g. [`github.com/shyim/go-version`](https://github.com/shyim/go-version):

```go
import "github.com/shyim/go-version"

check := func(constraint, ver string) bool {
	v, err := version.NewVersion(strings.TrimPrefix(ver, "v"))
	if err != nil {
		return false
	}
	cs, err := version.NewConstraint(constraint)
	if err != nil {
		return false
	}
	return cs.Check(v)
}

// All known advisories for a package:
for _, a := range adv.Package("monolog/monolog") {
	log.Println(a.CVE, a.Title)
}

// Advisories that affect one installed version:
for _, a := range adv.AffectingPackage("monolog/monolog", "3.0.0", check) {
	log.Printf("vulnerable: %s (%s)", a.Title, a.CVE)
}

// Or filter a whole lockfile-style map at once:
affected := adv.Affecting(map[string]string{
	"monolog/monolog": "3.0.0",
	"psr/log":         "3.0.0",
}, check)
```

Packagist-style `affectedVersions` strings use `|` (or `||`) between OR branches
and `,` for AND inside a branch (e.g. `>=6.7.0.0,<6.7.8.1`). `SecurityAdvisory.Affects`
splits those branches and feeds each one to your `ConstraintCheck`.

## Authentication

Pass a `*composer.Auth` (see [auth.json](./auth-json.md)) to `New` or
`FromComposer`. The client attaches the credentials whose host matches the
request origin, following Composer's conventions:

| `auth.json` method | Header sent |
|---|---|
| `http-basic` | `Authorization: Basic <base64 user:pass>` |
| `bearer` | `Authorization: Bearer <token>` |
| `github-oauth` | `Authorization: token <token>` |
| `gitlab-oauth` | `Authorization: Bearer <token>` |
| `gitlab-token` | `PRIVATE-TOKEN: <token>` |
| `custom-headers` | appended verbatim |

## Custom HTTP client

Set `Client.HTTPClient` to control timeouts, transport, proxies, etc. When
unset, a client with a 30s timeout is used.

```go
repo := repository.New("https://repo.example.com", auth)
repo.HTTPClient = &http.Client{Timeout: 10 * time.Second}
```

To host your own repository, see [Serving a repository](./repository-server.md).
