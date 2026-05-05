package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"wallet_service/internal/server_utils"
)

func (c *Client) Initiate(ctx context.Context, req InitiateRequest) (any, error) {
	// Response shape depends on `reason`. Caller chooses which fields to inspect.
	// - wallet topup: { transaction_id, checkout_url }
	// - fare/refund initiate path: { transaction_id, tx_ref }
	var raw map[string]any
	_, err := c.doJSON(ctx, http.MethodPost, "/initiate", req, &raw)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func (c *Client) InitiateTopup(ctx context.Context, req InitiateRequest) (InitiateTopupResponse, error) {
	var out InitiateTopupResponse
	_, err := c.doJSON(ctx, http.MethodPost, "/initiate", req, &out)
	return out, err
}

func (c *Client) Transfer(ctx context.Context, req TransferRequest) (TransferResponse, error) {
	var out TransferResponse
	_, err := c.doJSON(ctx, http.MethodPost, "/transfers", req, &out)
	return out, err
}

func (c *Client) ListTransactions(ctx context.Context, query url.Values) (TransactionsListResponse, error) {
	var out TransactionsListResponse
	p := "/transactions"
	if len(query) > 0 {
		p = fmt.Sprintf("%s?%s", p, query.Encode())
	}
	_, err := c.doJSON(ctx, http.MethodGet, p, nil, &out)
	return out, err
}

func (c *Client) GetChapaBanks(ctx context.Context) (ChapaBanksResponse, error) {
	var out ChapaBanksResponse
	_, err := c.doJSON(ctx, http.MethodGet, "/banks/chapa", nil, &out)
	return out, err
}

func (c *Client) InitiateWithdrawal(ctx context.Context, req WithdrawalRequest) (WithdrawalResponse, error) {
	var out WithdrawalResponse
	_, err := c.doJSON(ctx, http.MethodPost, "/withdrawals", req, &out)
	return out, err
}

// GetReceipt fetches `GET /receipts/:id`.
// Note: payment service may respond with 302 redirects or 200 HTML; callers can inspect status/body/headers.
func (c *Client) GetReceipt(ctx context.Context, transactionID string) (*http.Response, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.newURL("/receipts/"+transactionID), nil)
	if err != nil {
		return nil, nil, err
	}
	if rid := server_utils.RequestIDFromContext(ctx); rid != "" {
		req.Header.Set("X-Request-ID", rid)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		var er server_utils.ErrorResponse
		_ = json.Unmarshal(b, &er)
		return resp, b, &APIError{StatusCode: resp.StatusCode, Message: er.Message}
	}
	return resp, b, nil
}
