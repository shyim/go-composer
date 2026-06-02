# Working with `auth.json`

[← Back to README](../README.md)

`composer.ReadAuth` parses an `auth.json` into a `*composer.Auth`. If the file
does not exist, an empty (but usable) configuration is returned rather than an
error. `ReadAuth` also merges the `COMPOSER_AUTH` environment variable on top of
the file — see [Environment (`COMPOSER_AUTH`)](#environment-composer_auth) below.
`Save` writes the file back with `0600` permissions.

```go
package main

import (
	"log"

	"github.com/shyim/go-composer"
)

func main() {
	auth, err := composer.ReadAuth("auth.json")
	if err != nil {
		log.Fatal(err)
	}

	auth.HTTPBasicAuth["repo.example.com"] = composer.BasicAuth{
		Username: "token",
		Password: "secret",
	}
	auth.BearerAuth["repo.other.com"] = "a-bearer-token"

	if err := auth.Save(); err != nil {
		log.Fatal(err)
	}
}
```

## Authentication methods

Each field is keyed by host (origin), matching Composer's `auth.json` layout:

| Field | `auth.json` key | Value |
|---|---|---|
| `HTTPBasicAuth` | `http-basic` | `composer.BasicAuth{Username, Password}` |
| `BearerAuth` | `bearer` | token string |
| `GitlabAuth` | `gitlab-token` | `composer.GitlabToken` |
| `GitlabOAuth` | `gitlab-oauth` | `composer.GitlabOAuthToken` |
| `GithubOAuth` | `github-oauth` | token string |
| `BitbucketOauth` | `bitbucket-oauth` | `map[string]string` |
| `CustomHeaders` | `custom-headers` | `[]string` |
| `GitlabDomains` | `gitlab-domains` | `[]string` |
| `GithubDomains` | `github-domains` | `[]string` |

Unknown or future top-level keys are preserved across a read/modify/write
round-trip.

## Environment (`COMPOSER_AUTH`)

Like Composer itself, `ReadAuth` also reads the `COMPOSER_AUTH` environment
variable — a JSON document in `auth.json` format — and merges it **on top of**
whatever the file contains. This happens whether or not `auth.json` exists, so
credentials can be supplied entirely through the environment (handy in CI):

```sh
export COMPOSER_AUTH='{"http-basic":{"repo.example.com":{"username":"token","password":"secret"}}}'
```

Precedence when both the file and the environment provide a value:

- Per-host methods (`http-basic`, `bearer`, `gitlab-token`, `gitlab-oauth`,
  `github-oauth`, `bitbucket-oauth`, `custom-headers`) — an environment entry
  **overrides** the file entry for the **same host**; other hosts are untouched.
- `gitlab-domains` / `github-domains` — a non-empty list in the environment
  **replaces** the file's list entirely.
- An unset or malformed `COMPOSER_AUTH` is ignored.

If you want the file's contents only, read it yourself (`os.ReadFile` +
`json.Unmarshal` into a `composer.Auth`) instead of calling `ReadAuth`, which
always applies the environment merge.

## Serializing without writing to disk

```go
data, err := auth.Json(true) // true = indented; false = compact
if err != nil {
	log.Fatal(err)
}
log.Println(string(data))
```

## Using credentials when querying repositories

A `*composer.Auth` can be passed straight to the repository client, which
applies the matching credentials per request origin. See
[Querying repositories](./repository-client.md).
