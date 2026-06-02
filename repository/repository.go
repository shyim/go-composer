// Package repository queries Composer "type": "composer" repositories (such as
// packagist.org or a private Satis/Packagist instance) over Composer's V2
// metadata protocol.
//
// A Client fetches a repository's packages.json root, resolves its metadata-url
// (substituting %package% and reading the stable and ~dev files), expands the
// minified "composer/2.0" delta format, and returns a package's versions with
// their requirements, dist/source and other metadata. It also exposes the
// search and security-advisory endpoints. A Set queries several repositories in
// order with first-match-wins resolution and can be built from a parsed
// composer.json. Credentials from an auth.json are applied per request origin.
package repository

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	packagist "github.com/shyim/go-packagist"
)

// ErrPackageNotFound is returned when a repository does not know a package.
var ErrPackageNotFound = errors.New("package not found")

// Version describes a single released version of a package, as returned by a
// Composer repository's metadata endpoint. It mirrors the version objects found
// under packages[<name>] in a p2 metadata file, which are essentially the
// package's composer.json augmented with version_normalized, dist and source.
type Version struct {
	Name              string                              `json:"name"`
	Version           string                              `json:"version"`
	VersionNormalized string                              `json:"version_normalized,omitempty"`
	Description       string                              `json:"description,omitempty"`
	Type              string                              `json:"type,omitempty"`
	Keywords          []string                            `json:"keywords,omitempty"`
	Homepage          string                              `json:"homepage,omitempty"`
	Time              string                              `json:"time,omitempty"`
	License           *packagist.StringOrSlice            `json:"license,omitempty"`
	Authors           []packagist.ComposerJsonAuthor      `json:"authors,omitempty"`
	Support           *packagist.ComposerJsonSupport      `json:"support,omitempty"`
	Funding           []packagist.ComposerFunding         `json:"funding,omitempty"`
	Require           map[string]string                   `json:"require,omitempty"`
	RequireDev        map[string]string                   `json:"require-dev,omitempty"`
	Conflict          map[string]string                   `json:"conflict,omitempty"`
	Replace           map[string]string                   `json:"replace,omitempty"`
	Provide           map[string]string                   `json:"provide,omitempty"`
	Suggest           map[string]string                   `json:"suggest,omitempty"`
	Autoload          *packagist.ComposerJsonAutoload     `json:"autoload,omitempty"`
	Dist              packagist.ComposerLockPackageDist   `json:"dist,omitempty"`
	Source            packagist.ComposerLockPackageSource `json:"source,omitempty"`
	Extra             packagist.ExtraData                 `json:"extra,omitempty"`
}

// Package is the full set of versions a repository knows for a package.
type Package struct {
	Name     string
	Versions []Version
}

// Version returns the version whose "version" or "version_normalized" equals v,
// or nil when none matches. No constraint solving is performed; matching is
// exact (callers own version resolution to keep this library dependency-free).
func (p *Package) Version(v string) *Version {
	for i := range p.Versions {
		if p.Versions[i].Version == v || p.Versions[i].VersionNormalized == v {
			return &p.Versions[i]
		}
	}
	return nil
}

// SearchResult is one entry returned by a repository's search endpoint.
type SearchResult struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url,omitempty"`
	Repository  string `json:"repository,omitempty"`
	Downloads   int    `json:"downloads,omitempty"`
	Favers      int    `json:"favers,omitempty"`
	Virtual     bool   `json:"virtual,omitempty"`
}

// SecurityAdvisory describes a known vulnerability affecting a package, as
// returned by the security-advisories metadata or API endpoint.
type SecurityAdvisory struct {
	AdvisoryID       string `json:"advisoryId,omitempty"`
	PackageName      string `json:"packageName,omitempty"`
	AffectedVersions string `json:"affectedVersions,omitempty"`
	Title            string `json:"title,omitempty"`
	CVE              string `json:"cve,omitempty"`
	Link             string `json:"link,omitempty"`
	ReportedAt       string `json:"reportedAt,omitempty"`
	Severity         string `json:"severity,omitempty"`
	Sources          []struct {
		Name     string `json:"name,omitempty"`
		RemoteID string `json:"remoteId,omitempty"`
	} `json:"sources,omitempty"`
}

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

