package packagist

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
)

// ErrPackageNotFound is returned when a repository does not know a package.
var ErrPackageNotFound = errors.New("package not found")

// PackageVersion describes a single released version of a package, as returned
// by a Composer repository's metadata endpoint. It mirrors the version objects
// found under packages[<name>] in a p2 metadata file, which are essentially the
// package's composer.json augmented with version_normalized, dist and source.
type PackageVersion struct {
	Name              string                    `json:"name"`
	Version           string                    `json:"version"`
	VersionNormalized string                    `json:"version_normalized,omitempty"`
	Description       string                    `json:"description,omitempty"`
	Type              string                    `json:"type,omitempty"`
	Keywords          []string                  `json:"keywords,omitempty"`
	Homepage          string                    `json:"homepage,omitempty"`
	Time              string                    `json:"time,omitempty"`
	License           *StringOrSlice            `json:"license,omitempty"`
	Authors           []ComposerJsonAuthor      `json:"authors,omitempty"`
	Support           *ComposerJsonSupport      `json:"support,omitempty"`
	Funding           []ComposerFunding         `json:"funding,omitempty"`
	Require           map[string]string         `json:"require,omitempty"`
	RequireDev        map[string]string         `json:"require-dev,omitempty"`
	Conflict          map[string]string         `json:"conflict,omitempty"`
	Replace           map[string]string         `json:"replace,omitempty"`
	Provide           map[string]string         `json:"provide,omitempty"`
	Suggest           map[string]string         `json:"suggest,omitempty"`
	Autoload          *ComposerJsonAutoload     `json:"autoload,omitempty"`
	Dist              ComposerLockPackageDist   `json:"dist,omitempty"`
	Source            ComposerLockPackageSource `json:"source,omitempty"`
	Extra             ExtraData                 `json:"extra,omitempty"`
}

// PackageMetadata is the full set of versions a repository knows for a package.
type PackageMetadata struct {
	Name     string
	Versions []PackageVersion
}

