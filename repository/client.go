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

	packagist "github.com/shyim/go-packagist"
)

// ErrPackageNotFound is returned when a repository does not know a package.
var ErrPackageNotFound = errors.New("package not found")

// root models the subset of a packages.json root file this client understands.
// The presence of metadata-url marks a V2 protocol repository.
type root struct {
	MetadataURL              string                     `json:"metadata-url"`
	AvailablePackages        []string                   `json:"available-packages"`
	AvailablePackagePatterns []string                   `json:"available-package-patterns"`
	SearchURL                string                     `json:"search"`
	ListURL                  string                     `json:"list"`
	NotifyBatch              string                     `json:"notify-batch"`
	Packages                 map[string]json.RawMessage `json:"packages"`
	SecurityAdvisories       *struct {
		Metadata bool   `json:"metadata"`
		APIURL   string `json:"api-url"`
	} `json:"security-advisories"`
}

// Client is a client for a single Composer "type": "composer" repository (such
// as packagist.org or a private Satis/Packagist instance) speaking the V2
// metadata protocol.
type Client struct {
	// HTTPClient is used for all requests. When nil a client with a sane
	// timeout is used.
	HTTPClient *http.Client

	url  string                  // canonical base URL, no trailing slash
	auth *packagist.ComposerAuth // optional credentials applied per origin

	mu   sync.Mutex
	root *root
}

// New creates a client for the repository at repoURL. auth may be nil; when
// provided, credentials matching the request origin are attached.
func New(repoURL string, auth *packagist.ComposerAuth) *Client {
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
func (c *Client) loadRoot(ctx context.Context) (*root, error) {
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

	var r root
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

// knowsPackage reports whether name is covered by the repository's advertised
// available-packages / available-package-patterns list. When the repository
// advertises no such list, it is treated as lazy and every name is allowed.
func (r *root) knowsPackage(name string) bool {
	if len(r.AvailablePackages) == 0 && len(r.AvailablePackagePatterns) == 0 {
		return true
	}
	name = strings.ToLower(name)
	for _, p := range r.AvailablePackages {
		if strings.ToLower(p) == name {
			return true
		}
	}
	for _, pattern := range r.AvailablePackagePatterns {
		if matchPackagePattern(pattern, name) {
			return true
		}
	}
	return false
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
