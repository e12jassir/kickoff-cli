package locale

import (
	"testing"
	"time"
)

// resolve is tested directly (instead of Location) because Location caches its
// result with sync.Once: varying the environment between assertions is only
// possible when each call re-reads it through resolve.

func TestResolve_ValidEnvTimezone(t *testing.T) {
	t.Setenv(EnvVar, "Europe/Madrid")

	loc := resolve()
	if loc == nil {
		t.Fatal("resolve returned nil for a valid timezone")
	}
	if loc.String() != "Europe/Madrid" {
		t.Errorf("expected Europe/Madrid, got %s", loc.String())
	}
}

func TestResolve_InvalidEnvTimezoneFallsBack(t *testing.T) {
	t.Setenv(EnvVar, "Not/AZone")

	loc := resolve()
	if loc == nil {
		t.Fatal("resolve returned nil for an invalid timezone")
	}

	// The fallback must match the default zone when tzdata is available.
	if def, err := time.LoadLocation(defaultTZ); err == nil {
		if define, got := def.String(), loc.String(); define != got {
			t.Errorf("expected fallback to %s, got %s", define, got)
		}
	}
}

func TestResolve_UnsetEnvTimezoneUsesDefault(t *testing.T) {
	// An empty value is treated the same as an unset variable.
	t.Setenv(EnvVar, "")

	loc := resolve()
	if loc == nil {
		t.Fatal("resolve returned nil with no override")
	}

	if def, err := time.LoadLocation(defaultTZ); err == nil {
		if define, got := def.String(), loc.String(); define != got {
			t.Errorf("expected default %s, got %s", define, got)
		}
	}
}

// TestBogotaLocation_IsFixedCOT pins the source timezone used to interpret
// schedules published in Colombia. It must be a fixed UTC-5 zone so the
// resolution path never depends on the host tzdata database.
func TestBogotaLocation_IsFixedCOT(t *testing.T) {
	loc := BogotaLocation()
	if loc == nil {
		t.Fatal("BogotaLocation returned nil")
	}
	if got := loc.String(); got != "COT" {
		t.Errorf("expected zone name COT, got %q", got)
	}

	for _, at := range []time.Time{
		time.Date(2026, time.January, 15, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC),
	} {
		if _, offset := at.In(loc).Zone(); offset != -5*3600 {
			t.Errorf("expected a constant UTC-5 offset at %s, got %d seconds", at, offset)
		}
	}
}

func TestLocation_CachesResult(t *testing.T) {
	first := Location()
	if first == nil {
		t.Fatal("Location returned nil")
	}
	if second := Location(); first != second {
		t.Errorf("Location did not return the cached pointer: %p != %p", first, second)
	}
}
