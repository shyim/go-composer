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
// advisories. When implemented, the server advertises and serves a
// security-advisories API endpoint.
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

// NewHandler builds an http.Handler serving the repository protocol for p.
// Search and security-advisory endpoints are wired only when p implements the
// Searcher and Advisor capabilities respectively.
func NewHandler(p Provider, opts ...HandlerOption) http.Handler {
	h := &Handler{
		provider: p,
		isDev:    defaultIsDev,
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
	if _, ok := p.(Advisor); ok {
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

	if _, ok := h.provider.(Advisor); ok {
		root.SecurityAdvisories = &SecurityAdvisoriesConfig{APIURL: advisoriesPath}
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

	writeJSON(w, Metadata{
		Minified: minifiedComposer2,
		Packages: map[string]json.RawMessage{name: raw},
	})
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
