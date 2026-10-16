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

func TestLocation_CachesResult(t *testing.T) {
	first := Location()
	if first == nil {
		t.Fatal("Location returned nil")
	}
	if second := Location(); first != second {
		t.Errorf("Location did not return the cached pointer: %p != %p", first, second)
	}
}
