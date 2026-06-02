# Working with `auth.json`

[← Back to README](../README.md)

`composer.ReadAuth` parses an `auth.json` into a `*composer.Auth`. If the file
does not exist, an empty (but usable) configuration is returned rather than an
error, and credentials from the `COMPOSER_AUTH` environment variable are merged
in. `Save` writes the file back with `0600` permissions.

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
