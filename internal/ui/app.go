package ui

import (
	"Mogza/TaoDash/internal/ui/components"

	tea "github.com/charmbracelet/bubbletea"
)

func LaunchApp() {
	p := tea.NewProgram(
		components.InitialModel(),
		tea.WithAltScreen(),
	)
	if _, err := p.Run(); err != nil {
		panic(err)
	}
}
