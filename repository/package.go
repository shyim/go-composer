package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/shyim/go-composer"
)

// Version describes a single released version of a package, as returned by a
// Composer repository's metadata endpoint. It mirrors the version objects found
// under packages[<name>] in a p2 metadata file, which are essentially the
// package's composer.json augmented with version_normalized, dist and source.
type Version struct {
	Name              string                     `json:"name"`
	Version           string                     `json:"version"`
	VersionNormalized string                     `json:"version_normalized,omitempty"`
	Description       string                     `json:"description,omitempty"`
	Type              string                     `json:"type,omitempty"`
	Keywords          []string                   `json:"keywords,omitempty"`
	Homepage          string                     `json:"homepage,omitempty"`
	Time              string                     `json:"time,omitempty"`
	License           *composer.StringOrSlice    `json:"license,omitempty"`
	Authors           []composer.Author          `json:"authors,omitempty"`
	Support           *composer.Support          `json:"support,omitempty"`
	Funding           []composer.Funding         `json:"funding,omitempty"`
	Require           map[string]string          `json:"require,omitempty"`
	RequireDev        map[string]string          `json:"require-dev,omitempty"`
	Conflict          map[string]string          `json:"conflict,omitempty"`
	Replace           map[string]string          `json:"replace,omitempty"`
	Provide           map[string]string          `json:"provide,omitempty"`
	Suggest           map[string]string          `json:"suggest,omitempty"`
	Autoload          *composer.Autoload         `json:"autoload,omitempty"`
	Dist              composer.LockPackageDist   `json:"dist,omitempty"`
	Source            composer.LockPackageSource `json:"source,omitempty"`
	Extra             composer.ExtraData         `json:"extra,omitempty"`
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

	// Packages declared inline in the root file take priority over a lazy
	// metadata-url lookup, matching Composer. Repositories like
	// packages.shopware.com ship their entire catalog this way.
	if raw, ok := lookupInlinePackage(r.Packages, name); ok {
		versions, err := DecodePackageVersions(raw, false)
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

	var file Metadata
	if err := json.Unmarshal(body, &file); err != nil {
		return nil, false, fmt.Errorf("parsing %s: %w", reqURL, err)
	}

	raw, ok := file.Packages[packageName]
	if !ok {
		return nil, false, nil
	}

	versions, err := DecodePackageVersions(raw, file.Minified == minifiedComposer2)
	if err != nil {
		return nil, false, fmt.Errorf("parsing %s: %w", reqURL, err)
	}
	return versions, true, nil
}

// lookupInlinePackage finds packages[name] with a case-insensitive key match.
func lookupInlinePackage(packages InlinePackages, name string) (json.RawMessage, bool) {
	if packages == nil {
		return nil, false
	}
	if raw, ok := packages[name]; ok {
		return raw, true
	}
	lower := strings.ToLower(name)
	if raw, ok := packages[lower]; ok {
		return raw, true
	}
	for k, raw := range packages {
		if strings.EqualFold(k, name) {
			return raw, true
		}
	}
	return nil, false
}

// GetPackages returns every package declared inline in the repository's root
// packages.json "packages" map, keyed by package name. This covers Composer
// repositories that ship their full catalog in the root file (Satis full-dump,
// packages.shopware.com, …). Packages that are only reachable via metadata-url
// are not included — use GetPackage for those.
//
// An empty but present "packages": {} yields an empty map. When the root file
// has no packages map (nil / omitted), GetPackages returns
// ErrListingNotSupported — the repository does not expose a bulk catalog.
// Version values support both the array and the version-keyed map forms
// (see DecodePackageVersions).
func (c *Client) GetPackages(ctx context.Context) (map[string]*Package, error) {
	r, err := c.loadRoot(ctx)
	if err != nil {
		return nil, err
	}
	if r.Packages == nil {
		return nil, ErrListingNotSupported
	}

	out := make(map[string]*Package, len(r.Packages))
	for name, raw := range r.Packages {
		versions, err := DecodePackageVersions(raw, false)
		if err != nil {
			return nil, fmt.Errorf("decoding packages[%q]: %w", name, err)
		}
		if len(versions) == 0 {
			continue
		}
		// Prefer the package name embedded in version objects when present.
		pkgName := name
		if versions[0].Name != "" {
			pkgName = versions[0].Name
		}
		out[pkgName] = &Package{Name: pkgName, Versions: versions}
	}
	return out, nil
}

// PackageNames returns the names of packages the repository advertises. Order
// of preference: available-packages, then keys of the inline root packages
// map. When neither is present the repository is treated as lazy (metadata-url
// only) and PackageNames returns ErrListingNotSupported — individual packages
// must be asked for by name via GetPackage.
func (c *Client) PackageNames(ctx context.Context) ([]string, error) {
	r, err := c.loadRoot(ctx)
	if err != nil {
		return nil, err
	}
	if len(r.AvailablePackages) > 0 {
		return append([]string(nil), r.AvailablePackages...), nil
	}
	if r.Packages == nil {
		return nil, ErrListingNotSupported
	}
	names := make([]string, 0, len(r.Packages))
	for name := range r.Packages {
		names = append(names, name)
	}
	return names, nil
}
