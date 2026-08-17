package components

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Validator struct {
	UID      int
	Stake    float64
	Trust    float64
	Emission float64
}

func getMockValidators() []Validator {
	return []Validator{
		{UID: 0, Stake: 154320.50, Trust: 0.9854, Emission: 12.45},
		{UID: 14, Stake: 89040.20, Trust: 0.9120, Emission: 8.12},
		{UID: 42, Stake: 45000.00, Trust: 0.8500, Emission: 4.05},
		{UID: 128, Stake: 21000.75, Trust: 0.7200, Emission: 1.89},
		{UID: 256, Stake: 5000.10, Trust: 0.4500, Emission: 0.20},
	}
}

// DESIGN SYSTEM
var (
	taoNeon   = lipgloss.Color("#00FFAA")
	white     = lipgloss.Color("#FFFFFF")
	grayDark  = lipgloss.Color("#333333")
	grayLight = lipgloss.Color("#888888")
	redAlert  = lipgloss.Color("#FF3366")

	titleStyle = lipgloss.NewStyle().
			Foreground(grayDark).
			Background(taoNeon).
			Bold(true).
			Padding(0, 2)

	metricStyle = lipgloss.NewStyle().Foreground(white).Bold(true)
	labelStyle  = lipgloss.NewStyle().Foreground(grayLight)

	paneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(grayDark).
			Padding(0, 1)

	rowStyle         = lipgloss.NewStyle().Foreground(grayLight)
	selectedRowStyle = lipgloss.NewStyle().Foreground(taoNeon).Bold(true).Background(grayDark)
)

type Model struct {
	validators []Validator
	cursor     int
	block      int
	taoPrice   float64
	logs       []string
}

func InitialModel() Model {
	return Model{
		validators: getMockValidators(),
		cursor:     0,
		block:      2458901,
		taoPrice:   542.30,
		logs: []string{
			fmt.Sprintf("[%s] System initialized.", time.Now().Format("15:04:05")),
			fmt.Sprintf("[%s] Connected to Subtensor node via wss://...", time.Now().Format("15:04:05")),
			fmt.Sprintf("[%s] Syncing Subnet 1 weights...", time.Now().Format("15:04:05")),
		},
	}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.validators)-1 {
				m.cursor++
			}
		case "r":
			m.block++
			m.logs = append(m.logs, fmt.Sprintf("[%s] Block %d synced manually.", time.Now().Format("15:04:05"), m.block))
			if len(m.logs) > 4 {
				m.logs = m.logs[1:]
			}
		}
	}
	return m, nil
}

func (m Model) View() string {
	title := titleStyle.Render(" TAO-DASH // SUBNET 1 ")
	blockInfo := fmt.Sprintf("%s %s", labelStyle.Render("BLOCK:"), metricStyle.Render(fmt.Sprintf("%d", m.block)))
	priceInfo := fmt.Sprintf("%s %s", labelStyle.Render("TAO PRICE:"), metricStyle.Render(fmt.Sprintf("$%.2f", m.taoPrice)))

	headerInfo := lipgloss.JoinHorizontal(lipgloss.Center, blockInfo, "   |   ", priceInfo)
	header := lipgloss.JoinHorizontal(lipgloss.Bottom, title, "    ", headerInfo)

	var table strings.Builder
	fmt.Fprintf(&table, "%s\n", lipgloss.NewStyle().Bold(true).Foreground(white).Render(fmt.Sprintf(" %-5s | %-12s | %-8s | %-10s ", "UID", "STAKE (τ)", "TRUST", "EMISSION")))
	table.WriteString(strings.Repeat("-", 46) + "\n")

	for i, val := range m.validators {
		row := fmt.Sprintf(" %-5d | %-12.2f | %-8.4f | %-10.2f ", val.UID, val.Stake, val.Trust, val.Emission)

		if m.cursor == i {
			table.WriteString(selectedRowStyle.Render(row) + "\n")
		} else {
			table.WriteString(rowStyle.Render(row) + "\n")
		}
	}

	tablePane := paneStyle.Width(50).Height(10).Render(table.String())

	var logs strings.Builder
	logs.WriteString(lipgloss.NewStyle().Bold(true).Foreground(taoNeon).Render("LIVE LOGS:") + "\n")
	for _, l := range m.logs {
		logs.WriteString(labelStyle.Render(l) + "\n")
	}
	logPane := paneStyle.Width(50).Height(6).Render(logs.String())

	footer := labelStyle.Render("\n  [↑/k]: Up  [↓/j]: Down  [r]: Refresh  [q]: Quit")

	ui := lipgloss.JoinVertical(lipgloss.Left, header, "\n", tablePane, logPane, footer)

	return lipgloss.NewStyle().Margin(1, 2).Render(ui)
}
