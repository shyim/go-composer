package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/shyim/go-composer"
)

// ErrPackageNotFound is returned when a repository does not know a package.
var ErrPackageNotFound = errors.New("package not found")

// Client is a client for a single Composer "type": "composer" repository (such
// as packagist.org or a private Satis/Packagist instance) speaking the V2
// metadata protocol.
type Client struct {
	// HTTPClient is used for all requests. When nil a client with a sane
	// timeout is used.
	HTTPClient *http.Client

	url  string         // canonical base URL, no trailing slash
	auth *composer.Auth // optional credentials applied per origin

	mu   sync.Mutex
	root *RootFile
}

// New creates a client for the repository at repoURL. auth may be nil; when
// provided, credentials matching the request origin are attached.
func New(repoURL string, auth *composer.Auth) *Client {
	return &Client{
		url:  strings.TrimRight(repoURL, "/"),
		auth: auth,
	}
}

// URL returns the repository's canonical base URL.
func (c *Client) URL() string { return c.url }

func (c *Client) client() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// packagesJSONURL returns the URL of the repository root file, appending
// /packages.json unless the configured URL already points at a .json file.
func (c *Client) packagesJSONURL() string {
	if u, err := url.Parse(c.url); err == nil && strings.Contains(u.Path, ".json") {
		return c.url
	}
	return c.url + "/packages.json"
}

// canonicalizeURL resolves a possibly-relative URL advertised in the root file
// against the repository origin, matching Composer's behavior: a leading "/"
// is joined to scheme+host, everything else is returned unchanged.
func (c *Client) canonicalizeURL(raw string) string {
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "/") {
		if u, err := url.Parse(c.url); err == nil {
			return u.Scheme + "://" + u.Host + raw
		}
		return c.url
	}
	return raw
}

// loadRoot fetches and caches the repository root (packages.json).
func (c *Client) loadRoot(ctx context.Context) (*RootFile, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.root != nil {
		return c.root, nil
	}

	body, status, err := c.get(ctx, c.packagesJSONURL())
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("loading repository root %s: unexpected status %d", c.packagesJSONURL(), status)
	}

	var r RootFile
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("parsing repository root %s: %w", c.packagesJSONURL(), err)
	}

	r.MetadataURL = c.canonicalizeURL(r.MetadataURL)
	r.SearchURL = c.canonicalizeURL(r.SearchURL)
	r.ListURL = c.canonicalizeURL(r.ListURL)
	if r.SecurityAdvisories != nil {
		r.SecurityAdvisories.APIURL = c.canonicalizeURL(r.SecurityAdvisories.APIURL)
	}

	c.root = &r
	return c.root, nil
}

// matchPackagePattern matches a name against a pattern using "*" as a wildcard
// for any substring (e.g. "vendor/*"), as used by available-package-patterns.
func matchPackagePattern(pattern, name string) bool {
	parts := strings.Split(strings.ToLower(pattern), "*")
	pos := 0
	for i, part := range parts {
		if part == "" {
			continue
		}
		idx := strings.Index(name[pos:], part)
		if idx < 0 {
			return false
		}
		if i == 0 && idx != 0 {
			return false
		}
		pos += idx + len(part)
	}
	// A pattern with no trailing wildcard must consume the whole name.
	if !strings.HasSuffix(pattern, "*") {
		return pos == len(name)
	}
	return true
}
