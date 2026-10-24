// Package i18n holds every text Kickoff renders on its own behalf.
//
// The catalog is a typed struct instead of a string map: each domain text is a
// field, so a missing translation is a visible gap rather than a runtime lookup
// that silently returns nothing. Interpolated texts are filled through named
// placeholders ({league}, {n}) and never through positional verbs, because
// argument order changes with the target language.
//
// Language resolution follows a closed precedence chain (see ResolveLanguage):
// KICKOFF_LANG, then ~/.config/kickoff/config.json, then LC_ALL, LC_MESSAGES and
// LANG, then Spanish. An unusable value never aborts the program; it produces a
// single warning per process.
//
// Fase 3 compiles the English catalog so callers can select between ES and EN.
package i18n

import (
	"errors"
	"reflect"
	"strconv"
	"strings"

	"github.com/e12jassir/kickoff-cli/internal/model"
)

// Lang identifies a supported interface language.
type Lang string

const (
	// ES is Spanish, the language the catalog is compiled in.
	ES Lang = "es"

	// EN is English. New(EN) returns the compiled English catalog.
	EN Lang = "en"
)

// Catalog holds the texts Kickoff renders on its own behalf. Provider data
// (team names, status descriptions, venues) is never translated by heuristic and
// therefore never appears here.
//
// Fields that contain placeholders are paired with a method of the same domain
// name (HeaderTemplate with Header, for example); the method substitutes the
// named placeholders and is the only supported way to render the text.
type Catalog struct {
	lang Lang

	// Application chrome.
	WindowTooSmall         string // terminal too small to draw the interface
	StartupSubtitle        string // startup screen subtitle
	StartupLoaderTemplate  string // startup spinner line
	StartupHint            string // startup dismissal hint
	MenuSubtitle           string // main menu subtitle
	MenuHelp               string // main menu footer
	MenuHelpPlaying        string // main menu footer while a stream plays
	MenuHelpUpdateTemplate string // main menu footer advertising an update
	UpdateBannerTemplate   string // banner announcing an available update

	// Main menu categories.
	CategoryInternational string // "Torneos Internacionales" / "International Tournaments"
	CategoryNationalTeams string // "Selecciones" / "National Teams"
	CategoryEuropean      string // "Ligas Europeas" / "European Leagues"
	CategoryDomesticCups  string // "Copas Nacionales" / "Domestic Cups"
	CategorySouthAmerican string // "Ligas Sudamericanas" / "South American Leagues"
	CategoryAmericas      string // "Ligas de América" / "Americas Leagues"
	CategoryOther         string // "Otras Competiciones" / "Other Competitions"

	HeaderTemplate          string // league header
	OfflineBadgeTemplate    string // cached-data badge
	LiveBadge               string // live-sync badge
	TabResumen              string // "Resumen" tab label
	TabPartidos             string // "Partidos" tab label
	TabPosiciones           string // "Posiciones" tab label
	TabGoleadores           string // "Goleadores" tab label
	BrowseHelp              string // browsing footer
	BrowseHelpPlaying       string // browsing footer while a stream plays
	FallbackChannel         string // channel name fallback
	FallbackBitrate         string // bitrate label fallback
	ConnectingPulseTemplate string // connecting overlay
	ConnectingHelp          string // connecting footer
	BackHelp                string // generic back footer
	ErrorBodyTemplate       string // error body
	ErrorHelp               string // error footer
	ChannelModalHelp        string // channel modal footer, signals tab
	ChannelModalHelpTabs    string // channel modal footer, other tabs

	// Partidos tab.
	NoStreamsNotice              string // no stream source configured
	NoUpcomingMatchesTemplate    string // empty fixtures state
	MatchesTodayTemplate         string // header when the matchday is today
	UpcomingMatchesTemplate      string // header for a future matchday
	UpcomingMatchesDatedTemplate string // header for a future matchday with a date
	PositionCounterTemplate      string // match position within the list
	ScrollOlderOne               string // singular scroll-up marker
	ScrollOlderOther             string // plural scroll-up marker
	ScrollNewerOne               string // singular scroll-down marker
	ScrollNewerOther             string // plural scroll-down marker
	FinalBadge                   string // completed-result badge
	EnterDetailsHint             string // selected-card hint
	StatusPre                    string // fixture not started
	StatusPost                   string // fixture finished
	StatusLiveTemplate           string // live status prefix plus minute
	StatusFinished               string // finished modal header
	StatusScheduled              string // scheduled modal header without a time
	StatusTodayTemplate          string // kickoff marker for today
	StatusTomorrowTemplate       string // kickoff marker for tomorrow
	StatusYesterdayTemplate      string // kickoff marker for yesterday
	FallbackHome                 string // home team fallback
	FallbackAway                 string // away team fallback
	FallbackVenue                string // venue fallback
	FallbackPlayer               string // player name fallback
	FallbackCoachStaff           string // event fallback when no player is known
	FallbackOwnGoal              string // own-goal event fallback
	FallbackGoal                 string // goal event fallback

	// Resumen tab.
	LeagueLeaderTemplate           string // league leader highlight
	RecentResultsLastRoundTemplate string // empty recent-results header
	NoRecentResults                string // empty recent-results state
	RecentResultsTemplate          string // recent-results header

	// Posiciones tab.
	StandingsLoadError           string // standings fetch failure
	StandingsLoadingTemplate     string // standings spinner line
	StandingsHeaderTemplate      string // standings header
	GroupNavigationTemplate      string // group navigation row
	StandingsGroupHeaderTemplate string // group standings header
	StandingsUnavailable         string // empty standings state
	ColumnRank                   string // standings column: rank
	ColumnTeam                   string // standings column: team
	ColumnPlayed                 string // standings column: played
	ColumnWon                    string // standings column: won
	ColumnDrawn                  string // standings column: drawn
	ColumnLost                   string // standings column: lost
	ColumnGoalDiff               string // standings column: goal difference
	ColumnPoints                 string // standings column: points

	// Goleadores tab.
	ScorersLoadError       string // statistics fetch failure
	ScorersLoadingTemplate string // statistics spinner line
	TopScorersHeader       string // top scorers header
	NoScorerStats          string // empty scorers state
	TopAssistsHeader       string // top assists header
	NoAssistStats          string // empty assists state
	ColumnPlayer           string // statistics column: player
	ColumnGoals            string // statistics column: goals
	ColumnAssists          string // statistics column: assists

	// Channel modal.
	ModalTabSummary      string // modal tab label: summary
	ModalTabStats        string // modal tab label: statistics
	ModalTabLineups      string // modal tab label: lineups
	BroadcastFinished    string // no signals because the match is over
	NoSignalsBeforeMatch string // no signals before kickoff
	NoSignalsLive        string // no signals while live
	ResolvingSignals     string // signals being resolved
	SignalsSection       string // signals section header
	Recommended          string // recommended channel badge
	RefereeTemplate      string // referee line
	StatsUnavailable     string // empty statistics state
	StatsLoading         string // statistics being fetched
	Possession           string // possession bar label
	StatShots            string // shots statistic label
	StatPasses           string // passes statistic label
	StatFouls            string // fouls statistic label
	StatCorners          string // corners statistic label
	StatOffsides         string // offsides statistic label
	StatSaves            string // saves statistic label
	ExtraTimeBadge       string // extra-time marker in the timeline
	LineupStarters       string // starters section header
	LineupSubstitutes    string // substitutes section header
	LineupsUnavailable   string // empty lineups state
	CoachTemplate        string // coach line
	PenaltyTemplate      string // player name with a penalty marker
	OwnGoalTemplate      string // player name with an own-goal marker

	// Status messages produced by the update loop.
	NowPlayingTemplate      string // playback started
	PlaybackErrorTemplate   string // playback failed
	UpdateErrorTemplate     string // update failed
	UpdateSucceededTemplate string // update finished
	StreamStopped           string // stream stopped by the user
	LoadingMatchesTemplate  string // league fixtures being fetched
	ReloadingDataTemplate   string // league data being refreshed
	ConnectingTemplate      string // stream connection starting

	// Goal notifications.
	FallbackClock            string // clock fallback when the event has none
	GoalPenaltyTitleTemplate string // penalty goal notification title
	GoalOwnGoalTitleTemplate string // own-goal notification title
	GoalTitleTemplate        string // goal notification title
	GoalScoreTitleTemplate   string // notification title without event data
	AssistPenalty            string // penalty assist line
	AssistOwnGoal            string // own-goal assist line
	AssistNone               string // no-assist line
	AssistWithTemplate       string // assist line naming the assistant
	GoalBodyTemplate         string // notification body with score and assist

	// Setup wizard.
	SetupTitleTemplate                   string // setup screen title
	SetupMenuHeader                      string // setup menu header
	SetupOptionInstall                   string // setup menu option: install
	SetupOptionDoctor                    string // setup menu option: doctor
	SetupOptionTestStream                string // setup menu option: test stream
	SetupOptionImportChannels            string // setup menu option: import channels
	SetupOptionLanguageTemplate          string // setup menu option: language toggle
	SetupOptionCheckUpdates              string // setup menu option: check updates
	SetupOptionLaunch                    string // setup menu option: launch kickoff
	SetupOptionUninstall                 string // setup menu option: uninstall kickoff
	SetupOptionExit                      string // setup menu option: exit setup
	SetupMenuFooter                      string // setup menu footer keybinding hints
	SetupDoctorHeader                    string // setup doctor screen header
	SetupDoctorActions                   string // setup doctor actions list
	SetupInstallingHeader                string // setup installing screen header
	SetupInstallComplete                 string // setup installing complete banner
	SetupInstallAbortPrompt              string // setup installing abort confirmation banner
	SetupInstallFooter                   string // setup installing progress footer
	SetupInstallAbortFooter              string // setup installing abort footer
	SetupInstallCompleteFooter           string // setup installing complete footer
	SetupUpdateHeader                    string // setup update prompt header
	SetupUpdatePromptTemplate            string // setup update prompt question
	SetupUpdateOptions                   string // setup update prompt options
	SetupUpdateFooter                    string // setup update prompt footer
	SetupUninstallHeader                 string // setup uninstall prompt header
	SetupUninstallWarning                string // setup uninstall warning text
	SetupUninstallOptions                string // setup uninstall options
	SetupUninstallFooter                 string // setup uninstall footer
	SetupUninstallDone                   string // setup uninstalled exit message
	SetupStatusHealthy                   string // setup status: dependencies installed
	SetupStatusTestingMpv                string // setup status: testing mpv
	SetupStatusImportingChannels         string // setup status: importing channels
	SetupStatusCheckingUpdates           string // setup status: checking updates
	SetupStatusUpdatingTemplate          string // setup status: updating kickoff
	SetupStatusUpdatedTemplate           string // setup status: kickoff updated
	SetupStatusUpdateFailedTemplate      string // setup status: update failed
	SetupStatusUpdateCheckFailedTemplate string // setup status: update check failed
	SetupStatusUpToDateTemplate          string // setup status: up to date
	SetupStatusDevBuildTemplate          string // setup status: dev build
	SetupStatusCacheClearedTemplate      string // setup status: cache cleared
	SetupStatusCacheEmpty                string // setup status: cache empty
	SetupStatusCacheErrorTemplate        string // setup status: cache error
	SetupStatusDiagnosticsRefreshed      string // setup status: diagnostics refreshed
	SetupStatusInstallAborted            string // setup status: installation aborted
	SetupStatusInstallComplete           string // setup status: installation complete return prompt
	SetupStatusInstallAbortPrompt        string // setup status: installation abort confirm hint
	SetupStatusLangChanged               string // setup status: language changed confirmation
	SetupStepOS                          string // setup install step: detect os
	SetupStepMpv                         string // setup install step: verify mpv
	SetupStepFont                        string // setup install step: verify font
	SetupStepChannels                    string // setup install step: fetch channels
	SetupStepFinalize                    string // setup install step: finalize
	SetupStepOSDetectedTemplate          string // setup install detail: os detected
	SetupStepMpvInstalledTemplate        string // setup install detail: mpv installed
	SetupStepMpvNotFound                 string // setup install detail: mpv not found
	SetupStepFontVerified                string // setup install detail: font verified
	SetupStepFontWindows                 string // setup install detail: font verified windows
	SetupStepChannelsImportedTemplate    string // setup install detail: channels imported
	SetupStepAllVerified                 string // setup install detail: all verified
	SetupMpvNotFoundPath                 string // setup doctor: mpv not found on path
	SetupMpvOperationalTemplate          string // setup doctor: mpv operational
	SetupMpvErrorTemplate                string // setup doctor: mpv error

	// CLI and non-TUI surfaces.
	CLIHelpHeaderTemplate          string // CLI help header with version
	CLIHelpCommandsHeader          string // CLI help "Commands:" header
	CLIHelpCommandKickoff          string // CLI help kickoff command description
	CLIHelpCommandDemo             string // CLI help kickoff demo description
	CLIHelpCommandBar              string // CLI help kickoff bar description
	CLIHelpCommandInit             string // CLI help kickoff init description
	CLIHelpCommandConfigImport     string // CLI help kickoff config import description
	CLIHelpCommandImport           string // CLI help kickoff import description
	CLIHelpCommandUpdate           string // CLI help kickoff update description
	CLIHelpCommandSetup            string // CLI help kickoff setup description
	CLIHelpCommandVersion          string // CLI help kickoff version description
	CLIHelpFlagsHeader             string // CLI help "Flags:" header
	CLIFlagDemo                    string // CLI flag -demo description
	CLIFlagNoEmoji                 string // CLI flag -no-emoji description
	CLIFlagVersion                 string // CLI flag -version description
	CLIFlagVersionShort            string // CLI flag -v description
	CLIUsageConfigImport           string // CLI usage error for config import
	CLIUsageImport                 string // CLI usage error for import
	CLIUnknownConfigCommand        string // CLI unknown config command error
	CLIInitForceFlag               string // CLI init flag -force description
	CLIInitForceFlagShort          string // CLI init flag -f description
	CLIInitFileExistsTemplate      string // CLI init file already exists message
	CLIInitFileOverwrittenTemplate string // CLI init file overwritten message
	CLIInitFileCreatedTemplate     string // CLI init file created message
	CLIImportM3USuccessTemplate    string // CLI import M3U success message
	CLIImportJSONSuccessTemplate   string // CLI import JSON success message
	CLIImportPlaylistSavedTemplate string // CLI import playlist saved message
	CLIImportConfigSavedTemplate   string // CLI import config saved message
	CLIBarFlagFormat               string // CLI bar flag -format description
	CLIBarFlagFormatShort          string // CLI bar flag -f description
	CLIBarFlagWatch                string // CLI bar flag -watch description
	CLIBarFlagWatchShort           string // CLI bar flag -w description
	CLIBarFlagInterval             string // CLI bar flag -interval description
	CLIBarFlagIntervalShort        string // CLI bar flag -i description
	CLIBarFlagLeague               string // CLI bar flag -league description
	CLIBarFlagLeagueShort          string // CLI bar flag -l description
	CLIBarFlagTeam                 string // CLI bar flag -team description
	CLIBarFlagTeamShort            string // CLI bar flag -t description
	CLIBarFlagNotify               string // CLI bar flag -notify description
	CLIBarFlagNotifyShort          string // CLI bar flag -n description
	CLIBarFlagHasLive              string // CLI bar flag -has-live description
	CLIBarFlagNoEmoji              string // CLI bar flag -no-emoji description
	CLIBarFlagDemo                 string // CLI bar flag -demo description

	// Status bar.
	BarNoMatches     string // status bar empty match list
	BarVenueLabel    string // status bar tooltip venue label
	BarChannelsLabel string // status bar tooltip channels label

	// Domain errors.
	ErrPlayerTimeout    string // player initialization timed out
	ErrPlayerExited     string // player exited prematurely
	ErrInvalidURL       string // invalid stream url
	ErrCorruptedPayload string // corrupted or invalid payload
	ErrUpdateFailed     string // update installation failed
	ErrNetwork          string // network request failed
}

