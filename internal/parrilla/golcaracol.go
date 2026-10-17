package parrilla

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// defaultTimeout bounds the whole request/response cycle for a daily page.
	defaultTimeout = 30 * time.Second
	// maxBodyBytes caps how much of the (large) article is read into memory.
	maxBodyBytes = 16 << 20
	// browserUserAgent is a normal desktop UA; the source 403s some bots.
	browserUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)

// golCaracol is the concrete Fetcher backed by the Noticias Caracol article.
type golCaracol struct {
	baseURL   string
	client    *http.Client
	userAgent string
}

// Option customizes a golCaracol fetcher.
type Option func(*golCaracol)

// WithHTTPClient overrides the HTTP client (useful for tests and timeouts).
func WithHTTPClient(client *http.Client) Option {
	return func(g *golCaracol) {
		if client != nil {
			g.client = client
		}
	}
}

// WithBaseURL overrides the base article URL (useful for tests).
func WithBaseURL(baseURL string) Option {
	return func(g *golCaracol) {
		if baseURL != "" {
			g.baseURL = baseURL
		}
	}
}

// WithUserAgent overrides the request User-Agent header.
func WithUserAgent(userAgent string) Option {
	return func(g *golCaracol) {
		if userAgent != "" {
			g.userAgent = userAgent
		}
	}
}

// Fetch downloads and parses the Gol Caracol parrilla for the given date.
func (g *golCaracol) Fetch(ctx context.Context, date time.Time) (Snapshot, error) {
	url := buildURL(g.baseURL, date)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Snapshot{}, fmt.Errorf("parrilla: build request: %w", err)
	}
	req.Header.Set("User-Agent", g.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "es-CO,es;q=0.9,en;q=0.5")

	resp, err := g.client.Do(req)
	if err != nil {
		return Snapshot{}, fmt.Errorf("parrilla: fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Snapshot{}, fmt.Errorf("parrilla: unexpected status %d from %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return Snapshot{}, fmt.Errorf("parrilla: read body: %w", err)
	}

	return ParseHTML(date.Format("2006-01-02"), body)
}
