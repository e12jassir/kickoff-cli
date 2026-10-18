package extractor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/e12jassir/kickoff-cli/internal/model"
)

var (
	// Regex patterns to capture HLS streams (.m3u8) in HTML/JavaScript source code:
	// Matches quoted URLs ending with .m3u8, or player source parameters:
	// source: "https://.../live.m3u8", file: 'https://...', src="..."
	reM3U8Explicit = regexp.MustCompile(`(?i)(https?:\/\/[^"'\s\\]+?\.m3u8(?:\?[^"'\s\\]*)?)`)
	reM3U8Relative = regexp.MustCompile(`(?i)["']([^"'\s\\]+?\.m3u8(?:\?[^"'\s\\]*)?)["']`)

	// Common stream iframe patterns
	reIframeSrc = regexp.MustCompile(`(?i)<iframe[^>]+src=["']([^"']+)["']`)

	// Clappr / JWPlayer / VideoJS source parameters
	rePlayerSource = regexp.MustCompile(`(?i)(?:source|file|src)\s*:\s*["']([^"']+)["']`)
)

// DefaultProbeTimeout bounds the HLS header check.
const DefaultProbeTimeout = 2000 * time.Millisecond

// ResolveManifest fetches the player page and extracts the underlying HLS manifest URL and needed headers.
func (s *Service) ResolveManifest(ctx context.Context, stream *ExtractedStream) error {
	if stream == nil || stream.PlayerURL == "" {
		return fmt.Errorf("extractor: empty player url")
	}

	cleanURL, err := model.SanitizeURL(stream.PlayerURL)
	if err != nil {
		return fmt.Errorf("extractor: sanitize player url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cleanURL, nil)
	if err != nil {
		return fmt.Errorf("extractor: build request: %w", err)
	}

	req.Header.Set("User-Agent", s.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Referer", cleanURL)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("extractor: fetch player: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Errorf("extractor: unexpected status %d from %s", resp.StatusCode, cleanURL)
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit for player HTML
	if err != nil {
		return fmt.Errorf("extractor: read player html: %w", err)
	}
	body := string(bodyBytes)

	// Step 1: Check direct .m3u8 in page
	m3u8URL := extractM3U8(body, cleanURL)
	if m3u8URL != "" {
		stream.StreamURL = m3u8URL
		if stream.Headers == nil {
			stream.Headers = make(map[string]string)
		}
		stream.Headers["User-Agent"] = s.userAgent
		stream.Headers["Referer"] = cleanURL
		return nil
	}

	// Step 2: Check if there is an embedded iframe player (e.g. embed.html, player.php)
	iframeSrc := extractIframe(body, cleanURL)
	if iframeSrc != "" && iframeSrc != cleanURL {
		// Follow iframe once
		iframeReq, err := http.NewRequestWithContext(ctx, http.MethodGet, iframeSrc, nil)
		if err == nil {
			iframeReq.Header.Set("User-Agent", s.userAgent)
			iframeReq.Header.Set("Referer", cleanURL)

			iframeResp, err := s.client.Do(iframeReq)
			if err == nil {
				defer iframeResp.Body.Close()
				if iframeResp.StatusCode >= 200 && iframeResp.StatusCode < 400 {
					iframeBytes, _ := io.ReadAll(io.LimitReader(iframeResp.Body, 1<<20))
					iframeBody := string(iframeBytes)
					m3u8FromIframe := extractM3U8(iframeBody, iframeSrc)
					if m3u8FromIframe != "" {
						stream.StreamURL = m3u8FromIframe
						if stream.Headers == nil {
							stream.Headers = make(map[string]string)
						}
						stream.Headers["User-Agent"] = s.userAgent
						stream.Headers["Referer"] = iframeSrc
						return nil
					}
				}
			}
		}
	}

	return fmt.Errorf("extractor: no viable .m3u8 manifest found in %s", cleanURL)
}

// extractM3U8 looks for explicit or relative .m3u8 URLs in raw HTML/JS content.
func extractM3U8(content, pageURL string) string {
	// 1. Direct explicit absolute match
	if m := reM3U8Explicit.FindStringSubmatch(content); len(m) > 1 {
		return cleanExtractedURL(m[1])
	}

	// 2. Player config match (source: "...")
	if matches := rePlayerSource.FindAllStringSubmatch(content, -1); len(matches) > 0 {
		for _, m := range matches {
			if len(m) > 1 && strings.Contains(strings.ToLower(m[1]), ".m3u8") {
				return resolveURL(m[1], pageURL)
			}
		}
	}

	// 3. Relative .m3u8 match
	if m := reM3U8Relative.FindStringSubmatch(content); len(m) > 1 {
		return resolveURL(m[1], pageURL)
	}

	return ""
}

// extractIframe locates iframe sources within HTML content.
func extractIframe(content, pageURL string) string {
	matches := reIframeSrc.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		if len(m) > 1 {
			src := strings.TrimSpace(m[1])
			if src != "" && !strings.HasPrefix(src, "about:") && !strings.HasPrefix(src, "javascript:") {
				return resolveURL(src, pageURL)
			}
		}
	}
	return ""
}

// resolveURL resolves relative links against a base page URL.
func resolveURL(raw, base string) string {
	raw = cleanExtractedURL(raw)
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return raw
	}
	relURL, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return baseURL.ResolveReference(relURL).String()
}

func cleanExtractedURL(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.ReplaceAll(raw, `\/`, `/`)
	raw = strings.ReplaceAll(raw, `\`, "")
	return raw
}

// ProbeHLS performs an ultra-lightweight HTTP Range GET probe (0-512 bytes)
// to verify that the manifest exists, is reachable, and starts with #EXTM3U.
func (s *Service) ProbeHLS(ctx context.Context, stream *ExtractedStream) bool {
	if stream == nil || stream.StreamURL == "" {
		return false
	}

	cleanURL, err := model.SanitizeURL(stream.StreamURL)
	if err != nil {
		return false
	}

	probeCtx, cancel := context.WithTimeout(ctx, DefaultProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, cleanURL, nil)
	if err != nil {
		return false
	}

	// Apply headers (User-Agent, Referer, Origin)
	if stream.Headers != nil {
		for k, v := range stream.Headers {
			req.Header.Set(k, v)
		}
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", s.userAgent)
	}
	req.Header.Set("Range", "bytes=0-512")

	resp, err := s.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return false
	}

	buf := make([]byte, 512)
	n, _ := io.ReadFull(resp.Body, buf)
	bodyStr := string(buf[:n])

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	isStreamType := strings.Contains(contentType, "application/vnd.apple.mpegurl") ||
		strings.Contains(contentType, "application/x-mpegurl") ||
		strings.Contains(contentType, "audio/mpegurl") ||
		strings.Contains(contentType, "video/mp2t")

	if strings.Contains(bodyStr, "#EXTM3U") || isStreamType {
		stream.Alive = true
		return true
	}

	return false
}
