package trip

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
	baseURL      *url.URL
	validatePath string
	httpClient   *http.Client
}

func NewClient(baseURL, validatePath string, httpClient *http.Client) (*Client, error) {
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
	if strings.TrimSpace(validatePath) == "" {
		return nil, errors.New("validate path is required")
	}
	return &Client{
		baseURL:      u,
		validatePath: validatePath,
		httpClient:   httpClient,
	}, nil
}

// ValidateTripMembership calls the configured trip validation endpoint.
// Since the trip service spec is not present in this repo, wallet treats any 2xx response as valid
// and any non-2xx as invalid.
func (c *Client) ValidateTripMembership(ctx context.Context, tripID string, passengerUserID, driverUserID int64) error {
	u := *c.baseURL
	u.Path = strings.TrimRight(u.Path, "/") + c.validatePath

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	q := req.URL.Query()
	q.Set("trip_id", strings.TrimSpace(tripID))
	q.Set("passenger_user_id", fmt.Sprintf("%d", passengerUserID))
	q.Set("driver_user_id", fmt.Sprintf("%d", driverUserID))
	req.URL.RawQuery = q.Encode()

	if rid := server_utils.RequestIDFromContext(ctx); rid != "" {
		req.Header.Set("X-Request-ID", rid)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("trip validation failed: status=%d", resp.StatusCode)
}
