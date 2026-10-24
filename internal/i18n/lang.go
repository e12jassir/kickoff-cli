package i18n

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// EnvVar is the environment variable that selects the interface language. It
// always wins over the configuration file and the locale variables.
const EnvVar = "KICKOFF_LANG"

// localeVars are consulted in order when neither EnvVar nor the configuration
// file selects a language. Only their two-letter prefix counts.
var localeVars = []string{"LC_ALL", "LC_MESSAGES", "LANG"}

var (
	langOnce   sync.Once
	cachedLang Lang

	// warnOnce guards the single invalid-value warning per process. It is held
	// through a pointer so tests can install a fresh once without copying a lock.
	warnOnce = &sync.Once{}

	// warnOut is the destination of the single invalid-value warning. It is a
	// package variable instead of a direct os.Stderr reference so the warning
	// rule can be asserted without swapping the process file descriptor.
	warnOut io.Writer = os.Stderr
)

// ParseLang extracts a supported language from a locale-style value.
//
// Only the two-letter prefix counts, so "es_CO.UTF-8" resolves to Spanish and
// "en_US.UTF-8" to English. Values outside the supported set — including the
// "C" and "POSIX" placeholders and three-letter codes such as "spa" — are
// rejected with ok == false. ParseLang is pure: rejecting a value never warns,
// because the caller is the one that knows where the value came from.
func ParseLang(raw string) (Lang, bool) {
	value := strings.TrimSpace(raw)
	if separator := strings.IndexAny(value, "_.-@"); separator >= 0 {
		value = value[:separator]
	}

	switch strings.ToLower(value) {
	case string(ES):
		return ES, true
	case string(EN):
		return EN, true
	default:
		return "", false
	}
}

// Language returns the language resolved for the process. The result is cached
// for the lifetime of the process with sync.Once; ResolveLanguage is the uncached
// entry point used by tests and by code that must observe the environment live.
func Language() Lang {
	langOnce.Do(func() {
		cachedLang = ResolveLanguage()
	})
	return cachedLang
}

// SetLanguage overrides the cached process language and updates Default().
func SetLanguage(l Lang) {
	cachedLang = l
	langOnce.Do(func() {})
}

// ResolveLanguage walks the closed precedence chain on every call, so it stays
// testable with different environment values:
//
//  1. KICKOFF_LANG
//  2. ~/.config/kickoff/config.json ({"lang": "es"})
//  3. LC_ALL, then LC_MESSAGES, then LANG (prefix only, only es/en)
//  4. Spanish
//
// An unusable explicit value never aborts the program: resolution keeps walking
// the chain and a single warning line reaches stderr (see warnInvalid).
func ResolveLanguage() Lang {
	lang, bad := resolveLanguage()
	if bad.source != "" {
		warnInvalid(bad.source, bad.value, lang)
	}
	return lang
}

// invalidValue records the first unusable value met while resolving, so the
// warning can name the source and the value that were rejected.
type invalidValue struct {
	source string
	value  string
}

// resolveLanguage performs the lookup without the process-wide cache and without
// warning, so ResolveLanguage owns the warning and tests can exercise the chain
// in isolation.
func resolveLanguage() (Lang, invalidValue) {
	var bad invalidValue

	if raw := os.Getenv(EnvVar); strings.TrimSpace(raw) != "" {
		if lang, ok := ParseLang(raw); ok {
			return lang, bad
		}
		bad = invalidValue{source: EnvVar, value: raw}
	}

	if raw, ok := readConfigLanguage(); ok {
		if lang, ok := ParseLang(raw); ok {
			return lang, bad
		}
		if bad.source == "" {
			bad = invalidValue{source: configFileName, value: raw}
		}
	}

	for _, name := range localeVars {
		raw := os.Getenv(name)
		if raw == "" {
			continue
		}
		if lang, ok := ParseLang(raw); ok {
			return lang, bad
		}
		if bad.source == "" {
			bad = invalidValue{source: name, value: raw}
		}
	}

	return ES, bad
}

// warnInvalid reports an unusable language value at most once per process,
// mirroring internal/locale: the reader learns why their setting was ignored,
// and resolution continues with the fallback instead of aborting.
func warnInvalid(source, value string, fallback Lang) {
	warnOnce.Do(func() {
		_, _ = fmt.Fprintf(warnOut, "kickoff: invalid %s=%q, falling back to %s\n", source, value, fallback)
	})
}
