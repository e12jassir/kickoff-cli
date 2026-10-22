package parrilla

import "testing"

// TestCanonicalChannel_BridgesKnownDisplayVariants pins the explicit bridge
// between the display-name variants that publish the SAME signal.
//
// The sources disagree on the primary ESPN channel ("ESPN" in channelSpecs,
// "ESPN 1 HD" in tvguide.DefaultTargets and in every M3U playlist) and on RCN
// ("RCN TV"/"RCN" in channelSpecs, "Canal RCN HD" in DefaultTargets). Before the
// bridge, the second spelling had no entry, so a caller comparing identities saw
// two different channels and declined as ambiguous instead of choosing.
//
// Both spellings must land on the SAME existing canonical record, never on a new
// key and never on a duplicated record.
func TestCanonicalChannel_BridgesKnownDisplayVariants(t *testing.T) {
	tests := []struct {
		name    string
		a       string
		b       string
		wantKey string
	}{
		{
			name:    "primary ESPN signal: published name vs numbered display variant",
			a:       "ESPN",
			b:       "ESPN 1 HD",
			wantKey: "espn1",
		},
		{
			name:    "primary ESPN signal: published name vs numbered variant without a format label",
			a:       "ESPN",
			b:       "ESPN 1",
			wantKey: "espn1",
		},
		{
			name:    "primary ESPN signal: both display variants agree",
			a:       "ESPN 1",
			b:       "ESPN 1 HD",
			wantKey: "espn1",
		},
		{
			name:    "RCN signal: published name vs GatoTV display name",
			a:       "RCN HD",
			b:       "Canal RCN HD",
			wantKey: "rcn",
		},
		{
			name:    "RCN signal: alias vs unlabelled GatoTV display name",
			a:       "RCN",
			b:       "Canal RCN",
			wantKey: "rcn",
		},
		{
			name:    "RCN signal: both GatoTV display variants agree",
			a:       "Canal RCN",
			b:       "Canal RCN HD",
			wantKey: "rcn",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyA, playA, okA := CanonicalChannel(tt.a)
			keyB, playB, okB := CanonicalChannel(tt.b)

			if !okA {
				t.Fatalf("CanonicalChannel(%q) is unknown, want key %q", tt.a, tt.wantKey)
			}
			if !okB {
				t.Fatalf("CanonicalChannel(%q) is unknown, want key %q", tt.b, tt.wantKey)
			}
			if keyA != tt.wantKey {
				t.Errorf("CanonicalChannel(%q) key = %q, want %q", tt.a, keyA, tt.wantKey)
			}
			if keyB != tt.wantKey {
				t.Errorf("CanonicalChannel(%q) key = %q, want %q", tt.b, keyB, tt.wantKey)
			}
			if keyA != keyB {
				t.Errorf("bridged spellings disagree: %q -> %q, %q -> %q", tt.a, keyA, tt.b, keyB)
			}
			if playA != playB {
				t.Errorf("bridged spellings disagree on playability: %q -> %v, %q -> %v", tt.a, playA, tt.b, playB)
			}

			// The bridge must reuse the canonical record, not duplicate it: the
			// display name is what proves both spellings read the same entry.
			dispA, okDispA := CanonicalDisplay(tt.a)
			dispB, okDispB := CanonicalDisplay(tt.b)
			if !okDispA || !okDispB {
				t.Fatalf("CanonicalDisplay: %q known = %v, %q known = %v, want true for both", tt.a, okDispA, tt.b, okDispB)
			}
			if dispA != dispB {
				t.Errorf("bridged spellings report different displays: %q -> %q, %q -> %q", tt.a, dispA, tt.b, dispB)
			}
		})
	}
}

// TestCanonicalChannel_DoesNotBridgeAdjacentSignals pins the boundary of the
// bridge table: only hand-written entries bridge. Numbered siblings stay
// distinct signals, and names with no entry keep declining exactly as before.
// A generic suffix or digit normalization would silently pick the wrong
// broadcast here, which is the failure mode the bridge must not introduce.
func TestCanonicalChannel_DoesNotBridgeAdjacentSignals(t *testing.T) {
	distinct := []struct {
		name string
		a    string
		b    string
	}{
		{
			name: "numbered ESPN sibling is not the primary signal",
			a:    "ESPN 2",
			b:    "ESPN",
		},
		{
			name: "numbered ESPN sibling with a format label is not ESPN 1",
			a:    "ESPN 2 HD",
			b:    "ESPN 1 HD",
		},
		{
			name: "numbered RCN sibling is not the main RCN signal",
			a:    "RCN HD2",
			b:    "RCN HD",
		},
		{
			name: "numbered Caracol sibling is not Caracol TV",
			a:    "Caracol HD2",
			b:    "Caracol TV HD",
		},
	}

	for _, tt := range distinct {
		t.Run(tt.name, func(t *testing.T) {
			keyA, _, okA := CanonicalChannel(tt.a)
			keyB, _, okB := CanonicalChannel(tt.b)

			// Both being unknown already keeps them apart; the assertion only has
			// teeth when both resolve, and then they must resolve differently.
			if okA && okB && keyA == keyB {
				t.Errorf("CanonicalChannel(%q) and CanonicalChannel(%q) both resolve to %q, want distinct signals",
					tt.a, tt.b, keyA)
			}
		})
	}

	unknown := []string{"Caracol HD2", "RCN HD2", "ESPN 8", "Canal RCN 2"}
	for _, name := range unknown {
		if key, _, ok := CanonicalChannel(name); ok {
			t.Errorf("CanonicalChannel(%q) = %q, want unknown: names outside the table must keep declining", name, key)
		}
	}
}

// TestChannelBridges_DoNotRedefineDeclaredNames is the preserve check for the
// bridge: every display name and alias channelSpecs already declares keeps
// resolving to its own key. A bridge entry may only add a new spelling, so it can
// never hijack a name that already worked.
func TestChannelBridges_DoNotRedefineDeclaredNames(t *testing.T) {
	for _, spec := range channelSpecs {
		names := append([]string{spec.Display}, spec.Aliases...)
		for _, name := range names {
			key, p, ok := CanonicalChannel(name)
			if !ok || key != spec.Key || p != spec.P {
				t.Errorf("CanonicalChannel(%q) = (%q, %v, %v), want (%q, %v, true): the bridge must not redefine a declared name",
					name, key, p, ok, spec.Key, spec.P)
			}
		}
	}
}

// TestChannelBridges_TargetExistingSignals enforces the invariant that a bridge
// entry is only another spelling of a signal channelSpecs already declares. A
// bridge must never introduce a key: a key with no stream behind it would be a
// dead, unplayable channel.
func TestChannelBridges_TargetExistingSignals(t *testing.T) {
	if len(channelBridges) == 0 {
		t.Fatal("channelBridges is empty: the documented display-name bridge is missing")
	}

	specKeys := make(map[string]bool, len(channelSpecs))
	for _, spec := range channelSpecs {
		specKeys[spec.Key] = true
	}

	for _, bridge := range channelBridges {
		if !specKeys[bridge.Key] {
			t.Errorf("channelBridges entry %q targets key %q, which channelSpecs does not declare",
				bridge.Variant, bridge.Key)
		}
		if _, _, ok := CanonicalChannel(bridge.Variant); !ok {
			t.Errorf("channelBridges entry %q does not resolve; a dead bridge entry is a silent no-op",
				bridge.Variant)
		}
	}
}
