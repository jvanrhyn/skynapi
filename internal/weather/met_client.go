package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

const (
	metTimeFormat = time.RFC1123

	// maxResponseBytes caps how much of an upstream response we will read.
	// A locationforecast payload is ~100 KB; this leaves generous headroom
	// while keeping a misbehaving or hijacked upstream from exhausting memory.
	maxResponseBytes = 4 << 20 // 4 MiB
)

// metClient is the production implementation of Client.
type metClient struct {
	httpClient *http.Client
	baseURL    string
	userAgent  string
}

// NewClient returns a Client that calls api.met.no.
// baseURL should be "https://api.met.no/weatherapi/locationforecast/2.0".
// userAgent must include contact information per api.met.no terms.
func NewClient(baseURL, userAgent string) Client {
	return &metClient{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseURL:    baseURL,
		userAgent:  userAgent,
	}
}

func (c *metClient) Fetch(ctx context.Context, lat, lon float64, opts FetchOptions) (*FetchResult, error) {
	url := fmt.Sprintf("%s/compact?lat=%s&lon=%s",
		c.baseURL,
		strconv.FormatFloat(lat, 'f', 4, 64),
		strconv.FormatFloat(lon, 'f', 4, 64),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("weather: build request: %w", err)
	}

	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")

	if opts.IfModifiedSince != nil {
		req.Header.Set("If-Modified-Since", opts.IfModifiedSince.UTC().Format(metTimeFormat))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("weather: fetch request: %w", err)
	}
	// Drain before closing on every path so the connection returns to the pool.
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
		_ = resp.Body.Close()
	}()

	result := &FetchResult{}
	result.ExpiresAt = parseTimeHeader(resp.Header.Get("Expires"), metTimeFormat)
	result.LastModified = parseTimeHeader(resp.Header.Get("Last-Modified"), metTimeFormat)

	switch resp.StatusCode {
	case http.StatusOK:
		raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
		if err != nil {
			return nil, fmt.Errorf("weather: read response: %w", err)
		}
		if len(raw) > maxResponseBytes {
			return nil, fmt.Errorf("weather: upstream response exceeds %d bytes", maxResponseBytes)
		}
		// The body is stored and served verbatim, so verify it is at least
		// well-formed JSON rather than caching garbage behind a JSON content type.
		if !json.Valid(raw) {
			return nil, fmt.Errorf("weather: upstream returned malformed JSON")
		}
		result.Raw = raw

	case http.StatusNotModified:
		result.NotModified = true

	case http.StatusTooManyRequests:
		slog.WarnContext(ctx, "weather: rate limited by upstream", "url", url)
		return nil, fmt.Errorf("weather: upstream rate limit (429)")

	default:
		slog.ErrorContext(ctx, "weather: unexpected upstream status",
			"status", resp.StatusCode, "url", url)
		return nil, fmt.Errorf("weather: upstream returned %d", resp.StatusCode)
	}

	if v := resp.Header.Get("X-Forecast-Version"); v != "" {
		slog.InfoContext(ctx, "weather: upstream forecast version", "version", v)
	}
	if v := resp.Header.Get("Deprecation"); v != "" {
		slog.WarnContext(ctx, "weather: upstream deprecation notice", "header", v)
	}

	return result, nil
}

func parseTimeHeader(s, layout string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(layout, s)
	if err != nil {
		return nil
	}
	return &t
}
