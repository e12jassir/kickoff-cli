package model

import (
	"fmt"
	"net/url"
	"strings"
)

// SanitizeURL validates URL safety, enforcing http/https schemes, non-empty host,
// and no embedded credentials (userinfo).
func SanitizeURL(rawURL string) (string, error) {
	if strings.TrimSpace(rawURL) == "" {
		return "", ErrInvalidURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("%w: unsupported scheme %s", ErrInvalidURL, parsed.Scheme)
	}

	if parsed.User != nil {
		return "", fmt.Errorf("%w: embedded credentials not allowed", ErrInvalidURL)
	}

	return parsed.String(), nil
}
