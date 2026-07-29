package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

type theme struct {
	accent, accentSoft color.Color
	text, muted        color.Color
	surface, border    color.Color
	success, warning   color.Color
	danger             color.Color
}

func themeFor(isDark bool) theme {
	if !isDark {
		return theme{
			accent: lipgloss.Color("#087F8C"), accentSoft: lipgloss.Color("#DDF4F3"),
			text: lipgloss.Color("#17202A"), muted: lipgloss.Color("#667085"),
			surface: lipgloss.Color("#F2F4F7"), border: lipgloss.Color("#CDD5DF"),
			success: lipgloss.Color("#16794A"), warning: lipgloss.Color("#9A6700"), danger: lipgloss.Color("#C62828"),
		}
	}
	return theme{
		accent: lipgloss.Color("#4FD1C5"), accentSoft: lipgloss.Color("#173B3B"),
		text: lipgloss.Color("#E6EDF3"), muted: lipgloss.Color("#8B949E"),
		surface: lipgloss.Color("#161B22"), border: lipgloss.Color("#30363D"),
		success: lipgloss.Color("#56D364"), warning: lipgloss.Color("#E3B341"), danger: lipgloss.Color("#F85149"),
	}
}
