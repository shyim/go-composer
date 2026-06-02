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

## Search and security advisories

```go
results, _ := set.Search(ctx, "logger") // aggregates across the set
advisories, _ := repo.GetSecurityAdvisories(ctx, []string{"monolog/monolog"})
```

Both are no-ops (empty result, no error) against repositories that do not
advertise the corresponding endpoint.

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