// Version returns the version whose "version" or "version_normalized" equals v,
// or nil when none matches. No constraint solving is performed; matching is
// exact (callers own version resolution to keep this library dependency-free).
func (m *PackageMetadata) Version(v string) *PackageVersion {
	for i := range m.Versions {
		if m.Versions[i].Version == v || m.Versions[i].VersionNormalized == v {
			return &m.Versions[i]
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

// repositoryRoot models the subset of a packages.json root file this client
// understands. The presence of metadata-url marks a V2 protocol repository.
type repositoryRoot struct {
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

// ComposerRepository is a client for a single Composer "type": "composer"
// repository (such as packagist.org or a private Satis/Packagist instance)
// speaking the V2 metadata protocol.
type ComposerRepository struct {
	// HTTPClient is used for all requests. When nil a client with a sane
	// timeout is used.
	HTTPClient *http.Client

	url  string        // canonical base URL, no trailing slash
	auth *ComposerAuth // optional credentials applied per origin

	mu   sync.Mutex
	root *repositoryRoot
}

// NewComposerRepository creates a client for the repository at url. auth may be
// nil; when provided, credentials matching the request origin are attached.
func NewComposerRepository(repoURL string, auth *ComposerAuth) *ComposerRepository {
	return &ComposerRepository{
		url:  strings.TrimRight(repoURL, "/"),
		auth: auth,
	}
}

// URL returns the repository's canonical base URL.
func (r *ComposerRepository) URL() string { return r.url }

func (r *ComposerRepository) client() *http.Client {
	if r.HTTPClient != nil {
		return r.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// packagesJSONURL returns the URL of the repository root file, appending
// /packages.json unless the configured URL already points at a .json file.
func (r *ComposerRepository) packagesJSONURL() string {
	if u, err := url.Parse(r.url); err == nil && strings.Contains(u.Path, ".json") {
		return r.url
	}
	return r.url + "/packages.json"
}

// canonicalizeURL resolves a possibly-relative URL advertised in the root file
// against the repository origin, matching Composer's behavior: a leading "/"
// is joined to scheme+host, everything else is returned unchanged.
func (r *ComposerRepository) canonicalizeURL(raw string) string {
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "/") {
		if u, err := url.Parse(r.url); err == nil {
			return u.Scheme + "://" + u.Host + raw
		}
		return r.url
	}
	return raw
}

// loadRoot fetches and caches the repository root (packages.json).
func (r *ComposerRepository) loadRoot(ctx context.Context) (*repositoryRoot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.root != nil {
		return r.root, nil
	}

	body, status, err := r.get(ctx, r.packagesJSONURL())
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("loading repository root %s: unexpected status %d", r.packagesJSONURL(), status)
	}

	var root repositoryRoot
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("parsing repository root %s: %w", r.packagesJSONURL(), err)
	}

	root.MetadataURL = r.canonicalizeURL(root.MetadataURL)
	root.SearchURL = r.canonicalizeURL(root.SearchURL)
	root.ListURL = r.canonicalizeURL(root.ListURL)
	if root.SecurityAdvisories != nil {
		root.SecurityAdvisories.APIURL = r.canonicalizeURL(root.SecurityAdvisories.APIURL)
	}

	r.root = &root
	return r.root, nil
}

// knowsPackage reports whether name is covered by the repository's advertised
// available-packages / available-package-patterns list. When the repository
// advertises no such list, it is treated as lazy and every name is allowed.
func (root *repositoryRoot) knowsPackage(name string) bool {
	if len(root.AvailablePackages) == 0 && len(root.AvailablePackagePatterns) == 0 {
		return true
	}
	name = strings.ToLower(name)
	for _, p := range root.AvailablePackages {
		if strings.ToLower(p) == name {
			return true
		}
	}
	for _, pattern := range root.AvailablePackagePatterns {
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
func (r *ComposerRepository) GetPackage(ctx context.Context, name string) (*PackageMetadata, error) {
	root, err := r.loadRoot(ctx)
	if err != nil {
		return nil, err
	}

	if !root.knowsPackage(name) {
		return nil, fmt.Errorf("%q: %w", name, ErrPackageNotFound)
	}

	lower := strings.ToLower(name)

	// Partial packages declared inline in the root file take priority over a
	// lazy metadata-url lookup, matching Composer.
	if raw, ok := root.Packages[lower]; ok {
		versions, err := decodeVersionList(raw, "")
		if err != nil {
			return nil, err
		}
		if len(versions) > 0 {
			return &PackageMetadata{Name: name, Versions: versions}, nil
		}
	}

	if root.MetadataURL == "" {
		return nil, fmt.Errorf("repository %s does not support the v2 metadata protocol (no metadata-url)", r.url)
	}

	// V2 lazy lookup: fetch the stable file and the optional ~dev file.
	stable, found, err := r.fetchPackageFile(ctx, root.MetadataURL, lower, lower)
	if err != nil {
		return nil, err
	}
	dev, devFound, err := r.fetchPackageFile(ctx, root.MetadataURL, lower+"~dev", lower)
	if err != nil {
		return nil, err
	}

	if !found && !devFound {
		return nil, fmt.Errorf("%q: %w", name, ErrPackageNotFound)
	}

	return &PackageMetadata{Name: name, Versions: append(stable, dev...)}, nil
}

// metadataFile is the shape of a p2 metadata document.
type metadataFile struct {
	Minified string                     `json:"minified"`
	Packages map[string]json.RawMessage `json:"packages"`
}

// fetchPackageFile fetches a single metadata file, substituting %package% with
// fileName, and returns the expanded version list for packageName. A 404 is
// reported as found=false with no error (e.g. a missing ~dev file).
func (r *ComposerRepository) fetchPackageFile(ctx context.Context, metadataURL, fileName, packageName string) ([]PackageVersion, bool, error) {
	reqURL := strings.ReplaceAll(metadataURL, "%package%", fileName)

	body, status, err := r.get(ctx, reqURL)
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
func decodeVersionList(raw json.RawMessage, minified string) ([]PackageVersion, error) {
	var rawVersions []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rawVersions); err != nil {
		return nil, err
	}

	if minified == "composer/2.0" {
		rawVersions = expandMetadata(rawVersions)
	}

	versions := make([]PackageVersion, 0, len(rawVersions))
	for _, rv := range rawVersions {
		obj, err := json.Marshal(rv)
		if err != nil {
			return nil, err
		}
		var pv PackageVersion
		if err := json.Unmarshal(obj, &pv); err != nil {
			return nil, err
		}
		versions = append(versions, pv)
	}
	return versions, nil
}

// Search queries the repository's full-text search endpoint. It returns nil
// (no error) when the repository advertises no search URL.
func (r *ComposerRepository) Search(ctx context.Context, query string) ([]SearchResult, error) {
	root, err := r.loadRoot(ctx)
	if err != nil {
		return nil, err
	}
	if root.SearchURL == "" {
		return nil, nil
	}

	reqURL := strings.ReplaceAll(root.SearchURL, "%query%", url.QueryEscape(query))
	reqURL = strings.ReplaceAll(reqURL, "%type%", "")

	body, status, err := r.get(ctx, reqURL)
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
func (r *ComposerRepository) GetSecurityAdvisories(ctx context.Context, packages []string) (map[string][]SecurityAdvisory, error) {
	root, err := r.loadRoot(ctx)
	if err != nil {
		return nil, err
	}

	result := map[string][]SecurityAdvisory{}
	if root.SecurityAdvisories == nil || len(packages) == 0 {
		return result, nil
	}

	// Respect the advertised package list, if any.
	wanted := make([]string, 0, len(packages))
	for _, name := range packages {
		if root.knowsPackage(name) {
			wanted = append(wanted, name)
		}
	}
	if len(wanted) == 0 {
		return result, nil
	}

	if root.SecurityAdvisories.APIURL != "" {
		form := url.Values{}
		for _, name := range wanted {
			form.Add("packages[]", name)
		}

		body, status, err := r.post(ctx, root.SecurityAdvisories.APIURL, form)
		if err != nil {
			return nil, err
		}
		if status != http.StatusOK {
			return nil, fmt.Errorf("fetching advisories from %s: unexpected status %d", root.SecurityAdvisories.APIURL, status)
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
	if root.SecurityAdvisories.Metadata && root.MetadataURL != "" {
		for _, name := range wanted {
			lower := strings.ToLower(name)
			reqURL := strings.ReplaceAll(root.MetadataURL, "%package%", lower)
			body, status, err := r.get(ctx, reqURL)
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
func (r *ComposerRepository) get(ctx context.Context, reqURL string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, 0, err
	}
	return r.do(req)
}

// post performs an application/x-www-form-urlencoded POST.
func (r *ComposerRepository) post(ctx context.Context, reqURL string, form url.Values) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return r.do(req)
}

func (r *ComposerRepository) do(req *http.Request) ([]byte, int, error) {
	r.applyAuth(req)

	resp, err := r.client().Do(req)
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
func (r *ComposerRepository) applyAuth(req *http.Request) {
	if r.auth == nil {
		return
	}
	host := req.URL.Host

	if basic, ok := r.auth.HTTPBasicAuth[host]; ok {
		token := base64.StdEncoding.EncodeToString([]byte(basic.Username + ":" + basic.Password))
		req.Header.Set("Authorization", "Basic "+token)
		return
	}
	if token, ok := r.auth.BearerAuth[host]; ok {
		req.Header.Set("Authorization", "Bearer "+token)
		return
	}
	if token, ok := r.auth.GithubOAuth[host]; ok {
		req.Header.Set("Authorization", "token "+token)
		return
	}
	if tok, ok := r.auth.GitlabOAuth[host]; ok {
		req.Header.Set("Authorization", "Bearer "+tok.Token)
		return
	}
	if tok, ok := r.auth.GitlabAuth[host]; ok {
		req.Header.Set("PRIVATE-TOKEN", tok.Token)
		return
	}
	if headers, ok := r.auth.CustomHeaders[host]; ok {
		for _, h := range headers {
			if name, value, found := strings.Cut(h, ":"); found {
				req.Header.Add(strings.TrimSpace(name), strings.TrimSpace(value))
			}
		}
	}
}