// GetPackage returns every version the repository knows for name. It returns
// ErrPackageNotFound when the repository does not provide the package.
func (c *Client) GetPackage(ctx context.Context, name string) (*Package, error) {
	r, err := c.loadRoot(ctx)
	if err != nil {
		return nil, err
	}

	if !r.knowsPackage(name) {
		return nil, fmt.Errorf("%q: %w", name, ErrPackageNotFound)
	}

	lower := strings.ToLower(name)

	// Partial packages declared inline in the root file take priority over a
	// lazy metadata-url lookup, matching Composer.
	if raw, ok := r.Packages[lower]; ok {
		versions, err := decodeVersionList(raw, "")
		if err != nil {
			return nil, err
		}
		if len(versions) > 0 {
			return &Package{Name: name, Versions: versions}, nil
		}
	}

	if r.MetadataURL == "" {
		return nil, fmt.Errorf("repository %s does not support the v2 metadata protocol (no metadata-url)", c.url)
	}

	// V2 lazy lookup: fetch the stable file and the optional ~dev file.
	stable, found, err := c.fetchPackageFile(ctx, r.MetadataURL, lower, lower)
	if err != nil {
		return nil, err
	}
	dev, devFound, err := c.fetchPackageFile(ctx, r.MetadataURL, lower+"~dev", lower)
	if err != nil {
		return nil, err
	}

	if !found && !devFound {
		return nil, fmt.Errorf("%q: %w", name, ErrPackageNotFound)
	}

	return &Package{Name: name, Versions: append(stable, dev...)}, nil
}

// metadataFile is the shape of a p2 metadata document.
type metadataFile struct {
	Minified string                     `json:"minified"`
	Packages map[string]json.RawMessage `json:"packages"`
}

// fetchPackageFile fetches a single metadata file, substituting %package% with
// fileName, and returns the expanded version list for packageName. A 404 is
// reported as found=false with no error (e.g. a missing ~dev file).
func (c *Client) fetchPackageFile(ctx context.Context, metadataURL, fileName, packageName string) ([]Version, bool, error) {
	reqURL := strings.ReplaceAll(metadataURL, "%package%", fileName)

	body, status, err := c.get(ctx, reqURL)
	if err != nil {
		return nil, false, err
	}
	if status == http.StatusNotFound {
		return nil, false, nil
	}
	if status != http.StatusOK {
		return nil, false, fmt.Errorf("fetching %s: unexpected status %d", reqURL, status)
	}

	var file metadataFile
	if err := json.Unmarshal(body, &file); err != nil {
		return nil, false, fmt.Errorf("parsing %s: %w", reqURL, err)
	}

	raw, ok := file.Packages[packageName]
	if !ok {
		return nil, false, nil
	}

	versions, err := decodeVersionList(raw, file.Minified)
	if err != nil {
		return nil, false, fmt.Errorf("parsing %s: %w", reqURL, err)
	}
	return versions, true, nil
}

// decodeVersionList decodes a packages[<name>] array, expanding it first when
// the document was minified with the "composer/2.0" format.
func decodeVersionList(raw json.RawMessage, minified string) ([]Version, error) {
	var rawVersions []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rawVersions); err != nil {
		return nil, err
	}

	if minified == "composer/2.0" {
		rawVersions = expandMetadata(rawVersions)
	}

	versions := make([]Version, 0, len(rawVersions))
	for _, rv := range rawVersions {
		obj, err := json.Marshal(rv)
		if err != nil {
			return nil, err
		}
		var v Version
		if err := json.Unmarshal(obj, &v); err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, nil
}