// New returns the catalog for l. Every supported language yields a complete
// catalog; an unsupported code falls back to the Spanish base rather than to a
// half-empty struct, which keeps New total.
func New(l Lang) *Catalog {
	var c Catalog
	switch l {
	case EN:
		c = englishCatalog()
	default:
		c = spanishCatalog()
	}
	c.lang = l
	return &c
}

// Default returns the catalog for the process-wide language.
func Default() *Catalog {
	return New(Language())
}

// Text returns the text stored in the field named key.
//
// It exists for callers that must render a text chosen at runtime. A key the
// catalog does not know renders as "!" + key on purpose, so a missing text is
// visible instead of blank; an English field without a translation falls back to
// the Spanish base. Production catalogs are complete, so the marker never
// reaches the interface (TestMissingKeyVisible pins both halves of that rule).
func (c *Catalog) Text(key string) string {
	if c == nil {
		return "!" + key
	}
	if value, ok := c.text(key); ok {
		return value
	}
	if c.lang == EN {
		base := spanishCatalog()
		if value, ok := base.text(key); ok {
			return value
		}
	}
	return "!" + key
}

// text resolves one exported string field by name.
func (c *Catalog) text(key string) (string, bool) {
	value := reflect.ValueOf(c).Elem().FieldByName(key)
	if !value.IsValid() || value.Kind() != reflect.String || !value.CanInterface() {
		return "", false
	}
	text := value.String()
	if strings.TrimSpace(text) == "" {
		return "", false
	}
	return text, true
}

