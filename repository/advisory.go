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
