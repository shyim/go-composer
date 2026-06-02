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

	body, err := io.ReadAll(resp.Body)
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

	if basic, ok := c.auth.HTTPBasicAuth[host]; ok {
		token := base64.StdEncoding.EncodeToString([]byte(basic.Username + ":" + basic.Password))
		req.Header.Set("Authorization", "Basic "+token)
		return
	}
	if token, ok := c.auth.BearerAuth[host]; ok {
		req.Header.Set("Authorization", "Bearer "+token)
		return
	}
	if token, ok := c.auth.GithubOAuth[host]; ok {
		req.Header.Set("Authorization", "token "+token)
		return
	}
	if tok, ok := c.auth.GitlabOAuth[host]; ok {
		req.Header.Set("Authorization", "Bearer "+tok.Token)
		return
	}
	if tok, ok := c.auth.GitlabAuth[host]; ok {
		req.Header.Set("PRIVATE-TOKEN", tok.Token)
		return
	}
	if headers, ok := c.auth.CustomHeaders[host]; ok {
		for _, h := range headers {
			if name, value, found := strings.Cut(h, ":"); found {
				req.Header.Add(strings.TrimSpace(name), strings.TrimSpace(value))
			}
		}
	}
}