// StartupLoader renders the startup spinner line.
func (c *Catalog) StartupLoader(spinner string) string {
	return interpolate(c.StartupLoaderTemplate, map[string]string{"spinner": spinner})
}

// MenuHelpUpdate renders the main menu footer that advertises an update.
func (c *Catalog) MenuHelpUpdate(version string) string {
	return interpolate(c.MenuHelpUpdateTemplate, map[string]string{"version": version})
}

// UpdateBanner renders the available-update banner.
func (c *Catalog) UpdateBanner(icon, version string) string {
	return interpolate(c.UpdateBannerTemplate, map[string]string{"icon": icon, "version": version})
}

// Lang returns the language of the catalog.
func (c *Catalog) Lang() Lang {
	if c == nil {
		return ES
	}
	return c.lang
}

// CategoryName returns the localized display name for a category ID.
func (c *Catalog) CategoryName(catID string) string {
	if c == nil {
		return catID
	}
	switch catID {
	case "international":
		return c.CategoryInternational
	case "national_teams":
		return c.CategoryNationalTeams
	case "european":
		return c.CategoryEuropean
	case "domestic_cups":
		return c.CategoryDomesticCups
	case "south_american":
		return c.CategorySouthAmerican
	case "americas":
		return c.CategoryAmericas
	case "other":
		return c.CategoryOther
	default:
		return catID
	}
}

