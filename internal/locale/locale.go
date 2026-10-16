// Package locale resolves the timezone used to display match times to the user.
//
// Both the ESPN client and the TV guide (tvguide) rely on this package so that a
// single timezone governs when a match "belongs" to a given day. If the two ever
// disagreed, the day boundary would shift and a match could be shown on the wrong
// day, so this is the one source of truth for display time.
//
// Timezone precedence:
//
//  1. The KICKOFF_TZ environment variable, when set to a valid IANA name
//     (for example "Europe/Madrid"). This is the only configuration knob; there
//     is no CLI flag for it.
//  2. America/Bogota, when KICKOFF_TZ is unset (or empty). Colombia is the
//     project's default audience.
//  3. A fixed UTC-5 zone, when even America/Bogota cannot be loaded. Minimal
//     container images or distroless builds often ship without the IANA tzdata
//     database, so the binary must keep working instead of failing to render
//     times at all.
//
// An invalid KICKOFF_TZ value never aborts the program: the resolver falls back
// to the default and writes a single warning line to stderr.
package locale

import (
	"fmt"
	"os"
	"sync"
	"time"
)

const (
	// EnvVar is the environment variable that overrides the display timezone.
	// It accepts an IANA timezone name such as "Europe/Madrid" or "America/Bogota".
	// When unset or empty, the default zone is used.
	EnvVar = "KICKOFF_TZ"

	// defaultTZ is the timezone used when EnvVar is not set.
	defaultTZ = "America/Bogota"

	// fallbackOffset is the UTC offset in seconds used when the IANA database is
	// unavailable, chosen to match the default zone (Colombia Standard Time).
	fallbackOffset = -5 * 3600
)

var (
	once   sync.Once
	cached *time.Location

	warnOnce sync.Once
)

// Location returns the timezone used to display match times. The result is
// resolved once and cached for the lifetime of the process.
func Location() *time.Location {
	once.Do(func() {
		cached = resolve()
	})
	return cached
}

// resolve performs the timezone lookup on every call, so it can be exercised
// directly in tests with different environment values while Location() keeps its
// process-wide sync.Once cache. resolve never returns nil.
func resolve() *time.Location {
	if name := os.Getenv(EnvVar); name != "" {
		if loc, err := time.LoadLocation(name); err == nil {
			return loc
		}
		warnOnce.Do(func() {
			fmt.Fprintf(os.Stderr, "kickoff: invalid %s=%q, falling back to %s\n", EnvVar, name, defaultTZ)
		})
	}

	if loc, err := time.LoadLocation(defaultTZ); err == nil {
		return loc
	}

	// The tzdata database may be missing in minimal images; keep a fixed UTC-5
	// zone so the binary never breaks while displaying times.
	return time.FixedZone("COT", fallbackOffset)
}
