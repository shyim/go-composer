package repository

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// RootFile is the packages.json document at the root of a Composer V2
// repository. Clients parse it to discover a repository's endpoints and
// capabilities; servers emit it to advertise them.
//
// The presence of MetadataURL (the "metadata-url" key, a template containing
// "%package%") marks a V2 protocol repository.
type RootFile struct {
	MetadataURL              string                    `json:"metadata-url,omitempty"`
	AvailablePackages        []string                  `json:"available-packages,omitempty"`
	AvailablePackagePatterns []string                  `json:"available-package-patterns,omitempty"`
	SearchURL                string                    `json:"search,omitempty"`
	ListURL                  string                    `json:"list,omitempty"`
	NotifyBatch              string                    `json:"notify-batch,omitempty"`
	Packages                 InlinePackages            `json:"packages,omitempty"`
	SecurityAdvisories       *SecurityAdvisoriesConfig `json:"security-advisories,omitempty"`
}

// InlinePackages is the root packages.json "packages" object: a map of package
// name → raw version list/object. Packagist.org (and some other V2 roots) emit
// an empty JSON array "[]" here as a V1 compatibility stub; that is treated as
// no inline catalog (nil map), not a parse error.
type InlinePackages map[string]json.RawMessage

// UnmarshalJSON accepts either a JSON object (the real catalog) or an empty
// JSON array (legacy / V1 stub). A non-empty array is rejected.
func (p *InlinePackages) UnmarshalJSON(data []byte) error {
	if p == nil {
		return fmt.Errorf("packages: UnmarshalJSON on nil InlinePackages pointer")
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		*p = nil
		return nil
	}
	switch data[0] {
	case '{':
		var m map[string]json.RawMessage
		if err := json.Unmarshal(data, &m); err != nil {
			return err
		}
		// Distinguish omitted/null (nil) from an explicit empty object.
		if m == nil {
			m = map[string]json.RawMessage{}
		}
		*p = InlinePackages(m)
		return nil
	case '[':
		var arr []json.RawMessage
		if err := json.Unmarshal(data, &arr); err != nil {
			return err
		}
		if len(arr) != 0 {
			return fmt.Errorf("packages: expected object or empty array, got array of length %d", len(arr))
		}
		// Empty array means "no inline packages" — leave as nil so listing
		// correctly reports ErrListingNotSupported for lazy V2 repos.
		*p = nil
		return nil
	default:
		return fmt.Errorf("packages: expected object or empty array, got %s", summarizeJSONKind(data))
	}
}

func summarizeJSONKind(data []byte) string {
	if len(data) == 0 {
		return "empty"
	}
	switch data[0] {
	case '"':
		return "string"
	case 't', 'f':
		return "boolean"
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '-':
		return "number"
	default:
		return string(data[0])
	}
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
	if r == nil {
		return true
	}
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