// Header renders the league header for the browsing view.
func (c *Catalog) Header(league string) string {
	return interpolate(c.HeaderTemplate, map[string]string{"league": league})
}

// OfflineBadge renders the cached-data badge with its snapshot time.
func (c *Catalog) OfflineBadge(at string) string {
	return interpolate(c.OfflineBadgeTemplate, map[string]string{"time": at})
}

// ConnectingPulse renders the connecting overlay.
func (c *Catalog) ConnectingPulse(spinner, feed, wave, bitrate string) string {
	return interpolate(c.ConnectingPulseTemplate, map[string]string{
		"spinner": spinner,
		"feed":    feed,
		"wave":    wave,
		"bitrate": bitrate,
	})
}

// ErrorBody renders the error body with its message.
func (c *Catalog) ErrorBody(message string) string {
	return interpolate(c.ErrorBodyTemplate, map[string]string{"message": message})
}

// NoUpcomingMatches renders the empty fixtures state for a league.
func (c *Catalog) NoUpcomingMatches(league string) string {
	return interpolate(c.NoUpcomingMatchesTemplate, map[string]string{"league": league})
}

// MatchesToday renders the fixture-list header when the matchday is today.
func (c *Catalog) MatchesToday(n int) string {
	return interpolate(c.MatchesTodayTemplate, map[string]string{"n": count(n)})
}

