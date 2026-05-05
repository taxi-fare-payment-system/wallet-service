package trip

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"wallet_service/internal/server_utils"
)

var ErrTripNotActive = errors.New("trip not found or not active")

type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
}

func NewClient(baseURL string, httpClient *http.Client) (*Client, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, errors.New("trip service base url is required")
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
	return &Client{
		baseURL:    u,
		httpClient: httpClient,
	}, nil
}

// ValidateTripActive calls GET /trips/:id?status=ACTIVE (Trip Service).
// A 404 or non-ACTIVE trip body yields ErrTripNotActive.
func (c *Client) ValidateTripActive(ctx context.Context, tripID string) error {
	tripID = strings.TrimSpace(tripID)
	if tripID == "" {
		return ErrTripNotActive
	}
	base := strings.TrimRight(c.baseURL.String(), "/")
	reqURL := base + "/trips/" + url.PathEscape(tripID) + "?status=ACTIVE"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}
	if rid := server_utils.RequestIDFromContext(ctx); rid != "" {
		req.Header.Set("X-Request-ID", rid)
	}
	if auth := server_utils.AuthBearerFromContext(ctx); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return ErrTripNotActive
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("trip service error: status=%d", resp.StatusCode)
	}
	var payload struct {
		Status string `json:"status"`
		Data   *struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	b, _ := io.ReadAll(resp.Body)
	if len(b) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&payload); err != nil {
		return nil
	}
	st := strings.ToUpper(strings.TrimSpace(payload.Status))
	if st == "" && payload.Data != nil {
		st = strings.ToUpper(strings.TrimSpace(payload.Data.Status))
	}
	if st != "" && st != "ACTIVE" {
		return ErrTripNotActive
	}
	return nil
}
