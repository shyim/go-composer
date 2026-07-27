package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

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
	clear(resp.Results[len(out):])
	return out, nil
}
