package ui

func screenWidth(terminalWidth int) int {
	const horizontalPadding = 4
	return max(terminalWidth-horizontalPadding, 32)
}

func scrollPanelWidth(terminalWidth int) int {
	return max(screenWidth(terminalWidth)-2, 28)
}

func scrollViewportWidth(terminalWidth int) int {
	const panelBorderAndPadding = 4
	return max(scrollPanelWidth(terminalWidth)-panelBorderAndPadding, 24)
}