// UpcomingMatches renders the fixture-list header for a future matchday. An
// empty date drops the date segment instead of leaving a dangling separator.
func (c *Catalog) UpcomingMatches(date string, n int) string {
	if date == "" {
		return interpolate(c.UpcomingMatchesTemplate, map[string]string{"n": count(n)})
	}
	return interpolate(c.UpcomingMatchesDatedTemplate, map[string]string{"date": date, "n": count(n)})
}

// PositionCounter renders the selected position within a list.
func (c *Catalog) PositionCounter(current, total int) string {
	return interpolate(c.PositionCounterTemplate, map[string]string{
		"current": count(current),
		"total":   count(total),
	})
}

// ScrollOlder renders the scroll marker above the visible window.
func (c *Catalog) ScrollOlder(n int) string {
	return interpolate(pluralForm(c.ScrollOlderOne, c.ScrollOlderOther, n), map[string]string{"n": count(n)})
}

// ScrollNewer renders the scroll marker below the visible window.
func (c *Catalog) ScrollNewer(n int) string {
	return interpolate(pluralForm(c.ScrollNewerOne, c.ScrollNewerOther, n), map[string]string{"n": count(n)})
}

// LeagueLeader renders the league-leader highlight.
func (c *Catalog) LeagueLeader(team, points, played, diff string) string {
	return interpolate(c.LeagueLeaderTemplate, map[string]string{
		"team":   team,
		"points": points,
		"played": played,
		"diff":   diff,
	})
}

// RecentResultsLastRound renders the recent-results header of an empty matchday.
func (c *Catalog) RecentResultsLastRound(n int) string {
	return interpolate(c.RecentResultsLastRoundTemplate, map[string]string{"n": count(n)})
}

// RecentResults renders the recent-results header with its position counter.
func (c *Catalog) RecentResults(total, current int) string {
	return interpolate(c.RecentResultsTemplate, map[string]string{
		"n":       count(total),
		"current": count(current),
		"total":   count(total),
	})
}

