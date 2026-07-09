package repository

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// Provider is the data source backing a repository server. It is the only
// required capability: every server can answer "give me the versions of this
// package".
type Provider interface {
	// Package returns every version known for name. Implementations must return
	// ErrPackageNotFound (matchable with errors.Is) when the package is unknown.
	Package(ctx context.Context, name string) (*Package, error)
}

// PackageLister is an optional capability: a Provider that can enumerate the
// package names it serves. When implemented, the server advertises the list as
// "available-packages" in packages.json, telling clients the repository is
// finite.
type PackageLister interface {
	PackageNames(ctx context.Context) ([]string, error)
}

// Searcher is an optional capability: a Provider that supports full-text
// search. When implemented, the server advertises and serves a search endpoint.
type Searcher interface {
	Search(ctx context.Context, query string) ([]SearchResult, error)
}

// Advisor is an optional capability: a Provider that reports security
// advisories. When implemented, the server:
//
//   - advertises security-advisories.api-url and serves
//     POST /security-advisories.json (full advisories, batch lookup);
//   - advertises security-advisories.metadata=true and embeds that package's
//     advisories in its stable /p2/<name>.json metadata document (Composer
//     clients and audit tools that prefer local metadata can use them without
//     an extra round-trip).
//
// Use WithAdvisories to turn either channel off. Implementations should return
// only packages that have advisories; empty maps/slices are fine.
type Advisor interface {
	SecurityAdvisories(ctx context.Context, names []string) (map[string][]SecurityAdvisory, error)
}

// Relative endpoint paths advertised in packages.json. They are returned as
// origin-relative URLs ("/..."), which Composer clients resolve against the
// repository origin, so the server needs no knowledge of its own base URL.
const (
	metadataURLTemplate = "/p2/%package%.json"
	searchURLTemplate   = "/search.json?q=%query%"
	packagesPath        = "/packages.json"
	p2Prefix            = "/p2/"
	searchPath          = "/search.json"
	advisoriesPath      = "/security-advisories.json"
	devSuffix           = "~dev"
)

// Handler serves the V2 repository protocol for a Provider. It is the server
// counterpart to Client: anything it writes, a Client can read back. Per-package
// metadata is always delta-encoded with the "composer/2.0" format, as
// packagist.org serves it.
type Handler struct {
	provider Provider
	isDev    func(v Version) bool
	mux      *http.ServeMux

	// Advisory delivery channels. Defaults are set to true when the provider
	// implements Advisor; overridden by WithAdvisories.
	advisoryMetadata bool
	advisoryAPI      bool
}

// HandlerOption configures a Handler.
type HandlerOption func(*Handler)

// WithDevClassifier overrides how the server decides whether a version belongs
// in the stable (.json) or development (~dev.json) metadata file. The default
// classifies versions starting with "dev-" or ending in "-dev" as development
// versions.
func WithDevClassifier(isDev func(v Version) bool) HandlerOption {
	return func(h *Handler) { h.isDev = isDev }
}

// WithAdvisories controls how a provider that implements Advisor publishes
// advisories. metadata embeds them in stable per-package metadata and sets
// security-advisories.metadata; api serves POST /security-advisories.json and
// sets security-advisories.api-url. Both default to true when Advisor is
// implemented. Passing (false, false) disables advisory delivery entirely.
//
// When only metadata is enabled, configure PackageLister so clients can scope
// lookup efficiently — Composer requires available-packages (or patterns)
// for metadata-only advisory repositories without an api-url.
func WithAdvisories(metadata, api bool) HandlerOption {
	return func(h *Handler) {
		h.advisoryMetadata = metadata
		h.advisoryAPI = api
	}
}

// NewHandler builds an http.Handler serving the repository protocol for p.
// Search and security-advisory endpoints are wired only when p implements the
// Searcher and Advisor capabilities respectively.
func NewHandler(p Provider, opts ...HandlerOption) http.Handler {
	h := &Handler{
		provider: p,
		isDev:    defaultIsDev,
	}
	if _, ok := p.(Advisor); ok {
		h.advisoryMetadata = true
		h.advisoryAPI = true
	}
	for _, opt := range opts {
		opt(h)
	}

	h.mux = http.NewServeMux()
	h.mux.HandleFunc(packagesPath, h.handlePackagesJSON)
	h.mux.HandleFunc(p2Prefix, h.handleMetadata)
	if _, ok := p.(Searcher); ok {
		h.mux.HandleFunc(searchPath, h.handleSearch)
	}
	if _, ok := p.(Advisor); ok && h.advisoryAPI {
		h.mux.HandleFunc(advisoriesPath, h.handleAdvisories)
	}

	return h.mux
}

// defaultIsDev classifies a version as a development version by its version
// string, without a full semver parser: "dev-*" branches and "*-dev" aliases.
func defaultIsDev(v Version) bool {
	return strings.HasPrefix(v.Version, "dev-") || strings.HasSuffix(v.Version, "-dev")
}

