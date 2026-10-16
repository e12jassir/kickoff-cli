package ui

import "github.com/charmbracelet/lipgloss"

// AsciiLogo renders Kickoff in ANSI Shadow block font (Jules/Neovim style).
const AsciiLogo = `██╗  ██╗██╗ ██████╗██╗  ██╗ ██████╗ ███████╗███████╗
██║ ██╔╝██║██╔════╝██║ ██╔╝██╔═══██╗██╔════╝██╔════╝
█████╔╝ ██║██║     █████╔╝ ██║   ██║█████╗  █████╗  
██╔═██╗ ██║██║     ██╔═██╗ ██║   ██║██╔══╝  ██╔══╝  
██║  ██╗██║╚██████╗██║  ██╗╚██████╔╝██║     ██║     
╚═╝  ╚═╝╚═╝ ╚═════╝╚═╝  ╚═╝ ╚═════╝ ╚═╝     ╚═╝     `

var (
	// Neutral Theme-Agnostic Dark Palette
	ColorHighlight      = lipgloss.Color("#FFFFFF") // Crisp White
	ColorText           = lipgloss.Color("#F4F4F5") // Zinc 100
	ColorTextMuted      = lipgloss.Color("#D4D4D8") // Zinc 300
	ColorCategory       = lipgloss.Color("#A1A1AA") // Zinc 400
	ColorMuted          = lipgloss.Color("#71717A") // Zinc 500
	ColorBorder         = lipgloss.Color("#3F3F46") // Zinc 700 (Subtle dark border)
	ColorBorderSelected = lipgloss.Color("#E4E4E7") // Zinc 200 (Crisp selection border)
	ColorError          = lipgloss.Color("#EF4444") // Coral Red
	ColorSuccess        = lipgloss.Color("#22C55E") // Bright Green (Badges/Success only)
	ColorAccent         = lipgloss.Color("#F59E0B") // Amber

	// Header, Title & Subtitle
	AppTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorHighlight).
			Padding(0, 1).
			MarginBottom(1)

	AsciiLogoStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorHighlight)

	SubtitleStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true)

	// Main Menu
	MenuBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Padding(1, 2).
			Width(56)

	CategoryHeaderStyle = lipgloss.NewStyle().
				Foreground(ColorCategory).
				Bold(true)

	MenuItemNormal = lipgloss.NewStyle().
			Foreground(ColorTextMuted)

	MenuItemSelected = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorHighlight)

	// League Tabs

	// Match Cards & Lists
	MatchBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Padding(1, 2).
			Width(62)

	MatchTimeStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Bold(true)

	TeamNameStyle = lipgloss.NewStyle().
			Foreground(ColorText).
			Bold(true)

	ScoreStyle = lipgloss.NewStyle().
			Foreground(ColorHighlight).
			Bold(true)

	ScoreboardTeams = lipgloss.NewStyle().
			Foreground(ColorHighlight).
			Bold(true)

	LiveTextStyle = lipgloss.NewStyle().
			Foreground(ColorError).
			Bold(true)

	LiveDotBright = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EF4444")).
			Bold(true)

	LiveDotDim = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7F1D1D")).
			Bold(true)

	StatusPostStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	StatusPreStyle = lipgloss.NewStyle().
			Foreground(ColorAccent)

	VenueStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true)

	// Modal Channel & Scoreboard Selector
	ModalBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorderSelected).
			Padding(1, 2).
			Width(66)

	ActiveTabPillStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorHighlight).
				Background(ColorBorder).
				Padding(0, 1)

	InactiveTabPillStyle = lipgloss.NewStyle().
				Foreground(ColorMuted).
				Padding(0, 1)

	ModalSectionHeader = lipgloss.NewStyle().
				Foreground(ColorCategory).
				Bold(true)

	ModalDivider = lipgloss.NewStyle().
			Foreground(ColorBorder)

	ChannelItemNormal = lipgloss.NewStyle().
				Foreground(ColorTextMuted).
				PaddingLeft(1)

	ChannelItemSelected = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorHighlight).
				PaddingLeft(1)

	RecommendedBadge = lipgloss.NewStyle().
				Foreground(ColorSuccess).
				Bold(true)

	ResolutionBadge = lipgloss.NewStyle().
			Foreground(ColorMuted)

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

	OfflineBadgeStyle = lipgloss.NewStyle().
				Foreground(ColorAccent).
				Bold(true)

	LiveBadgeStyle = lipgloss.NewStyle().
			Foreground(ColorCategory)

	// League Top View Tabs (Resumen, Partidos, Posiciones, Goleadores)
	LeagueTabActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorHighlight).
			Background(ColorBorder).
			Padding(0, 1)

	LeagueTabInactive = lipgloss.NewStyle().
				Foreground(ColorMuted).
				Padding(0, 1)

	// Connecting stream animation style
	ConnectingPulseStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#38BDF8")).
				Bold(true)
)