// StandingsLoading renders the standings spinner line.
func (c *Catalog) StandingsLoading(spinner string) string {
	return interpolate(c.StandingsLoadingTemplate, map[string]string{"spinner": spinner})
}

// StandingsHeader renders the standings header for a league.
func (c *Catalog) StandingsHeader(league string) string {
	return interpolate(c.StandingsHeaderTemplate, map[string]string{"league": league})
}

// GroupNavigation renders the group navigation row.
func (c *Catalog) GroupNavigation(name string, current, total int) string {
	return interpolate(c.GroupNavigationTemplate, map[string]string{
		"name":    name,
		"current": count(current),
		"total":   count(total),
	})
}

// StandingsGroupHeader renders the group standings header around a navigation row.
func (c *Catalog) StandingsGroupHeader(nav string) string {
	return interpolate(c.StandingsGroupHeaderTemplate, map[string]string{"nav": nav})
}

// ScorersLoading renders the statistics spinner line.
func (c *Catalog) ScorersLoading(spinner string) string {
	return interpolate(c.ScorersLoadingTemplate, map[string]string{"spinner": spinner})
}

// LiveStatus renders the live status prefix plus the current minute.
func (c *Catalog) LiveStatus(detail string) string {
	return interpolate(c.StatusLiveTemplate, map[string]string{"detail": detail})
}

// StatusToday renders a kickoff time marked as today.
func (c *Catalog) StatusToday(at string) string {
	return interpolate(c.StatusTodayTemplate, map[string]string{"time": at})
}

// StatusTomorrow renders a kickoff time marked as tomorrow.
func (c *Catalog) StatusTomorrow(at string) string {
	return interpolate(c.StatusTomorrowTemplate, map[string]string{"time": at})
}

// StatusYesterday renders a kickoff time marked as yesterday.
func (c *Catalog) StatusYesterday(at string) string {
	return interpolate(c.StatusYesterdayTemplate, map[string]string{"time": at})
}

// Referee renders the referee line.
func (c *Catalog) Referee(name string) string {
	return interpolate(c.RefereeTemplate, map[string]string{"name": name})
}

// Coach renders the coach line of a lineup column.
func (c *Catalog) Coach(name string) string {
	return interpolate(c.CoachTemplate, map[string]string{"name": name})
}

// WithPenalty marks a player name as a penalty scorer.
func (c *Catalog) WithPenalty(name string) string {
	return interpolate(c.PenaltyTemplate, map[string]string{"name": name})
}

// WithOwnGoal marks a player name as an own-goal scorer.
func (c *Catalog) WithOwnGoal(name string) string {
	return interpolate(c.OwnGoalTemplate, map[string]string{"name": name})
}

// NowPlaying renders the playback-started status message.
func (c *Catalog) NowPlaying(channel string) string {
	return interpolate(c.NowPlayingTemplate, map[string]string{"channel": channel})
}

// PlaybackError renders the playback failure prefix around a message.
func (c *Catalog) PlaybackError(message string) string {
	return interpolate(c.PlaybackErrorTemplate, map[string]string{"message": message})
}

// FormatPlaybackError maps err to a localized message and formats it using
// PlaybackErrorTemplate. A nil error returns "".
func (c *Catalog) FormatPlaybackError(err error) string {
	if err == nil {
		return ""
	}
	if c == nil {
		return err.Error()
	}
	return c.PlaybackError(c.FormatError(err))
}

// UpdateError renders the update failure prefix around a message.
func (c *Catalog) UpdateError(message string) string {
	return interpolate(c.UpdateErrorTemplate, map[string]string{"message": message})
}

// FormatUpdateError maps err to a localized message and formats it using
// UpdateErrorTemplate. A nil error returns "".
func (c *Catalog) FormatUpdateError(err error) string {
	if err == nil {
		return ""
	}
	if c == nil {
		return err.Error()
	}
	return c.UpdateError(c.FormatError(err))
}

// FormatError maps err to a localized message. Domain errors defined in
// internal/model are recognized via errors.Is and mapped to their catalog
// text; unrecognized errors fall back to err.Error(). A nil error returns "".
func (c *Catalog) FormatError(err error) string {
	if err == nil {
		return ""
	}
	if c == nil {
		return err.Error()
	}
	switch {
	case errors.Is(err, model.ErrPlayerTimeout):
		return c.ErrPlayerTimeout
	case errors.Is(err, model.ErrPlayerExited):
		return c.ErrPlayerExited
	case errors.Is(err, model.ErrInvalidURL):
		return c.ErrInvalidURL
	case errors.Is(err, model.ErrCorruptedPayload):
		return c.ErrCorruptedPayload
	case errors.Is(err, model.ErrUpdateFailed):
		return c.ErrUpdateFailed
	case errors.Is(err, model.ErrNetwork):
		return c.ErrNetwork
	default:
		return err.Error()
	}
}

