package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("auth service error: status=%d", e.StatusCode)
	}
	return fmt.Sprintf("auth service error: status=%d message=%q", e.StatusCode, e.Message)
}

type MeResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Data    struct {
		ID          string `json:"id"`
		Phone       string `json:"phone"`
		DisplayName string `json:"display_name"`
	} `json:"data"`
}

// InternalUserContactResponse matches Auth `GET /internal/users/:id/contact` (auth.md).
type InternalUserContactResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Data    struct {
		Phone        string  `json:"phone"`
		Email        *string `json:"email,omitempty"`
		DisplayName  string  `json:"display_name"`
	} `json:"data"`
}

func NewClient(baseURL string, httpClient *http.Client) (*Client, error) {
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
	return &Client{baseURL: u, httpClient: httpClient}, nil
}

func (c *Client) meURL() string {
	u := *c.baseURL
	u.Path = path.Join(strings.TrimRight(u.Path, "/"), "/api/v1/auth/me")
	return u.String()
}

func (c *Client) GetMe(ctx context.Context) (MeResponse, error) {
	var out MeResponse
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.meURL(), nil)
	if err != nil {
		return out, err
	}
	if rid := server_utils.RequestIDFromContext(ctx); rid != "" {
		req.Header.Set("X-Request-ID", rid)
	}
	if auth := server_utils.AuthBearerFromContext(ctx); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var er server_utils.ErrorResponse
		_ = json.NewDecoder(resp.Body).Decode(&er)
		return out, &APIError{StatusCode: resp.StatusCode, Message: er.Message}
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, err
	}
	return out, nil
}

func (c *Client) internalUserContactURL(userID string) string {
	u := *c.baseURL
	u.Path = path.Join(strings.TrimRight(u.Path, "/"), "/internal/users", strings.TrimSpace(userID), "contact")
	return u.String()
}

// GetInternalUserContact returns public contact fields for a user (inter-service; documented in auth.md).
func (c *Client) GetInternalUserContact(ctx context.Context, userID string) (InternalUserContactResponse, error) {
	var out InternalUserContactResponse
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.internalUserContactURL(userID), nil)
	if err != nil {
		return out, err
	}
	if rid := server_utils.RequestIDFromContext(ctx); rid != "" {
		req.Header.Set("X-Request-ID", rid)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var er server_utils.ErrorResponse
		_ = json.NewDecoder(resp.Body).Decode(&er)
		return out, &APIError{StatusCode: resp.StatusCode, Message: er.Message}
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, err
	}
	return out, nil
}

