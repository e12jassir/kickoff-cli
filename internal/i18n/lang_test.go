package i18n

import (
	"bytes"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestParseLang(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want Lang
		ok   bool
	}{
		{"lowercase code", "es", ES, true},
		{"uppercase code", "EN", EN, true},
		{"colombian locale", "es_CO.UTF-8", ES, true},
		{"english regional locale", "en_US.UTF-8", EN, true},
		{"hyphen separated locale", "es-CO", ES, true},
		{"surrounding spaces", "  es  ", ES, true},
		{"c locale", "C", "", false},
		{"c utf8 locale", "C.UTF-8", "", false},
		{"posix locale", "POSIX", "", false},
		{"unsupported language", "fr_FR.UTF-8", "", false},
		{"three letter code", "spa", "", false},
		{"empty value", "", "", false},
		{"spaces only", "   ", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseLang(tc.raw)
			if ok != tc.ok {
				t.Fatalf("ParseLang(%q) ok = %v, want %v", tc.raw, ok, tc.ok)
			}
			if got != tc.want {
				t.Errorf("ParseLang(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// TestResolve_Precedence walks the closed precedence chain:
// KICKOFF_LANG > config.json > LC_ALL > LC_MESSAGES > LANG > es.
func TestResolve_Precedence(t *testing.T) {
	cases := []struct {
		name        string
		kickoffLang string
		configBody  string
		lcAll       string
		lcMessages  string
		lang        string
		want        Lang
	}{
		{
			name:        "kickoff lang wins over config and locale",
			kickoffLang: "en",
			configBody:  `{"lang":"es"}`,
			lcAll:       "es_ES.UTF-8",
			lcMessages:  "es_ES.UTF-8",
			lang:        "es_ES.UTF-8",
			want:        EN,
		},
		{
			name:        "kickoff lang accepts a regional locale",
			kickoffLang: "es_CO.UTF-8",
			lang:        "en_US.UTF-8",
			want:        ES,
		},
		{
			name:       "config wins over every locale variable",
			configBody: `{"lang":"en"}`,
			lcAll:      "es_ES.UTF-8",
			lcMessages: "es_ES.UTF-8",
			lang:       "es_ES.UTF-8",
			want:       EN,
		},
		{
			name:       "lc_all wins over lc_messages and lang",
			lcAll:      "es_ES.UTF-8",
			lcMessages: "en_US.UTF-8",
			lang:       "en_US.UTF-8",
			want:       ES,
		},
		{
			name:       "lc_messages wins over lang",
			lcMessages: "en_US.UTF-8",
			lang:       "es_ES.UTF-8",
			want:       EN,
		},
		{
			name: "lang is the last locale layer",
			lang: "en_US.UTF-8",
			want: EN,
		},
		{
			name: "default when nothing is set",
			want: ES,
		},
		{
			name:        "invalid kickoff lang falls through to config",
			kickoffLang: "fr",
			configBody:  `{"lang":"en"}`,
			want:        EN,
		},
		{
			name:        "invalid kickoff lang falls through to locale",
			kickoffLang: "fr",
			lcAll:       "en_US.UTF-8",
			want:        EN,
		},
		{
			name:       "invalid config language falls through to locale",
			configBody: `{"lang":"fr"}`,
			lang:       "en_US.UTF-8",
			want:       EN,
		},
		{
			name:       "malformed config is ignored",
			configBody: `{not json`,
			lang:       "en_US.UTF-8",
			want:       EN,
		},
		{
			name:       "config without a language is ignored",
			configBody: `{}`,
			lang:       "es_ES.UTF-8",
			want:       ES,
		},
		{
			name:       "c and posix locales fall back to the default",
			lcAll:      "C",
			lcMessages: "POSIX",
			lang:       "C.UTF-8",
			want:       ES,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, configDir := isolateConfigHome(t)
			if tc.configBody != "" {
				writeConfigBody(t, configDir, tc.configBody)
			}

			setLanguageEnv(t, EnvVar, tc.kickoffLang)
			setLanguageEnv(t, "LC_ALL", tc.lcAll)
			setLanguageEnv(t, "LC_MESSAGES", tc.lcMessages)
			setLanguageEnv(t, "LANG", tc.lang)

			if got := ResolveLanguage(); got != tc.want {
				t.Errorf("ResolveLanguage() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestInvalidLangWarnsOnce pins the single stderr warning: a broken value is
// reported once per process and resolution keeps going.
func TestInvalidLangWarnsOnce(t *testing.T) {
	_, _ = isolateConfigHome(t)
	setLanguageEnv(t, EnvVar, "")
	setLanguageEnv(t, "LC_ALL", "")
	setLanguageEnv(t, "LC_MESSAGES", "")
	setLanguageEnv(t, "LANG", "")

	var warned bytes.Buffer
	restore := captureWarnings(&warned)
	defer restore()

	t.Setenv(EnvVar, "fr")

	if got := ResolveLanguage(); got != ES {
		t.Fatalf("ResolveLanguage() = %q, want the %q fallback", got, ES)
	}
	if got := ResolveLanguage(); got != ES {
		t.Fatalf("second ResolveLanguage() = %q, want the %q fallback", got, ES)
	}
	// A different invalid value must not produce a second line either.
	t.Setenv(EnvVar, "de")
	_ = ResolveLanguage()

	output := warned.String()
	if output == "" {
		t.Fatal("expected one warning on stderr, got none")
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected exactly one warning line, got %d: %q", len(lines), output)
	}
	if !strings.Contains(lines[0], `invalid KICKOFF_LANG="fr"`) {
		t.Errorf("warning does not name the rejected value: %q", lines[0])
	}
	if !strings.Contains(lines[0], "falling back to es") {
		t.Errorf("warning does not name the fallback: %q", lines[0])
	}
}

// TestInvalidConfigLanguageWarnsOnce covers the config layer of the same rule.
func TestInvalidConfigLanguageWarnsOnce(t *testing.T) {
	_, configDir := isolateConfigHome(t)
	writeConfigBody(t, configDir, `{"lang":"fr"}`)
	setLanguageEnv(t, EnvVar, "")
	setLanguageEnv(t, "LC_ALL", "")
	setLanguageEnv(t, "LC_MESSAGES", "")
	setLanguageEnv(t, "LANG", "")

	var warned bytes.Buffer
	restore := captureWarnings(&warned)
	defer restore()

	if got := ResolveLanguage(); got != ES {
		t.Fatalf("ResolveLanguage() = %q, want %q", got, ES)
	}

	output := warned.String()
	if !strings.Contains(output, `invalid config.json="fr"`) {
		t.Errorf("warning does not name the config file value: %q", output)
	}
	if count := strings.Count(strings.TrimSpace(output), "\n") + 1; count != 1 {
		t.Errorf("expected exactly one warning line, got %d: %q", count, output)
	}
}

// TestInvalidLocaleVariableWarnsOnce covers the locale layer of the same rule.
func TestInvalidLocaleVariableWarnsOnce(t *testing.T) {
	_, _ = isolateConfigHome(t)
	setLanguageEnv(t, EnvVar, "")
	setLanguageEnv(t, "LC_ALL", "")
	setLanguageEnv(t, "LC_MESSAGES", "")
	t.Setenv("LANG", "C")

	var warned bytes.Buffer
	restore := captureWarnings(&warned)
	defer restore()

	if got := ResolveLanguage(); got != ES {
		t.Fatalf("ResolveLanguage() = %q, want %q", got, ES)
	}

	output := warned.String()
	if !strings.Contains(output, `invalid LANG="C"`) {
		t.Errorf("warning does not name the rejected locale: %q", output)
	}
	if count := strings.Count(strings.TrimSpace(output), "\n") + 1; count != 1 {
		t.Errorf("expected exactly one warning line, got %d: %q", count, output)
	}
}

// TestLanguageCachesResult proves the process-wide accessor resolves once: the
// environment changes between the two calls and the answer must not.
func TestLanguageCachesResult(t *testing.T) {
	_, _ = isolateConfigHome(t)
	setLanguageEnv(t, "LC_ALL", "")
	setLanguageEnv(t, "LC_MESSAGES", "")
	setLanguageEnv(t, "LANG", "")

	t.Setenv(EnvVar, "en")
	first := Language()

	t.Setenv(EnvVar, "es")
	second := Language()

	if first != second {
		t.Errorf("Language() returned %q then %q; the result must be cached", first, second)
	}
}

// TestResolveLanguageIsLive proves ResolveLanguage is not cached, so the
// precedence chain stays testable across environment changes.
func TestResolveLanguageIsLive(t *testing.T) {
	_, _ = isolateConfigHome(t)
	setLanguageEnv(t, "LC_ALL", "")
	setLanguageEnv(t, "LC_MESSAGES", "")
	setLanguageEnv(t, "LANG", "")

	t.Setenv(EnvVar, "en")
	if got := ResolveLanguage(); got != EN {
		t.Fatalf("ResolveLanguage() = %q, want %q", got, EN)
	}

	t.Setenv(EnvVar, "es")
	if got := ResolveLanguage(); got != ES {
		t.Fatalf("ResolveLanguage() = %q, want %q", got, ES)
	}
}

// TestLanguageIndependentOfTimezone covers S10: KICKOFF_TZ never influences the
// language, and language resolution never rewrites the timezone variables.
func TestLanguageIndependentOfTimezone(t *testing.T) {
	timezones := []string{"Europe/Madrid", "America/Bogota", "Not/AZone", ""}

	for _, tz := range timezones {
		t.Run("tz="+tz, func(t *testing.T) {
			_, _ = isolateConfigHome(t)
			setLanguageEnv(t, EnvVar, "")
			setLanguageEnv(t, "LC_ALL", "")
			setLanguageEnv(t, "LC_MESSAGES", "")
			setLanguageEnv(t, "LANG", "en_US.UTF-8")

			t.Setenv("KICKOFF_TZ", tz)

			if got := ResolveLanguage(); got != EN {
				t.Errorf("ResolveLanguage() = %q with KICKOFF_TZ=%q, want %q", got, tz, EN)
			}
			if got := os.Getenv("KICKOFF_TZ"); got != tz {
				t.Errorf("language resolution changed KICKOFF_TZ to %q, want %q", got, tz)
			}
		})
	}
}

// setLanguageEnv sets a variable for the test, treating an empty value the same
// way the resolver does: absent.
func setLanguageEnv(t *testing.T, name, value string) {
	t.Helper()
	t.Setenv(name, value)
}

// captureWarnings redirects the package warning sink to buf for the duration of
// one test and resets the process-wide once so the assertion is deterministic.
func captureWarnings(buf *bytes.Buffer) func() {
	previousOut := warnOut
	previousOnce := warnOnce

	warnOut = buf
	warnOnce = &sync.Once{}

	return func() {
		warnOut = previousOut
		warnOnce = previousOnce
	}
}
