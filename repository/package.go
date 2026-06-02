package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	packagist "github.com/shyim/go-packagist"
)

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
