package parrilla

import (
	"strings"
	"unicode"
)

// Playability classifies how a broadcast channel can be watched.
type Playability int

const (
	// Unknown means the channel could not be classified.
	Unknown Playability = iota
	// Linear is a traditional linear TV channel (cable / satellite / OTA).
	Linear
	// Streaming is an over-the-top / internet-only service.
	Streaming
)

// String returns a stable lowercase label for the playability class.
func (p Playability) String() string {
	switch p {
	case Linear:
		return "linear"
	case Streaming:
		return "streaming"
	default:
		return "unknown"
	}
}

// channelInfo is the canonical record for a single broadcast channel.
type channelInfo struct {
	Key     string
	Display string
	P       Playability
}

// channelSpec is the seed definition used to build the lookup index.
type channelSpec struct {
	Display string
	Key     string
	P       Playability
	Aliases []string
}

// channelSpecs is the seed mapping from published display names to canonical
// keys and playability classes. The key set is deliberately EXACTLY the stream
// key set declared in channels.json.example (the source of truth for which
// signals a user can play); a key that does not exist there can never resolve to
// a stream URL, so registering it would only produce dead, unplayable channels.
// Extend this table only together with channels.json.example.
var channelSpecs = []channelSpec{
	// Linear channels.
	{Display: "ESPN", Key: "espn1", P: Linear},
	{Display: "ESPN 2", Key: "espn2", P: Linear, Aliases: []string{"ESPN2"}},
	{Display: "ESPN 3", Key: "espn3", P: Linear, Aliases: []string{"ESPN3"}},
	{Display: "ESPN 4", Key: "espn4", P: Linear, Aliases: []string{"ESPN4"}},
	{Display: "ESPN 5", Key: "espn5", P: Linear, Aliases: []string{"ESPN5"}},
	{Display: "ESPN 6", Key: "espn6", P: Linear, Aliases: []string{"ESPN6"}},
	{Display: "ESPN 7", Key: "espn7", P: Linear, Aliases: []string{"ESPN7"}},
	{Display: "DSports", Key: "dsports", P: Linear, Aliases: []string{"D Sports"}},
	{Display: "DSports 2", Key: "dsports2", P: Linear, Aliases: []string{"D Sports 2"}},
	{Display: "DSports 4", Key: "dsports4", P: Linear, Aliases: []string{"D Sports 4"}},
	{Display: "Win Sports", Key: "win", P: Linear, Aliases: []string{"Win"}},
	{Display: "Win Sports Más", Key: "win_sports_mas", P: Linear, Aliases: []string{"Win Sports Mas", "Win Más"}},
	{Display: "Win+", Key: "win_plus", P: Linear, Aliases: []string{"Win Plus", "Win+ Fútbol", "Win Sports +"}},
	{Display: "TyC Sports Internacional", Key: "tyc", P: Linear, Aliases: []string{"TyC Sports"}},
	{Display: "Claro Sports", Key: "claro_sports", P: Linear},
	{Display: "Gol Caracol", Key: "gol_caracol", P: Linear},
	{Display: "Caracol TV", Key: "caracol", P: Linear, Aliases: []string{"Caracol"}},
	{Display: "RCN TV", Key: "rcn", P: Linear, Aliases: []string{"RCN"}},

	// Streaming services.
	{Display: "DSports+", Key: "dsports_plus", P: Streaming, Aliases: []string{"DSPORTS+", "DSports Plus", "D Sports+"}},
}

// channelIndex maps a normalized alias to its canonical record.
var channelIndex = buildChannelIndex()

func buildChannelIndex() map[string]channelInfo {
	idx := make(map[string]channelInfo, len(channelSpecs)*2)
	for _, spec := range channelSpecs {
		info := channelInfo{Key: spec.Key, Display: spec.Display, P: spec.P}
		idx[normalizeChannelName(spec.Display)] = info
		for _, alias := range spec.Aliases {
			idx[normalizeChannelName(alias)] = info
		}
	}
	return idx
}

// normalizeChannelName lowercases a published channel name and strips all
// whitespace and periods so that "ESPN 2", "ESPN2" and "espn.2" all collide,
// and "DSPORTS+" matches "DSports +".
func normalizeChannelName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		if r == '.' || unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// CanonicalChannel maps a published channel display name to its canonical key
// and playability class. It is tolerant of case, whitespace and periods.
// It returns ("", Unknown, false) for any channel it does not recognize.
func CanonicalChannel(name string) (key string, p Playability, ok bool) {
	info, found := channelIndex[normalizeChannelName(name)]
	if !found {
		return "", Unknown, false
	}
	return info.Key, info.P, true
}

// CanonicalDisplay returns the canonical display name for a published channel
// name. Unknown names report false and an empty string.
func CanonicalDisplay(name string) (string, bool) {
	info, found := channelIndex[normalizeChannelName(name)]
	if !found {
		return "", false
	}
	return info.Display, true
}
