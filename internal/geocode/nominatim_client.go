package geocode

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// maxResponseBytes caps how much of an upstream response we will read. A
// reverse lookup returns a few kilobytes.
const maxResponseBytes = 1 << 20 // 1 MiB

type nominatimClient struct {
	httpClient *http.Client
	baseURL    string
	userAgent  string
}

// NewClient returns a Client backed by a Nominatim instance.
// baseURL should be "https://nominatim.openstreetmap.org".
// userAgent must identify the application per Nominatim's usage policy — this
// is the reason the lookup is proxied rather than made from the browser, which
// cannot set the header at all.
func NewClient(baseURL, userAgent string) Client {
	return &nominatimClient{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseURL:    strings.TrimRight(baseURL, "/"),
		userAgent:  userAgent,
	}
}

// nominatimResponse is the subset of the Nominatim reverse payload we use.
type nominatimResponse struct {
	Address struct {
		City         string `json:"city"`
		Town         string `json:"town"`
		Village      string `json:"village"`
		Municipality string `json:"municipality"`
		County       string `json:"county"`
		State        string `json:"state"`
		Country      string `json:"country"`
		CountryCode  string `json:"country_code"`
	} `json:"address"`
}

func (c *nominatimClient) Reverse(ctx context.Context, lat, lon float64) (*Place, error) {
	q := url.Values{}
	q.Set("format", "json")
	q.Set("lat", strconv.FormatFloat(lat, 'f', 4, 64))
	q.Set("lon", strconv.FormatFloat(lon, 'f', 4, 64))
	q.Set("zoom", "10")
	endpoint := c.baseURL + "/reverse?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("geocode: build request: %w", err)
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("geocode: reverse request: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("geocode: upstream returned %d", resp.StatusCode)
	}

	var body nominatimResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&body); err != nil {
		return nil, fmt.Errorf("geocode: decode response: %w", err)
	}

	a := body.Address
	place := &Place{
		City:        firstNonEmpty(a.City, a.Town, a.Village, a.Municipality, a.County, a.State),
		Country:     a.Country,
		CountryCode: strings.ToUpper(a.CountryCode),
	}
	if place.Country == "" {
		place.Country = place.CountryCode
	}
	place.Label = strings.Join(nonEmpty(place.City, place.Country), ", ")
	if place.Label == "" {
		return nil, fmt.Errorf("geocode: upstream returned no usable place name")
	}
	return place, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func nonEmpty(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
