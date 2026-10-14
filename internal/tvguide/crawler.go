package tvguide

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	DefaultBaseURL   = "https://www.gatotv.com/canal"
	DefaultUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	DefaultTimeout   = 3 * time.Second
	WorkerCount      = 4
)

var (
	reProgramTitle = regexp.MustCompile(`(?si)<div[^>]*class="[^"]*div_program_title_on_channel[^"]*"[^>]*>.*?<span>(.*?)</span>`)
	reProgramsCol  = regexp.MustCompile(`(?si)<div[^>]*class="[^"]*tbl_EPG_ProgramsColumn[^"]*"[^>]*>(.*?)</div>`)
	reSpan         = regexp.MustCompile(`(?si)<span>(.*?)</span>`)
)

// GatoTVCrawler scrapes Colombian TV guide listings from Gatotv.com.
type GatoTVCrawler struct {
	httpClient *http.Client
	baseURL    string
	userAgent  string
}

// NewCrawler creates a new crawler instance.
func NewCrawler(client *http.Client, baseURL string) *GatoTVCrawler {
	if client == nil {
		client = &http.Client{
			Timeout: DefaultTimeout,
		}
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &GatoTVCrawler{
		httpClient: client,
		baseURL:    strings.TrimRight(baseURL, "/"),
		userAgent:  DefaultUserAgent,
	}
}

// ParseHTML extracts program titles from the Gatotv channel HTML.
func ParseHTML(htmlContent string) []string {
	seen := make(map[string]bool)
	var titles []string

	addTitle := func(raw string) {
		t := strings.TrimSpace(html.UnescapeString(raw))
		if t != "" && !seen[t] {
			seen[t] = true
			titles = append(titles, t)
		}
	}

	// 1. Primary extraction via div_program_title_on_channel <span>
	matches := reProgramTitle.FindAllStringSubmatch(htmlContent, -1)
	for _, m := range matches {
		if len(m) > 1 {
			addTitle(m[1])
		}
	}

	// 2. Secondary fallback via tbl_EPG_ProgramsColumn <span> if none found
	if len(titles) == 0 {
		cols := reProgramsCol.FindAllStringSubmatch(htmlContent, -1)
		for _, col := range cols {
			if len(col) > 1 {
				spans := reSpan.FindAllStringSubmatch(col[1], 1)
				if len(spans) > 0 && len(spans[0]) > 1 {
					addTitle(spans[0][1])
				}
			}
		}
	}

	return titles
}

// FetchChannel retrieves and parses the day's programming for a single channel slug.
func (c *GatoTVCrawler) FetchChannel(ctx context.Context, slug string) ([]string, error) {
	endpoint := fmt.Sprintf("%s/%s", c.baseURL, slug)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for %s: %w", slug, err)
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed for %s: %w", slug, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream gatotv returned status %d for %s", resp.StatusCode, slug)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response for %s: %w", slug, err)
	}

	titles := ParseHTML(string(body))
	return titles, nil
}

// Crawl fetches all channel targets concurrently while preserving target priority order.
func (c *GatoTVCrawler) Crawl(ctx context.Context, targets []ChannelTarget) ([]ChannelGuide, error) {
	if len(targets) == 0 {
		return nil, nil
	}

	var wg sync.WaitGroup
	results := make([]*ChannelGuide, len(targets))

	sem := make(chan struct{}, WorkerCount)

	for i, target := range targets {
		wg.Add(1)
		go func(idx int, tgt ChannelTarget) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			reqCtx, cancel := context.WithTimeout(ctx, DefaultTimeout)
			defer cancel()

			programs, err := c.FetchChannel(reqCtx, tgt.Slug)
			if err != nil || len(programs) == 0 {
				return
			}

			results[idx] = &ChannelGuide{
				Name:      tgt.Name,
				Slug:      tgt.Slug,
				StreamURL: tgt.StreamURL,
				Programs:  programs,
			}
		}(i, target)
	}

	wg.Wait()

	guides := make([]ChannelGuide, 0, len(targets))
	for _, res := range results {
		if res != nil {
			guides = append(guides, *res)
		}
	}

	return guides, nil
}
