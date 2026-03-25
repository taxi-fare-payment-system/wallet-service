package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"wallet_service/internal/server_utils"
)

type Client struct {
	baseURL    *url.URL
	verifyPath string
	httpClient *http.Client
}

func NewClient(baseURL, verifyPath string, httpClient *http.Client) (*Client, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, errors.New("auth service base url is required")
	}
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
	if strings.TrimSpace(verifyPath) == "" {
		return nil, errors.New("verify path is required")
	}
	return &Client{baseURL: u, verifyPath: verifyPath, httpClient: httpClient}, nil
}

// VerifyAdmin checks whether the given user is an admin.
// Since the auth service spec is not present in this repo, wallet treats any 2xx response as admin=true
// and any non-2xx response as admin=false.
func (c *Client) VerifyAdmin(ctx context.Context, userID int64) (bool, error) {
	u := *c.baseURL
	u.Path = strings.TrimRight(u.Path, "/") + c.verifyPath

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return false, err
	}
	q := req.URL.Query()
	q.Set("user_id", fmt.Sprintf("%d", userID))
	req.URL.RawQuery = q.Encode()

	if rid := server_utils.RequestIDFromContext(ctx); rid != "" {
		req.Header.Set("X-Request-ID", rid)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300, nil
}
