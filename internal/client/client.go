// Package client provides a SerpAPI client for the three Google Trends engines.
// Auth is read exclusively from environment variables SERPAPI_API_KEY or SERPAPI_KEY.
// The key value is never logged or returned by any public method.
package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	DefaultBaseURL = "https://serpapi.com"
	EnvKeyPrimary  = "SERPAPI_API_KEY"
	EnvKeyAlias    = "SERPAPI_KEY"
	EnvBaseURL     = "SERPAPI_BASE_URL"
)

// Client is the shared SerpAPI Trends client.
type Client struct {
	baseURL    string
	httpClient *http.Client
	// key is private; never exported or logged.
	key string
}

// NewFromEnv creates a Client using env vars only. Returns error if no key is present.
func NewFromEnv() (*Client, error) {
	key := os.Getenv(EnvKeyPrimary)
	if key == "" {
		key = os.Getenv(EnvKeyAlias)
	}
	if key == "" {
		return nil, fmt.Errorf("missing API key: set %s or %s", EnvKeyPrimary, EnvKeyAlias)
	}
	base := os.Getenv(EnvBaseURL)
	if base == "" {
		base = DefaultBaseURL
	}
	return &Client{
		baseURL: strings.TrimRight(base, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		key: key,
	}, nil
}

// KeyFingerprint returns the first 4 characters of the key for doctor output only.
func (c *Client) KeyFingerprint() string {
	if len(c.key) < 4 {
		return "****"
	}
	return c.key[:4] + "…"
}

// HasKey reports whether a key is loaded (for doctor).
func (c *Client) HasKey() bool {
	return c.key != ""
}

// Params is a generic map for query parameters.
type Params map[string]string

// Search performs a GET against /search with the given engine and params.
// api_key is injected automatically and never appears in the returned Params echo beyond search_parameters.
func (c *Client) Search(engine string, params Params) (map[string]interface{}, error) {
	u, err := url.Parse(c.baseURL + "/search")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("engine", engine)
	q.Set("api_key", c.key)
	for k, v := range params {
		if v != "" {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "serpapi-trends-cli/0.1")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("json decode: %w (http %d)", err, resp.StatusCode)
	}

	// Surface HTTP-level errors into a consistent form when possible.
	if resp.StatusCode == 401 {
		return result, fmt.Errorf("auth: invalid or missing API key (HTTP 401)")
	}
	if resp.StatusCode == 429 {
		return result, fmt.Errorf("rate-limit: quota or hourly throughput exceeded (HTTP 429)")
	}
	if resp.StatusCode == 400 {
		if e, ok := result["error"].(string); ok {
			return result, fmt.Errorf("input: %s (HTTP 400)", e)
		}
		return result, fmt.Errorf("input: bad request (HTTP 400)")
	}
	if resp.StatusCode >= 500 {
		return result, fmt.Errorf("api-error: SerpAPI server (HTTP %d)", resp.StatusCode)
	}
	if status, _ := result["search_metadata"].(map[string]interface{})["status"].(string); status == "Error" {
		if e, ok := result["error"].(string); ok {
			return result, fmt.Errorf("api-error: %s", e)
		}
	}
	return result, nil
}

// Autocomplete is a convenience for engine=google_trends_autocomplete.
func (c *Client) Autocomplete(q, hl string, noCache bool) (map[string]interface{}, error) {
	p := Params{"q": q}
	if hl != "" {
		p["hl"] = hl
	}
	if noCache {
		p["no_cache"] = "true"
	}
	return c.Search("google_trends_autocomplete", p)
}

// TrendingNow is a convenience for engine=google_trends_trending_now.
func (c *Client) TrendingNow(geo, hours, categoryID string, onlyActive, noCache bool) (map[string]interface{}, error) {
	p := Params{}
	if geo != "" {
		p["geo"] = geo
	}
	if hours != "" {
		p["hours"] = hours
	}
	if categoryID != "" {
		p["category_id"] = categoryID
	}
	if onlyActive {
		p["only_active"] = "true"
	}
	if noCache {
		p["no_cache"] = "true"
	}
	return c.Search("google_trends_trending_now", p)
}

// Trends is a convenience for engine=google_trends with arbitrary data_type.
func (c *Client) Trends(q, dataType, geo, date, region, gprop, cat, tz, hl string, includeLow, noCache bool) (map[string]interface{}, error) {
	p := Params{"q": q}
	if dataType != "" {
		p["data_type"] = dataType
	}
	if geo != "" {
		p["geo"] = geo
	}
	if date != "" {
		p["date"] = date
	}
	if region != "" {
		p["region"] = region
	}
	if gprop != "" {
		p["gprop"] = gprop
	}
	if cat != "" {
		p["cat"] = cat
	}
	if tz != "" {
		p["tz"] = tz
	}
	if hl != "" {
		p["hl"] = hl
	}
	if includeLow {
		p["include_low_search_volume"] = "true"
	}
	if noCache {
		p["no_cache"] = "true"
	}
	return c.Search("google_trends", p)
}

// Account hits the free /account.json endpoint.
func (c *Client) Account() (map[string]interface{}, error) {
	u := c.baseURL + "/account.json?api_key=" + url.QueryEscape(c.key)
	resp, err := c.httpClient.Get(u)
	if err != nil {
		return nil, fmt.Errorf("network: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	if resp.StatusCode == 401 {
		return result, fmt.Errorf("auth: invalid API key (HTTP 401)")
	}
	return result, nil
}
