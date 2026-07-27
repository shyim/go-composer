package repository

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// get performs a GET, attaching auth headers for the request origin, and
// returns the body and status code.
func (c *Client) get(ctx context.Context, reqURL string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, 0, err
	}
	return c.do(req)
}

// post performs an application/x-www-form-urlencoded POST.
func (c *Client) post(ctx context.Context, reqURL string, form url.Values) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(req)
}

func (c *Client) do(req *http.Request) ([]byte, int, error) {
	c.applyAuth(req)

	resp, err := c.client().Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	const maxBodySize = 50 * 1024 * 1024 // 50MB limit
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

// applyAuth attaches credentials from the associated auth.json that match the
// request's origin (host), following Composer's auth header conventions.
func (c *Client) applyAuth(req *http.Request) {
	if c.auth == nil {
		return
	}
	host := req.URL.Host
	hostname := req.URL.Hostname()

	getMapKey := func(check func(key string) bool) string {
		if host != "" && check(host) {
			return host
		}
		if hostname != "" && hostname != host && check(hostname) {
			return hostname
		}
		return ""
	}

	if k := getMapKey(func(k string) bool { _, ok := c.auth.HTTPBasicAuth[k]; return ok }); k != "" {
		basic := c.auth.HTTPBasicAuth[k]
		token := base64.StdEncoding.EncodeToString([]byte(basic.Username + ":" + basic.Password))
		req.Header.Set("Authorization", "Basic "+token)
		return
	}
	if k := getMapKey(func(k string) bool { _, ok := c.auth.BearerAuth[k]; return ok }); k != "" {
		token := c.auth.BearerAuth[k]
		req.Header.Set("Authorization", "Bearer "+token)
		return
	}
	if k := getMapKey(func(k string) bool { _, ok := c.auth.GithubOAuth[k]; return ok }); k != "" {
		token := c.auth.GithubOAuth[k]
		req.Header.Set("Authorization", "token "+token)
		return
	}
	if k := getMapKey(func(k string) bool { _, ok := c.auth.GitlabOAuth[k]; return ok }); k != "" {
		tok := c.auth.GitlabOAuth[k]
		req.Header.Set("Authorization", "Bearer "+tok.Token)
		return
	}
	if k := getMapKey(func(k string) bool { _, ok := c.auth.GitlabAuth[k]; return ok }); k != "" {
		tok := c.auth.GitlabAuth[k]
		req.Header.Set("PRIVATE-TOKEN", tok.Token)
		return
	}
	if k := getMapKey(func(k string) bool { _, ok := c.auth.CustomHeaders[k]; return ok }); k != "" {
		headers := c.auth.CustomHeaders[k]
		for _, h := range headers {
			if name, value, found := strings.Cut(h, ":"); found {
				req.Header.Add(strings.TrimSpace(name), strings.TrimSpace(value))
			}
		}
	}
}
