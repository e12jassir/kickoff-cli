// Package locale resolves the timezone used to display match times to the user.
//
// ESPN kickoff timestamps are absolute instants, and the view layer converts
// them to this display timezone through Location. The Colombian schedule
// sources (Gol Caracol and GatoTV) publish wall-clock times instead, so those
// are always interpreted in BogotaLocation regardless of the user's choice.
// Keeping the two concerns separate is what stops the day boundary from
// shifting when a user outside Colombia displays a fixture.
//
// Timezone precedence for display:
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

	// sourceZoneName is the stable name of the fixed source timezone. Colombia
	// has no daylight saving time, so a fixed offset is exact year-round.
	sourceZoneName = "COT"
)

var (
	once   sync.Once
	cached *time.Location

	warnOnce sync.Once

	// sourceLocation is the fixed zone used to interpret schedules published by
	// the Colombian sources (Gol Caracol and GatoTV). It is deliberately a fixed
	// zone instead of a time.LoadLocation call: container images may ship without
	// the IANA tzdata database, and the source schedule must never depend on it.
	sourceLocation = time.FixedZone(sourceZoneName, fallbackOffset)
)

// BogotaLocation returns the fixed Colombia Standard Time (UTC-5) zone used to
// interpret the wall-clock times published by the Colombian schedule sources.
//
// This is not the display timezone. Source times are always anchored here,
// independent of KICKOFF_TZ, and only converted to the user's display zone in
// the view layer. The result is a fixed offset because Colombia does not observe
// daylight saving time, which also keeps resolution working without tzdata.
func BogotaLocation() *time.Location {
	return sourceLocation
}

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
