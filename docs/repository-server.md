# Serving a repository (server)

[← Back to README](../README.md)

The `repository` package is also the server — like `net/http`, client and server
live together and share the wire types. `repository.NewHandler` turns a data
source into a Composer V2 repository over HTTP. Anything it serves can be read
back by the `Client` (see [Querying repositories](./repository-client.md)).

## The Provider interface

Implement one required method:

```go
import (
	"context"

	"github.com/shyim/go-composer/repository"
)

type myProvider struct{ /* DB, filesystem, object store, ... */ }

func (p *myProvider) Package(ctx context.Context, name string) (*repository.Package, error) {
	// Return repository.ErrPackageNotFound (matchable with errors.Is) when unknown.
	return &repository.Package{
		Name: name,
		Versions: []repository.Version{
			{Name: name, Version: "1.0.0", VersionNormalized: "1.0.0.0", Type: "library"},
			{Name: name, Version: "dev-main", VersionNormalized: "dev-main", Type: "library"},
		},
	}, nil
}

func main() {
	http.Handle("/", repository.NewHandler(&myProvider{}))
	http.ListenAndServe(":8080", nil)
}
```

This already serves:

- `GET /packages.json` — the root file (advertises `metadata-url`).
- `GET /p2/<vendor>/<pkg>.json` — the package's **stable** versions.
- `GET /p2/<vendor>/<pkg>~dev.json` — the package's **development** versions.

Metadata is always delta-encoded with the `composer/2.0` format, as
packagist.org serves it.

## Optional capabilities

Implement any of these extra interfaces and the handler advertises and serves
the matching endpoint automatically — implement none and it stays a minimal
metadata repository.

| Interface | Method | Effect |
|---|---|---|
| `PackageLister` | `PackageNames(ctx) ([]string, error)` | adds `available-packages` to packages.json (marks the repo finite) |
| `Searcher` | `Search(ctx, query) ([]repository.SearchResult, error)` | serves `GET /search.json?q=` |
| `Advisor` | `SecurityAdvisories(ctx, names) (map[string][]repository.SecurityAdvisory, error)` | serves `POST /security-advisories.json` **and** embeds advisories in stable `/p2/...json` metadata |

```go
func (p *myProvider) PackageNames(ctx context.Context) ([]string, error) {
	return []string{"acme/lib", "acme/tool"}, nil
}

func (p *myProvider) SecurityAdvisories(ctx context.Context, names []string) (map[string][]repository.SecurityAdvisory, error) {
	out := map[string][]repository.SecurityAdvisory{}
	for _, name := range names {
		if list := p.lookupAdvisories(name); len(list) > 0 {
			out[name] = list
		}
	}
	return out, nil
}
```

### Security advisories delivery

When a provider implements `Advisor`, the handler by default publishes advisories
over **both** channels Composer supports (packagist.org does the same):

1. **API** — `security-advisories.api-url` → `POST /security-advisories.json`
   with form field `packages[]`. Preferred by clients for full advisories
   (title, CVE, sources, severity).
2. **Metadata** — `security-advisories.metadata=true`, and each stable
   `/p2/<vendor>/<pkg>.json` document gains a top-level `security-advisories`
   **list** (not a map) next to `packages`. Development (`~dev.json`) files
   never carry advisories.

Control the channels with `WithAdvisories(metadata, api)`:

```go
// Metadata embed only (no batch API). Prefer this together with PackageLister
// so clients can avoid probing packages you do not know.
h := repository.NewHandler(p, repository.WithAdvisories(true, false))

// Batch API only — no per-package embed (reduces metadata size for huge advisory sets).
h = repository.NewHandler(p, repository.WithAdvisories(false, true))

// Disable entirely even though Advisor is implemented.
h = repository.NewHandler(p, repository.WithAdvisories(false, false))
```

> Composer rejects metadata-only advisory repositories that advertise neither
> `available-packages` nor `available-package-patterns`: without an api-url it
> needs a finite package list to know which metadata files to fetch. Implement
> `PackageLister` when serving metadata-only advisories.

## Stable vs. development split

The handler routes each version into the stable (`.json`) or development
(`~dev.json`) file. By default a version is considered a development version
when it starts with `dev-` or ends with `-dev`. Override that with
`WithDevClassifier`:

```go
h := repository.NewHandler(&myProvider{}, repository.WithDevClassifier(
	func(v repository.Version) bool {
		return strings.Contains(v.Version, "-dev") || strings.HasPrefix(v.Version, "dev-")
	},
))
```

## Authentication

The handler itself serves everything publicly — authentication is added with
standard `net/http` middleware, the same way you'd secure any handler (the
library deliberately keeps auth out of `NewHandler`).

A Composer client authenticating to a private repository sends a normal HTTP
header on **every** request (`/packages.json`, each `/p2/...`, and the search and
advisory endpoints), picked by origin host from its `auth.json` —
`Authorization: Basic …` for `http-basic`, `Authorization: Bearer …` for
`bearer`, `PRIVATE-TOKEN: …` for `gitlab-token`, and so on. The server's job is
to validate that header and return `401` with `WWW-Authenticate` when it is
missing or invalid. (This is the mirror of what the
[client](./repository-client.md#authentication) sends, so a private setup
round-trips.)

### Gate the whole repository

Wrap the handler; the middleware covers all endpoints at once because each
request carries the same header.

```go
func basicAuth(next http.Handler, ok func(user, pass string) bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, hasAuth := r.BasicAuth()
		if !hasAuth || !ok(user, pass) {
			w.Header().Set("WWW-Authenticate", `Basic realm="composer"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

handler := basicAuth(repository.NewHandler(provider), checkCreds)
http.ListenAndServe(":8080", handler)
```

### Per-user visibility

`Package`, `Search` and `PackageNames` all receive a `context.Context`, so the
middleware can authenticate, attach the identity to the context, and let the
provider tailor what each caller sees — including returning
`repository.ErrPackageNotFound` (→ `404`) to hide packages a user may not access.

```go
type ctxKey struct{}

func auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := authenticate(r)
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="composer"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, user)))
	})
}

func (p *myProvider) Package(ctx context.Context, name string) (*repository.Package, error) {
	user, _ := ctx.Value(ctxKey{}).(string)
	if !p.canSee(user, name) {
		return nil, repository.ErrPackageNotFound // 404, indistinguishable from absent
	}
	return p.lookup(name)
}
```

Returning a user-specific list from `PackageNames(ctx)` likewise scopes the
`available-packages` advertised in `packages.json` per caller.

## Round-tripping with the client

Because both halves share the wire types, the client reads back exactly what the
handler writes — handy in tests:

```go
srv := httptest.NewServer(repository.NewHandler(&myProvider{}))
defer srv.Close()

client := repository.New(srv.URL, nil)
pkg, _ := client.GetPackage(context.Background(), "acme/lib")
```
