package ui

import "github.com/charmbracelet/lipgloss"

var (
	// Base Colors
	ColorPrimary   = lipgloss.Color("#10B981") // Emerald Green
	ColorSecondary = lipgloss.Color("#06B6D4") // Cyan
	ColorAccent    = lipgloss.Color("#F59E0B") // Amber
	ColorError     = lipgloss.Color("#EF4444") // Coral Red
	ColorSuccess   = lipgloss.Color("#22C55E") // Bright Green
	ColorMuted     = lipgloss.Color("#6B7280") // Cool Gray
	ColorText      = lipgloss.Color("#F9FAFB") // Bright White
	ColorDarkBg    = lipgloss.Color("#111827") // Dark Slate
	ColorActiveBg  = lipgloss.Color("#065F46") // Forest Green

	// Header & Title
	AppTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			Padding(0, 1).
			MarginBottom(1)

	// League Tabs
	ActiveTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorText).
			Background(ColorActiveBg).
			Padding(0, 1).
			MarginRight(1)

	InactiveTabStyle = lipgloss.NewStyle().
				Foreground(ColorMuted).
				Padding(0, 1).
				MarginRight(1)

	TabsRowStyle = lipgloss.NewStyle().
			MarginBottom(1)

	// Match Cards
	MatchCardNormal = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorMuted).
			Padding(0, 1).
			MarginBottom(1)

	MatchCardSelected = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorPrimary).
				Padding(0, 1).
				MarginBottom(1)

	MatchTimeStyle = lipgloss.NewStyle().
			Foreground(ColorSecondary).
			Bold(true)

	TeamNameStyle = lipgloss.NewStyle().
			Foreground(ColorText).
			Bold(true)

	ScoreStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)

	StatusLiveStyle = lipgloss.NewStyle().
			Foreground(ColorError).
			Bold(true)

	StatusPostStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	StatusPreStyle = lipgloss.NewStyle().
			Foreground(ColorAccent)

	VenueStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true)

	// Modal Channel Selector
	ModalBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Padding(1, 2)

	ModalTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			MarginBottom(1)

	ChannelItemNormal = lipgloss.NewStyle().
				Foreground(ColorText).
				PaddingLeft(1)

	ChannelItemSelected = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorPrimary).
				Background(ColorDarkBg).
				PaddingLeft(1)

	RecommendedBadge = lipgloss.NewStyle().
				Foreground(ColorSuccess).
				Bold(true)

	ResolutionBadge = lipgloss.NewStyle().
			Foreground(ColorSecondary)

	// Empty State & Info
	EmptyStateStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true).
			Padding(2, 4)

	// Footer & Status Bar
	StatusBarSuccess = lipgloss.NewStyle().
				Foreground(ColorSuccess).
				Bold(true)

	StatusBarError = lipgloss.NewStyle().
			Foreground(ColorError).
			Bold(true)

	FooterHelpStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			MarginTop(1)

	FooterKeyStyle = lipgloss.NewStyle().
			Foreground(ColorSecondary).
			Bold(true)
)