// Search queries the repository's full-text search endpoint. It returns nil
// (no error) when the repository advertises no search URL.
func (c *Client) Search(ctx context.Context, query string) ([]SearchResult, error) {
	r, err := c.loadRoot(ctx)
	if err != nil {
		return nil, err
	}
	if r.SearchURL == "" {
		return nil, nil
	}

	reqURL := strings.ReplaceAll(r.SearchURL, "%query%", url.QueryEscape(query))
	reqURL = strings.ReplaceAll(reqURL, "%type%", "")

	body, status, err := c.get(ctx, reqURL)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("searching %s: unexpected status %d", reqURL, status)
	}

	var resp struct {
		Results []SearchResult `json:"results"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing search response: %w", err)
	}

	// Drop virtual packages, which are not directly installable.
	out := resp.Results[:0]
	for _, res := range resp.Results {
		if res.Virtual {
			continue
		}
		out = append(out, res)
	}
	return out, nil
}

// advisoriesResponse is the shape of the security-advisories API response.
type advisoriesResponse struct {
	Advisories map[string][]SecurityAdvisory `json:"advisories"`
}

// GetSecurityAdvisories returns known advisories for the given packages, keyed
// by package name. It prefers the repository's security-advisories api-url
// (a single POST), falling back to per-package metadata when only the metadata
// flag is advertised. Repositories with no advisory support return an empty map.
func (c *Client) GetSecurityAdvisories(ctx context.Context, packages []string) (map[string][]SecurityAdvisory, error) {
	r, err := c.loadRoot(ctx)
	if err != nil {
		return nil, err
	}

	result := map[string][]SecurityAdvisory{}
	if r.SecurityAdvisories == nil || len(packages) == 0 {
		return result, nil
	}

	// Respect the advertised package list, if any.
	wanted := make([]string, 0, len(packages))
	for _, name := range packages {
		if r.knowsPackage(name) {
			wanted = append(wanted, name)
		}
	}
	if len(wanted) == 0 {
		return result, nil
	}

	if r.SecurityAdvisories.APIURL != "" {
		form := url.Values{}
		for _, name := range wanted {
			form.Add("packages[]", name)
		}

		body, status, err := c.post(ctx, r.SecurityAdvisories.APIURL, form)
		if err != nil {
			return nil, err
		}
		if status != http.StatusOK {
			return nil, fmt.Errorf("fetching advisories from %s: unexpected status %d", r.SecurityAdvisories.APIURL, status)
		}

		var resp advisoriesResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("parsing advisories response: %w", err)
		}
		for name, list := range resp.Advisories {
			if len(list) > 0 {
				result[name] = list
			}
		}
		return result, nil
	}

	// metadata-only: advisories are embedded in each package's metadata file.
	if r.SecurityAdvisories.Metadata && r.MetadataURL != "" {
		for _, name := range wanted {
			lower := strings.ToLower(name)
			reqURL := strings.ReplaceAll(r.MetadataURL, "%package%", lower)
			body, status, err := c.get(ctx, reqURL)
			if err != nil {
				return nil, err
			}
			if status != http.StatusOK {
				continue
			}
			var file struct {
				SecurityAdvisories []SecurityAdvisory `json:"security-advisories"`
			}
			if err := json.Unmarshal(body, &file); err != nil {
				return nil, fmt.Errorf("parsing %s: %w", reqURL, err)
			}
			if len(file.SecurityAdvisories) > 0 {
				result[name] = file.SecurityAdvisories
			}
		}
	}

	return result, nil
}

// get performs a GET, attaching auth headers for the request origin, and
// returns the body and status code.
func (c *Client) get(ctx context.Context, reqURL string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, 0, err
	}
	return c.do(req)
}

// post performs an application/x-www-form-urlencoded POST.
func (c *Client) post(ctx context.Context, reqURL string, form url.Values) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(req)
}

func (c *Client) do(req *http.Request) ([]byte, int, error) {
	c.applyAuth(req)

	resp, err := c.client().Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

// applyAuth attaches credentials from the associated auth.json that match the
// request's origin (host), following Composer's auth header conventions.
func (c *Client) applyAuth(req *http.Request) {
	if c.auth == nil {
		return
	}
	host := req.URL.Host

	if basic, ok := c.auth.HTTPBasicAuth[host]; ok {
		token := base64.StdEncoding.EncodeToString([]byte(basic.Username + ":" + basic.Password))
		req.Header.Set("Authorization", "Basic "+token)
		return
	}
	if token, ok := c.auth.BearerAuth[host]; ok {
		req.Header.Set("Authorization", "Bearer "+token)
		return
	}
	if token, ok := c.auth.GithubOAuth[host]; ok {
		req.Header.Set("Authorization", "token "+token)
		return
	}
	if tok, ok := c.auth.GitlabOAuth[host]; ok {
		req.Header.Set("Authorization", "Bearer "+tok.Token)
		return
	}
	if tok, ok := c.auth.GitlabAuth[host]; ok {
		req.Header.Set("PRIVATE-TOKEN", tok.Token)
		return
	}
	if headers, ok := c.auth.CustomHeaders[host]; ok {
		for _, h := range headers {
			if name, value, found := strings.Cut(h, ":"); found {
				req.Header.Add(strings.TrimSpace(name), strings.TrimSpace(value))
			}
		}
	}
}