func (h *Handler) handlePackagesJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	root := RootFile{MetadataURL: metadataURLTemplate}

	if lister, ok := h.provider.(PackageLister); ok {
		names, err := lister.PackageNames(ctx)
		if err != nil {
			httpError(w, err)
			return
		}
		// An explicit (possibly empty) list marks the repository as finite.
		if names == nil {
			names = []string{}
		}
		root.AvailablePackages = names
	}

	if _, ok := h.provider.(Searcher); ok {
		root.SearchURL = searchURLTemplate
	}

	if _, ok := h.provider.(Advisor); ok && (h.advisoryMetadata || h.advisoryAPI) {
		cfg := &SecurityAdvisoriesConfig{Metadata: h.advisoryMetadata}
		if h.advisoryAPI {
			cfg.APIURL = advisoriesPath
		}
		root.SecurityAdvisories = cfg
	}

	writeJSON(w, root)
}

func (h *Handler) handleMetadata(w http.ResponseWriter, r *http.Request) {
	name, dev, ok := parseMetadataPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	pkg, err := h.provider.Package(r.Context(), name)
	if err != nil {
		if errors.Is(err, ErrPackageNotFound) {
			http.NotFound(w, r)
			return
		}
		httpError(w, err)
		return
	}

	// Split the package's versions into the stable and development files.
	selected := make([]Version, 0, len(pkg.Versions))
	for _, v := range pkg.Versions {
		if h.isDev(v) == dev {
			selected = append(selected, v)
		}
	}

	raw, err := EncodePackageVersions(selected, true)
	if err != nil {
		httpError(w, err)
		return
	}

	meta := Metadata{
		Minified: minifiedComposer2,
		Packages: map[string]json.RawMessage{name: raw},
	}

	// Embed advisories only in the stable metadata file, matching packagist.org
	// (dev files never carry security-advisories).
	if !dev && h.advisoryMetadata {
		if list, err := h.packageAdvisories(r.Context(), name); err != nil {
			httpError(w, err)
			return
		} else if len(list) > 0 {
			meta.SecurityAdvisories = list
		}
	}

	writeJSON(w, meta)
}

// packageAdvisories loads advisories for a single package from the Advisor
// capability, filling PackageName when the provider leaves it empty. Returns
// nil with no error when the provider is not an Advisor or has no entry.
func (h *Handler) packageAdvisories(ctx context.Context, name string) ([]SecurityAdvisory, error) {
	advisor, ok := h.provider.(Advisor)
	if !ok {
		return nil, nil
	}
	adv, err := advisor.SecurityAdvisories(ctx, []string{name})
	if err != nil {
		return nil, err
	}
	list := lookupAdvisories(adv, name)
	normalizeAdvisoryPackageNames(list, name)
	return list, nil
}

// lookupAdvisories returns advisories[name], falling back to a case-insensitive
// key match (Composer package names are case-insensitive).
func lookupAdvisories(adv map[string][]SecurityAdvisory, name string) []SecurityAdvisory {
	if list, ok := adv[name]; ok {
		return list
	}
	for k, list := range adv {
		if strings.EqualFold(k, name) {
			return list
		}
	}
	return nil
}

// normalizeAdvisoryPackageNames sets PackageName on entries that omit it.
func normalizeAdvisoryPackageNames(list []SecurityAdvisory, name string) {
	for i := range list {
		if list[i].PackageName == "" {
			list[i].PackageName = name
		}
	}
}

func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	searcher := h.provider.(Searcher)

	results, err := searcher.Search(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		httpError(w, err)
		return
	}
	if results == nil {
		results = []SearchResult{}
	}

	writeJSON(w, struct {
		Results []SearchResult `json:"results"`
	}{Results: results})
}

func (h *Handler) handleAdvisories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	advisor := h.provider.(Advisor)
	advisories, err := advisor.SecurityAdvisories(r.Context(), r.PostForm["packages[]"])
	if err != nil {
		httpError(w, err)
		return
	}
	if advisories == nil {
		advisories = map[string][]SecurityAdvisory{}
	}
	for name, list := range advisories {
		normalizeAdvisoryPackageNames(list, name)
		advisories[name] = list
	}

	writeJSON(w, struct {
		Advisories map[string][]SecurityAdvisory `json:"advisories"`
	}{Advisories: advisories})
}

// parseMetadataPath extracts the package name from a /p2/<vendor>/<name>.json
// (or ~dev.json) request path, reporting whether it is the development file.
func parseMetadataPath(path string) (name string, dev bool, ok bool) {
	rest, found := strings.CutPrefix(path, p2Prefix)
	if !found {
		return "", false, false
	}
	rest, found = strings.CutSuffix(rest, ".json")
	if !found {
		return "", false, false
	}
	if trimmed, isDev := strings.CutSuffix(rest, devSuffix); isDev {
		rest, dev = trimmed, true
	}
	// A valid Composer package name is exactly "vendor/package".
	if strings.Count(rest, "/") != 1 || strings.HasPrefix(rest, "/") || strings.HasSuffix(rest, "/") {
		return "", false, false
	}
	return rest, dev, true
}

func writeJSON(w http.ResponseWriter, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		httpError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

func httpError(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
