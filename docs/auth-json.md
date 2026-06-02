# Working with `auth.json`

[← Back to README](../README.md)

`composer.ReadAuth` parses an `auth.json` into a `*composer.Auth`. If the file
does not exist, an empty (but usable) configuration is returned rather than an
error. It reads only the file; to also layer in the `COMPOSER_AUTH` environment
variable (as the Composer CLI does), call `MergeEnv` — see
[Environment (`COMPOSER_AUTH`)](#environment-composer_auth) below. `Save` writes
the file back with `0600` permissions.

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

The Composer CLI also reads credentials from the `COMPOSER_AUTH` environment
variable — a JSON document in `auth.json` format. `ReadAuth` reads **only the
file**; layering in the environment is **opt-in** via `MergeEnv`, so it never
happens behind your back:

```go
auth, err := composer.ReadAuth("auth.json")
if err != nil {
	log.Fatal(err)
}
if err := auth.MergeEnv(); err != nil {
	log.Fatal(err) // COMPOSER_AUTH was set but is not valid JSON
}
```

`MergeEnv` merges the environment **on top of** the current configuration, and
works on a fresh `&composer.Auth{}` too, so credentials can come entirely from
the environment (handy in CI):

```sh
export COMPOSER_AUTH='{"http-basic":{"repo.example.com":{"username":"token","password":"secret"}}}'
```

Precedence when both the current config and the environment provide a value:

- Per-host methods (`http-basic`, `bearer`, `gitlab-token`, `gitlab-oauth`,
  `github-oauth`, `bitbucket-oauth`, `custom-headers`) — an environment entry
  **overrides** the entry for the **same host**; other hosts are untouched.
- `gitlab-domains` / `github-domains` — a non-empty list in the environment
  **replaces** the current list entirely.
- `MergeEnv` is a no-op when `COMPOSER_AUTH` is unset or empty, and returns an
  error when it is set but not valid JSON.

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
