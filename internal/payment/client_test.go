package payment

import (
	"net/url"
	"testing"
)

func TestNewURL_splitsPathAndQuery(t *testing.T) {
	base, err := url.Parse("http://payment:8080")
	if err != nil {
		t.Fatal(err)
	}
	c := &Client{baseURL: base}

	got := c.newURL("/api/v1/payments/transactions?limit=50&wallet_id=abc")
	want := "http://payment:8080/api/v1/payments/transactions?limit=50&wallet_id=abc"
	if got != want {
		t.Fatalf("newURL() = %q, want %q", got, want)
	}
}

func TestNewURLWithQuery_setsRawQuery(t *testing.T) {
	base, err := url.Parse("http://payment:8080")
	if err != nil {
		t.Fatal(err)
	}
	c := &Client{baseURL: base}

	q := url.Values{}
	q.Set("limit", "50")
	q.Set("wallet_id", "abc")

	got := c.newURLWithQuery("/api/v1/payments/transactions", q)
	want := "http://payment:8080/api/v1/payments/transactions?limit=50&wallet_id=abc"
	if got != want {
		t.Fatalf("newURLWithQuery() = %q, want %q", got, want)
	}

	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "/api/v1/payments/transactions" {
		t.Fatalf("Path = %q, want /api/v1/payments/transactions", parsed.Path)
	}
	if parsed.RawQuery == "" {
		t.Fatal("RawQuery is empty")
	}
}
