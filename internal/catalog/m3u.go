package catalog

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/e12jassir/kickoff/internal/tvguide"
)

// M3UChannel represents a single parsed channel from an M3U / M3U8 playlist.
type M3UChannel struct {
	Name    string `json:"name"`
	TvgName string `json:"tvg_name"`
	TvgID   string `json:"tvg_id"`
	Group   string `json:"group"`
	URL     string `json:"url"`
}

var (
	reAttr         = regexp.MustCompile(`([a-zA-Z0-9_\-]+)=(?:"([^"]*)"|'([^']*)'|([^\s,]+))`)
	reSeparateNum1 = regexp.MustCompile(`([a-zA-Z]+)(\d+)`)
	reSeparateNum2 = regexp.MustCompile(`(\d+)([a-zA-Z]+)`)
	reDigits       = regexp.MustCompile(`\b\d+\b`)
)

func splitExtInf(line string) (attrsPart, title string) {
	content := strings.TrimPrefix(line, "#EXTINF:")

	inQuote := false
	var quoteChar rune
	commaIdx := -1
	for i, r := range content {
		if !inQuote && (r == '"' || r == '\'') {
			inQuote = true
			quoteChar = r
		} else if inQuote && r == quoteChar {
			inQuote = false
		} else if !inQuote && r == ',' {
			commaIdx = i
			break
		}
	}

	if commaIdx != -1 {
		attrsPart = strings.TrimSpace(content[:commaIdx])
		title = strings.TrimSpace(content[commaIdx+1:])
	} else {
		attrsPart = strings.TrimSpace(content)
		title = ""
	}
	return attrsPart, title
}

// ParseM3U parses M3U/M3U8 content from an io.Reader and extracts valid channels with sanitized URLs.
func ParseM3U(r io.Reader) ([]M3UChannel, error) {
	scanner := bufio.NewScanner(r)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var channels []M3UChannel
	var pending *M3UChannel

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "#EXTINF:") {
			attrsPart, title := splitExtInf(line)
			ch := &M3UChannel{
				Name: title,
			}

			matches := reAttr.FindAllStringSubmatch(attrsPart, -1)
			for _, m := range matches {
				if len(m) < 5 {
					continue
				}
				key := strings.ToLower(m[1])
				val := m[2]
				if val == "" {
					val = m[3]
				}
				if val == "" {
					val = m[4]
				}
				switch key {
				case "tvg-name":
					ch.TvgName = val
				case "tvg-id":
					ch.TvgID = val
				case "group-title":
					ch.Group = val
				}
			}

			pending = ch
			continue
		}

		if strings.HasPrefix(line, "#EXTGRP:") && pending != nil {
			if pending.Group == "" {
				pending.Group = strings.TrimSpace(strings.TrimPrefix(line, "#EXTGRP:"))
			}
			continue
		}

		// Skip other directives or comments starting with '#'
		if strings.HasPrefix(line, "#") {
			continue
		}

		// Non-comment line is the stream URL
		if pending != nil {
			cleanURL, err := SanitizeURL(line)
			if err == nil && cleanURL != "" {
				pending.URL = cleanURL
				if pending.Name == "" {
					if pending.TvgName != "" {
						pending.Name = pending.TvgName
					} else if pending.TvgID != "" {
						pending.Name = pending.TvgID
					} else {
						pending.Name = "Channel"
					}
				}
				if pending.TvgName == "" {
					pending.TvgName = pending.Name
				}
				channels = append(channels, *pending)
			}
			pending = nil
		}
	}

	if err := scanner.Err(); err != nil {
		return channels, fmt.Errorf("error reading M3U stream: %w", err)
	}

	return channels, nil
}

// ParseM3UFile reads and parses an M3U playlist file from a local path (supporting '~' expansion).
func ParseM3UFile(path string) ([]M3UChannel, error) {
	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("could not determine user home directory: %w", err)
		}
		path = filepath.Join(home, path[2:])
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("could not open M3U file %s: %w", path, err)
	}
	defer f.Close()

	return ParseM3U(f)
}

var noiseWords = map[string]bool{
	"hd": true, "fhd": true, "uhd": true, "4k": true, "sd": true,
	"hevc": true, "1080p": true, "720p": true, "live": true, "en": true,
	"vivo": true, "tv": true, "canal": true, "deportes": true, "sports": true,
	"co": true, "colombia": true, "ar": true, "argentina": true, "mx": true,
	"mexico": true, "cl": true, "chile": true, "pe": true, "peru": true,
	"sur": true, "norte": true, "latam": true, "latino": true, "stream": true,
}

