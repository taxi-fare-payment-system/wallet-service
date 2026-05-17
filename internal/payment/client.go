package payment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	"wallet_service/internal/server_utils"
)

type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
}

func NewClient(baseURL string, httpClient *http.Client) (*Client, error) {
	if httpClient == nil {
		return nil, errors.New("http client is required")
	}
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return nil, fmt.Errorf("parse base url: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid base url: %q", baseURL)
	}
	return &Client{baseURL: u, httpClient: httpClient}, nil
}

type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("payment service error: status=%d", e.StatusCode)
	}
	return fmt.Sprintf("payment service error: status=%d message=%q", e.StatusCode, e.Message)
}

func (c *Client) newURL(p string) string {
	pathPart, rawQuery, _ := strings.Cut(p, "?")
	u := *c.baseURL
	u.Path = path.Join(strings.TrimRight(u.Path, "/"), pathPart)
	u.RawQuery = rawQuery
	return u.String()
}

func (c *Client) newURLWithQuery(p string, query url.Values) string {
	u := *c.baseURL
	u.Path = path.Join(strings.TrimRight(u.Path, "/"), p)
	if len(query) > 0 {
		u.RawQuery = query.Encode()
	}
	return u.String()
}

func (c *Client) doJSON(ctx context.Context, method, p string, in any, out any) (*http.Response, error) {
	return c.doJSONQuery(ctx, method, p, nil, in, out)
}

func (c *Client) doJSONQuery(ctx context.Context, method, p string, query url.Values, in any, out any) (*http.Response, error) {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.newURLWithQuery(p, query), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if rid := server_utils.RequestIDFromContext(ctx); rid != "" {
		req.Header.Set("X-Request-ID", rid)
	}
	if uid := server_utils.TrustUserIDFromContext(ctx); uid != "" {
		req.Header.Set("X-User-ID", uid)
	}
	if role := server_utils.TrustUserRoleFromContext(ctx); role != "" {
		req.Header.Set("X-User-Role", role)
	}
	if auth := server_utils.AuthBearerFromContext(ctx); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		var er server_utils.ErrorResponse
		_ = json.NewDecoder(resp.Body).Decode(&er)
		return resp, &APIError{StatusCode: resp.StatusCode, Message: er.Message}
	}

	if out != nil {
		defer resp.Body.Close()
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp, err
		}
	}

	return resp, nil
}
