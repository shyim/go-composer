package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// SecurityAdvisory describes a known vulnerability affecting a package, as
// returned by the security-advisories metadata or API endpoint.
//
// Full advisories (from the API) typically include Title, Sources, ReportedAt
// and optionally CVE/Link/Severity. Partial advisories (embedded in package
// metadata when security-advisories.metadata is true) may only carry
// AdvisoryID and AffectedVersions.
type SecurityAdvisory struct {
	AdvisoryID         string           `json:"advisoryId,omitempty"`
	PackageName        string           `json:"packageName,omitempty"`
	RemoteID           string           `json:"remoteId,omitempty"`
	Title              string           `json:"title,omitempty"`
	Link               string           `json:"link,omitempty"`
	CVE                string           `json:"cve,omitempty"`
	AffectedVersions   string           `json:"affectedVersions,omitempty"`
	Source             string           `json:"source,omitempty"`
	ReportedAt         string           `json:"reportedAt,omitempty"`
	ComposerRepository string           `json:"composerRepository,omitempty"`
	Severity           string           `json:"severity,omitempty"`
	Sources            []AdvisorySource `json:"sources,omitempty"`
}

// AdvisorySource is one provenance entry for a security advisory (e.g. GitHub
// Advisory Database, FriendsOfPHP/security-advisories).
type AdvisorySource struct {
	Name     string `json:"name,omitempty"`
	RemoteID string `json:"remoteId,omitempty"`
}

// Advisories is the result of GetSecurityAdvisories: known advisories keyed by
// package name. Methods on this type narrow the set to concrete installs; the
// unfiltered map is left intact for callers that want everything.
type Advisories map[string][]SecurityAdvisory

// ConstraintCheck reports whether version satisfies the given Composer
// constraint expression. Callers typically implement this with a semver engine
// (for example github.com/shyim/go-version) so this package stays
// dependency-free.
//
//	check := func(constraint, ver string) bool {
//	    v, err := version.NewVersion(strings.TrimPrefix(ver, "v"))
//	    if err != nil {
//	        return false
//	    }
//	    cs, err := version.NewConstraint(constraint)
//	    if err != nil {
//	        return false
//	    }
//	    return cs.Check(v)
//	}
type ConstraintCheck func(constraint, version string) bool

// Package returns the advisories for name, or nil when none are known.
// Lookup is case-insensitive (Composer package names are).
func (a Advisories) Package(name string) []SecurityAdvisory {
	if len(a) == 0 || name == "" {
		return nil
	}
	if list, ok := a[name]; ok {
		return list
	}
	for k, list := range a {
		if strings.EqualFold(k, name) {
			return list
		}
	}
	return nil
}

// AffectingPackage returns the advisories for name that cover version according
// to check. A nil result means no matching advisory (not an error). See
// SecurityAdvisory.Affects for constraint syntax and check semantics.
func (a Advisories) AffectingPackage(name, version string, check ConstraintCheck) []SecurityAdvisory {
	list := a.Package(name)
	if len(list) == 0 || check == nil {
		return nil
	}
	var matching []SecurityAdvisory
	for _, adv := range list {
		if adv.Affects(version, check) {
			matching = append(matching, adv)
		}
	}
	return matching
}

// Affecting returns a new Advisories containing, for each package in versions,
// only the advisories that cover that package's installed version. Packages
// without a matching advisory are omitted. check must be non-nil; a nil check
// yields an empty result.
func (a Advisories) Affecting(versions map[string]string, check ConstraintCheck) Advisories {
	out := Advisories{}
	if len(a) == 0 || len(versions) == 0 || check == nil {
		return out
	}
	for name, ver := range versions {
		if matching := a.AffectingPackage(name, ver, check); len(matching) > 0 {
			out[name] = matching
		}
	}
	return out
}

// Len returns the total number of advisories across all packages.
func (a Advisories) Len() int {
	n := 0
	for _, list := range a {
		n += len(list)
	}
	return n
}

// Affects reports whether targetVersion is covered by this advisory's
// AffectedVersions. Packagist (and Composer) use "|" to separate OR branches;
// each branch is an AND of comma-separated constraints (e.g.
// ">=6.7.0.0,<6.7.8.1"). "||" is accepted as an alias for "|".
//
// check performs the actual version comparison for one branch; if check is nil
// or AffectedVersions/targetVersion is empty, Affects returns false. A leading
// "v" on targetVersion is stripped before calling check.
func (a SecurityAdvisory) Affects(targetVersion string, check ConstraintCheck) bool {
	if check == nil || a.AffectedVersions == "" || targetVersion == "" {
		return false
	}
	targetVersion = strings.TrimPrefix(targetVersion, "v")

	for _, branch := range splitAffectedBranches(a.AffectedVersions) {
		if check(branch, targetVersion) {
			return true
		}
	}
	return false
}

// splitAffectedBranches splits an AffectedVersions string into OR branches.
// Empty branches are dropped. "|" and "||" are both treated as OR separators
// (go-version and Composer both accept either form).
func splitAffectedBranches(affected string) []string {
	// Normalize "||" first so a single Split on "|" works.
	affected = strings.ReplaceAll(affected, "||", "|")
	parts := strings.Split(affected, "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// advisoriesResponse is the shape of the security-advisories API response.
type advisoriesResponse struct {
	Advisories map[string][]SecurityAdvisory `json:"advisories"`
}

// GetSecurityAdvisories returns known advisories for the given packages as an
// Advisories value (keyed by package name). It prefers the repository's
// security-advisories api-url (a single POST), falling back to per-package
// metadata when only the metadata flag is advertised. Repositories with no
// advisory support return an empty Advisories.
//
// The result is unrestricted by version — call Advisories.Affecting or
// Advisories.AffectingPackage (with a ConstraintCheck) to narrow it to
// concrete installs.
func (c *Client) GetSecurityAdvisories(ctx context.Context, packages []string) (Advisories, error) {
	r, err := c.loadRoot(ctx)
	if err != nil {
		return nil, err
	}

	result := Advisories{}
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
				// Ensure PackageName is populated when the API omits it.
				for i := range list {
					if list[i].PackageName == "" {
						list[i].PackageName = name
					}
				}
				result[name] = list
			}
		}
		return result, nil
	}

	// metadata-only: advisories are embedded in each package's metadata file as
	// a list under "security-advisories" (partial: typically advisoryId +
	// affectedVersions only). Prefer the stable well-known metadata file.
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
			// Per-package p2 files use a list; tolerate a missing key.
			var file struct {
				SecurityAdvisories []SecurityAdvisory `json:"security-advisories"`
			}
			if err := json.Unmarshal(body, &file); err != nil {
				return nil, fmt.Errorf("parsing %s: %w", reqURL, err)
			}
			if len(file.SecurityAdvisories) == 0 {
				continue
			}
			for i := range file.SecurityAdvisories {
				if file.SecurityAdvisories[i].PackageName == "" {
					file.SecurityAdvisories[i].PackageName = name
				}
			}
			result[name] = file.SecurityAdvisories
		}
	}

	return result, nil
}