// UpdateSucceeded renders the update-finished status message.
func (c *Catalog) UpdateSucceeded(version string) string {
	return interpolate(c.UpdateSucceededTemplate, map[string]string{"version": version})
}

// LoadingMatches renders the fixtures-loading message for a league.
func (c *Catalog) LoadingMatches(league string) string {
	return interpolate(c.LoadingMatchesTemplate, map[string]string{"league": league})
}

// ReloadingData renders the refresh message for a league.
func (c *Catalog) ReloadingData(league string) string {
	return interpolate(c.ReloadingDataTemplate, map[string]string{"league": league})
}

// Connecting renders the connection-starting message for a channel.
func (c *Catalog) Connecting(channel string) string {
	return interpolate(c.ConnectingTemplate, map[string]string{"channel": channel})
}

// GoalPenaltyTitle renders the title of a penalty-goal notification.
func (c *Catalog) GoalPenaltyTitle(player, clock string) string {
	return interpolate(c.GoalPenaltyTitleTemplate, map[string]string{"player": player, "clock": clock})
}

// GoalOwnGoalTitle renders the title of an own-goal notification.
func (c *Catalog) GoalOwnGoalTitle(player, clock string) string {
	return interpolate(c.GoalOwnGoalTitleTemplate, map[string]string{"player": player, "clock": clock})
}

// GoalTitle renders the title of a goal notification.
func (c *Catalog) GoalTitle(player, clock string) string {
	return interpolate(c.GoalTitleTemplate, map[string]string{"player": player, "clock": clock})
}

// GoalScoreTitle renders the notification title for a goal without event data.
func (c *Catalog) GoalScoreTitle(score string) string {
	return interpolate(c.GoalScoreTitleTemplate, map[string]string{"score": score})
}

// AssistWith renders the assist line naming the assistant.
func (c *Catalog) AssistWith(name string) string {
	return interpolate(c.AssistWithTemplate, map[string]string{"name": name})
}

// GoalBody renders the notification body: the score line, then the assist line.
func (c *Catalog) GoalBody(score, assist string) string {
	return interpolate(c.GoalBodyTemplate, map[string]string{"score": score, "assist": assist})
}

// SetupTitle renders the setup screen title.
func (c *Catalog) SetupTitle(version string) string {
	return interpolate(c.SetupTitleTemplate, map[string]string{"version": version})
}

// SetupOptionLanguage renders the language toggle option.
func (c *Catalog) SetupOptionLanguage(langName string) string {
	return interpolate(c.SetupOptionLanguageTemplate, map[string]string{"lang": langName})
}

// SetupUpdatePrompt renders the update prompt text.
func (c *Catalog) SetupUpdatePrompt(current, latest string) string {
	return interpolate(c.SetupUpdatePromptTemplate, map[string]string{"current": current, "latest": latest})
}

// SetupStatusUpdating renders the updating kickoff message.
func (c *Catalog) SetupStatusUpdating(version string) string {
	return interpolate(c.SetupStatusUpdatingTemplate, map[string]string{"version": version})
}

// SetupStatusUpdated renders the updated kickoff message.
func (c *Catalog) SetupStatusUpdated(version string) string {
	return interpolate(c.SetupStatusUpdatedTemplate, map[string]string{"version": version})
}

// SetupStatusUpdateFailed renders the update failed message.
func (c *Catalog) SetupStatusUpdateFailed(err string) string {
	return interpolate(c.SetupStatusUpdateFailedTemplate, map[string]string{"error": err})
}

// SetupStatusUpdateCheckFailed renders the update check failed message.
func (c *Catalog) SetupStatusUpdateCheckFailed(err string) string {
	return interpolate(c.SetupStatusUpdateCheckFailedTemplate, map[string]string{"error": err})
}

// SetupStatusUpToDate renders the up to date message.
func (c *Catalog) SetupStatusUpToDate(version string) string {
	return interpolate(c.SetupStatusUpToDateTemplate, map[string]string{"version": version})
}

