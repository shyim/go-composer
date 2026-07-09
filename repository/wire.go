package repository

import (
	"encoding/json"
	"strings"
)

// RootFile is the packages.json document at the root of a Composer V2
// repository. Clients parse it to discover a repository's endpoints and
// capabilities; servers emit it to advertise them.
//
// The presence of MetadataURL (the "metadata-url" key, a template containing
// "%package%") marks a V2 protocol repository.
type RootFile struct {
	MetadataURL              string                     `json:"metadata-url,omitempty"`
	AvailablePackages        []string                   `json:"available-packages,omitempty"`
	AvailablePackagePatterns []string                   `json:"available-package-patterns,omitempty"`
	SearchURL                string                     `json:"search,omitempty"`
	ListURL                  string                     `json:"list,omitempty"`
	NotifyBatch              string                     `json:"notify-batch,omitempty"`
	Packages                 map[string]json.RawMessage `json:"packages,omitempty"`
	SecurityAdvisories       *SecurityAdvisoriesConfig  `json:"security-advisories,omitempty"`
}

// SecurityAdvisoriesConfig is the "security-advisories" section of a RootFile.
// APIURL, when set, is a (possibly relative) URL accepting a POST of package
// names; Metadata reports whether advisories are embedded in the per-package
// metadata files.
type SecurityAdvisoriesConfig struct {
	Metadata bool   `json:"metadata,omitempty"`
	APIURL   string `json:"api-url,omitempty"`
}

// Metadata is a p2 metadata document: the versions of one or more packages
// keyed by name under Packages, each value being a JSON array of version
// objects. When Minified is "composer/2.0" those arrays are delta-encoded and
// must be passed through Expand (the client does this automatically).
//
// SecurityAdvisories, when present, is the list of (often partial) advisories
// for the package whose metadata this document describes. Composer emits a
// bare list here — not a map keyed by package name.
type Metadata struct {
	Minified           string                     `json:"minified,omitempty"`
	Packages           map[string]json.RawMessage `json:"packages"`
	SecurityAdvisories []SecurityAdvisory         `json:"security-advisories,omitempty"`
}

// knowsPackage reports whether name is covered by the repository's advertised
// available-packages / available-package-patterns list. When the repository
// advertises no such list, it is treated as lazy and every name is allowed.
func (r *RootFile) knowsPackage(name string) bool {
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