func cleanForMatching(s string) string {
	s = strings.ReplaceAll(s, "+", " plus ")
	s = reSeparateNum1.ReplaceAllString(s, "$1 $2")
	s = reSeparateNum2.ReplaceAllString(s, "$1 $2")
	norm := tvguide.Normalize(s)

	// Expand common channel abbreviations
	words := strings.Fields(norm)
	var expanded []string
	for i := 0; i < len(words); i++ {
		w := words[i]
		switch w {
		case "fs":
			expanded = append(expanded, "fox", "sports")
		case "rmtv":
			expanded = append(expanded, "real", "madrid", "tv")
		case "tdp":
			expanded = append(expanded, "teledeporte")
		case "cbssn":
			expanded = append(expanded, "cbs", "sports", "network")
		default:
			expanded = append(expanded, w)
		}
	}
	return strings.Join(expanded, " ")
}

func extractCoreTokens(norm string) (core []string, digits []string, hasPlus bool) {
	fields := strings.Fields(norm)
	for _, f := range fields {
		if f == "plus" {
			hasPlus = true
		}
		if reDigits.MatchString(f) {
			digits = append(digits, f)
			continue
		}
		if !noiseWords[f] && len(f) > 1 {
			core = append(core, f)
		}
	}
	return core, digits, hasPlus
}

func scoreNameMatch(broadcastNorm string, candidateRaw string) int {
	if strings.TrimSpace(candidateRaw) == "" {
		return -1
	}
	candNorm := cleanForMatching(candidateRaw)
	if candNorm == "" {
		return -1
	}

	// Exact match bonus
	if broadcastNorm == candNorm {
		return 100
	}

	bCore, bDigits, bPlus := extractCoreTokens(broadcastNorm)
	cCore, cDigits, cPlus := extractCoreTokens(candNorm)

	// Digit conflict check:
	// If broadcast specifies a digit (e.g. ESPN 2 -> "2"), candidate must not have a conflicting digit.
	if len(bDigits) > 0 {
		matchDigit := false
		for _, bd := range bDigits {
			for _, cd := range cDigits {
				if bd == cd {
					matchDigit = true
					break
				}
			}
		}
		if len(cDigits) > 0 && !matchDigit {
			return -1
		}
	} else {
		// Broadcast has NO digits (e.g. "ESPN", "Caracol", "Canal RCN").
		// If candidate has a digit like "2", "3", "4", "5", "6", "7", reject!
		// (Exception: "1" often denotes primary channel, so "ESPN 1" can match "ESPN")
		for _, cd := range cDigits {
			if cd != "1" {
				return -1
			}
		}
	}

	score := 0

	// Plus check (e.g. Win+ vs Win Sports)
	if bPlus {
		if cPlus {
			score += 30
		} else {
			return -1 // Broadcast specifically requested plus channel!
		}
	} else {
		if cPlus {
			score -= 20 // Broadcast did not ask for plus channel
		} else {
			score += 10
		}
	}

	// Substring matches
	if strings.Contains(candNorm, broadcastNorm) {
		score += 30
	} else if strings.Contains(broadcastNorm, candNorm) {
		score += 20
	}

	// Digits match bonus
	if len(bDigits) > 0 {
		for _, bd := range bDigits {
			for _, cd := range cDigits {
				if bd == cd {
					score += 25
				}
			}
		}
	}

	// Core token matching
	if len(bCore) == 0 {
		return score
	}

	cCoreMap := make(map[string]bool)
	for _, c := range cCore {
		cCoreMap[c] = true
	}

	matchedTokens := 0
	for _, bt := range bCore {
		if cCoreMap[bt] {
			matchedTokens++
			score += 20
		} else {
			// Prefix matching for tokens (e.g. "caracol" matches "caracoltv")
			prefixFound := false
			for ct := range cCoreMap {
				if len(bt) >= 3 && len(ct) >= 3 {
					if strings.HasPrefix(ct, bt) || strings.HasPrefix(bt, ct) {
						matchedTokens++
						score += 15
						prefixFound = true
						break
					}
				}
			}
			if !prefixFound {
				score -= 10
			}
		}
	}

	if matchedTokens == 0 {
		return -1
	}

	return score
}

// MatchM3UChannel finds the best matching channel in channels for a given broadcast signal.
// Returns the matched M3UChannel and true, or an empty M3UChannel and false if no confident match.
func MatchM3UChannel(broadcastName string, channels []M3UChannel) (M3UChannel, bool) {
	bNorm := cleanForMatching(broadcastName)
	if bNorm == "" || len(channels) == 0 {
		return M3UChannel{}, false
	}

	bestScore := -1
	var bestChannel M3UChannel

	for _, ch := range channels {
		scoreName := scoreNameMatch(bNorm, ch.Name)
		scoreTvgName := scoreNameMatch(bNorm, ch.TvgName)
		scoreTvgID := scoreNameMatch(bNorm, ch.TvgID)

		maxChScore := scoreName
		if scoreTvgName > maxChScore {
			maxChScore = scoreTvgName
		}
		if scoreTvgID > maxChScore {
			maxChScore = scoreTvgID
		}

		if maxChScore > bestScore {
			bestScore = maxChScore
			bestChannel = ch
		}
	}

	if bestScore >= 20 {
		return bestChannel, true
	}

	return M3UChannel{}, false
}