// SetupStatusDevBuild renders the development build message.
func (c *Catalog) SetupStatusDevBuild(version string) string {
	return interpolate(c.SetupStatusDevBuildTemplate, map[string]string{"version": version})
}

// SetupStatusCacheCleared renders the cache cleared message.
func (c *Catalog) SetupStatusCacheCleared(count int) string {
	return interpolate(c.SetupStatusCacheClearedTemplate, map[string]string{"count": strconv.Itoa(count)})
}

// SetupStatusCacheError renders the cache error message.
func (c *Catalog) SetupStatusCacheError(err string) string {
	return interpolate(c.SetupStatusCacheErrorTemplate, map[string]string{"error": err})
}

// SetupStepOSDetected renders the detected OS and architecture detail.
func (c *Catalog) SetupStepOSDetected(osName, arch string) string {
	return interpolate(c.SetupStepOSDetectedTemplate, map[string]string{"os": osName, "arch": arch})
}

// SetupStepMpvInstalled renders the installed mpv version detail.
func (c *Catalog) SetupStepMpvInstalled(version string) string {
	return interpolate(c.SetupStepMpvInstalledTemplate, map[string]string{"version": version})
}

// SetupStepChannelsImported renders the imported channels count and file detail.
func (c *Catalog) SetupStepChannelsImported(count int, file string) string {
	return interpolate(c.SetupStepChannelsImportedTemplate, map[string]string{"count": strconv.Itoa(count), "file": file})
}

// SetupMpvOperational renders the mpv operational status message.
func (c *Catalog) SetupMpvOperational(version string) string {
	return interpolate(c.SetupMpvOperationalTemplate, map[string]string{"version": version})
}

// SetupMpvError renders the mpv error status message.
func (c *Catalog) SetupMpvError(err string) string {
	return interpolate(c.SetupMpvErrorTemplate, map[string]string{"error": err})
}

// CLIHelpHeader renders the CLI help header line.
func (c *Catalog) CLIHelpHeader(version string) string {
	return interpolate(c.CLIHelpHeaderTemplate, map[string]string{"version": version})
}

// CLIInitFileExists renders the init file exists message.
func (c *Catalog) CLIInitFileExists(path string) string {
	return interpolate(c.CLIInitFileExistsTemplate, map[string]string{"path": path})
}

// CLIInitFileOverwritten renders the init file overwritten message.
func (c *Catalog) CLIInitFileOverwritten(path string) string {
	return interpolate(c.CLIInitFileOverwrittenTemplate, map[string]string{"path": path})
}

// CLIInitFileCreated renders the init file created message.
func (c *Catalog) CLIInitFileCreated(path string) string {
	return interpolate(c.CLIInitFileCreatedTemplate, map[string]string{"path": path})
}

// CLIImportM3USuccess renders the import M3U success message.
func (c *Catalog) CLIImportM3USuccess(count int) string {
	return interpolate(c.CLIImportM3USuccessTemplate, map[string]string{"count": strconv.Itoa(count)})
}

// CLIImportJSONSuccess renders the import JSON success message.
func (c *Catalog) CLIImportJSONSuccess(count int) string {
	return interpolate(c.CLIImportJSONSuccessTemplate, map[string]string{"count": strconv.Itoa(count)})
}

// CLIImportPlaylistSaved renders the import playlist saved message.
func (c *Catalog) CLIImportPlaylistSaved(path string) string {
	return interpolate(c.CLIImportPlaylistSavedTemplate, map[string]string{"path": path})
}

// CLIImportConfigSaved renders the import config saved message.
func (c *Catalog) CLIImportConfigSaved(path string) string {
	return interpolate(c.CLIImportConfigSavedTemplate, map[string]string{"path": path})
}

// interpolate substitutes every {name} placeholder in template with its value.
//
// Placeholders are named on purpose: positional verbs would silently pair the
// wrong argument with the wrong slot as soon as a translation reorders them.
// strings.Replacer performs a single pass, so a substituted value that happens to
// contain braces is never reinterpreted.
func interpolate(template string, values map[string]string) string {
	pairs := make([]string, 0, len(values)*2)
	for name, value := range values {
		pairs = append(pairs, "{"+name+"}", value)
	}
	return strings.NewReplacer(pairs...).Replace(template)
}

// pluralForm selects the singular form for exactly one item and the plural form
// for every other count, including zero.
func pluralForm(one, other string, n int) string {
	if n == 1 {
		return one
	}
	return other
}

// count renders a count for interpolation.
func count(n int) string {
	return strconv.Itoa(n)
}
