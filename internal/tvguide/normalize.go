package tvguide

import (
	"html"
	"regexp"
	"strings"
	"unicode"

	"github.com/e12jassir/kickoff-cli/internal/model"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

var (
	reNonAlphaNum = regexp.MustCompile(`[^\p{L}\p{N}]+`)
	reTokensFC    = regexp.MustCompile(`\b(fc|cf|f\.c\.|c\.f\.)\b`)
	reWhitespace  = regexp.MustCompile(`\s+`)
)

// accentTransformer strips diacritical marks (e.g., á->a, ñ->n, ü->u, ç->c).
var accentTransformer = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// RemoveAccents removes diacritical marks from UTF-8 strings.
func RemoveAccents(s string) string {
	res, _, err := transform.String(accentTransformer, s)
	if err != nil {
		return s
	}
	return res
}

// Normalize normalizes a team name, fixture title, or TV guide program entry:
// 1. Unescapes HTML entities (&amp;, etc.)
// 2. Removes diacritical accents
// 3. Converts to lowercase
// 4. Strips dots
// 5. Strips "FC", "CF" acronyms
// 6. Replaces non-alphanumeric punctuation with space and collapses whitespace
func Normalize(s string) string {
	s = html.UnescapeString(s)
	s = RemoveAccents(s)
	s = strings.ToLower(s)
	// Strip dots directly so C.F. becomes cf, S.K. becomes sk
	s = strings.ReplaceAll(s, ".", "")
	// Strip FC / CF acronyms
	s = reTokensFC.ReplaceAllString(s, " ")
	// Replace remaining non-alphanumeric chars with space
	s = reNonAlphaNum.ReplaceAllString(s, " ")
	return strings.TrimSpace(reWhitespace.ReplaceAllString(s, " "))
}

// Stopwords are common sports/regional words that shouldn't match on their own as a single token.
var stopwords = map[string]bool{
	"de": true, "del": true, "la": true, "el": true, "los": true, "las": true,
	"san": true, "santa": true, "van": true, "von": true, "sur": true, "der": true,
	"the": true, "and": true, "und": true, "vs": true, "en": true, "y": true,
	"club": true, "cd": true, "ca": true, "ac": true, "sc": true, "fk": true, "afc": true,
	"deportivo": true, "atletico": true, "sporting": true, "racing": true,
	"united": true, "city": true, "real": true, "inter": true, "union": true,
	"balompie": true,
}

// ExtractKeywords returns distinctive words from a normalized team name.
func ExtractKeywords(normName string) []string {
	words := strings.Fields(normName)
	var keywords []string
	for _, w := range words {
		if len(w) < 3 {
			continue
		}
		if stopwords[w] {
			continue
		}
		keywords = append(keywords, w)
	}
	return keywords
}

// MatchScore evaluates how well a TV guide title matches a soccer match.
// Returns:
//
//	matched: whether a genuine match is found
//	score: 10 (title match), 5 (both teams matched), 2 (single team matched), 0 (no match)
func MatchScore(guideTitle string, match model.Match) (bool, int) {
	gNorm := Normalize(guideTitle)
	if gNorm == "" {
		return false, 0
	}

	hNorm := Normalize(match.HomeTeam.DisplayName)
	aNorm := Normalize(match.AwayTeam.DisplayName)
	tNorm := Normalize(match.Title)

	// 1. Full fixture title match (e.g. "LASK vs Liverpool")
	if tNorm != "" {
		if strings.Contains(gNorm, tNorm) || strings.Contains(tNorm, gNorm) {
			return true, 10
		}
	}

	// 2. Direct containment of full team names
	hInGuide := hNorm != "" && strings.Contains(gNorm, hNorm)
	aInGuide := aNorm != "" && strings.Contains(gNorm, aNorm)

	// 3. Keyword-level matching for distinctive team names
	hKeywords := ExtractKeywords(hNorm)
	aKeywords := ExtractKeywords(aNorm)

	gWords := strings.Fields(gNorm)
	tokenMatches := func(keywords []string) bool {
		for _, kw := range keywords {
			for _, gw := range gWords {
				if gw == kw {
					return true
				}
				if len(kw) >= 4 && len(gw) >= 4 {
					if strings.HasPrefix(gw, kw) || strings.HasPrefix(kw, gw) {
						return true
					}
				}
			}
		}
		return false
	}

	hMatch := hInGuide || tokenMatches(hKeywords)
	aMatch := aInGuide || tokenMatches(aKeywords)

	if hMatch && aMatch {
		return true, 5
	}
	if hMatch || aMatch {
		return true, 2
	}

	return false, 0
}
