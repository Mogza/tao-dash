package components

import (
	"fmt"
	"strings"
	"time"

	"Mogza/TaoDash/internal/config"
	"Mogza/TaoDash/internal/network"
	"Mogza/TaoDash/internal/types"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Auto-refresh tick ---

const refreshInterval = 30 * time.Second

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// --- DESIGN SYSTEM ---
var (
	taoNeon   = lipgloss.Color("#00FFAA")
	white     = lipgloss.Color("#FFFFFF")
	grayDark  = lipgloss.Color("#333333")
	grayLight = lipgloss.Color("#888888")
	redAlert  = lipgloss.Color("#FF3366")
	yellow    = lipgloss.Color("#FFD700")

	titleStyle = lipgloss.NewStyle().
			Foreground(grayDark).
			Background(taoNeon).
			Bold(true).
			Padding(0, 2)

	metricStyle = lipgloss.NewStyle().Foreground(white).Bold(true)
	labelStyle  = lipgloss.NewStyle().Foreground(grayLight)
	warnStyle   = lipgloss.NewStyle().Foreground(yellow).Bold(true)

	paneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(grayDark).
			Padding(0, 1)

	rowStyle         = lipgloss.NewStyle().Foreground(grayLight)
	selectedRowStyle = lipgloss.NewStyle().Foreground(taoNeon).Bold(true).Background(grayDark)
)

// --- Model ---

type Model struct {
	neurons  []types.Neuron
	cursor   int
	block    int
	taoPrice float64
	logs     []string
	cfg      config.Config
	loading  bool
	netUID   int
}

func InitialModel() Model {
	cfg := config.Load()
	return Model{
		neurons:  nil,
		cursor:   0,
		block:    0,
		taoPrice: 0,
		logs: []string{
			fmt.Sprintf("[%s] TAO-DASH initialized.", time.Now().Format("15:04:05")),
		},
		cfg:     cfg,
		loading: true,
		netUID:  cfg.DefaultNetUID,
	}
}

// Init fires the first metagraph fetch + starts the auto-refresh timer.
func (m Model) Init() tea.Cmd {
	if !m.cfg.HasAPIKey() {
		// No API key — we'll show mock data, logged in Update via a nil MetagraphMsg
		return tea.Batch(
			func() tea.Msg {
				return network.MetagraphErrMsg{Err: fmt.Errorf("TAOSTATS_API_KEY not set — using mock data. Export it to go live")}
			},
			tickCmd(),
		)
	}

	m.logs = append(m.logs, fmt.Sprintf("[%s] Fetching Subnet %d metagraph...", time.Now().Format("15:04:05"), m.netUID))
	return tea.Batch(
		network.FetchMetagraph(m.cfg.TaostatsAPIKey, m.netUID),
		tickCmd(),
	)
}

// --- Update ---

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
			if m.cursor < len(m.neurons)-1 {
				m.cursor++
			}
		case "r":
			// Manual refresh
			if m.cfg.HasAPIKey() {
				m.loading = true
				m.addLog("Manual refresh triggered.")
				return m, network.FetchMetagraph(m.cfg.TaostatsAPIKey, m.netUID)
			}
			m.addLog("Cannot refresh: no API key set.")
		}

	case network.MetagraphMsg:
		m.loading = false
		m.neurons = msg.Neurons
		m.block = msg.Block
		m.addLog(fmt.Sprintf("Synced %d neurons @ block %d (Subnet %d).",
			len(msg.Neurons), msg.Block, msg.NetUID))

	case network.MetagraphErrMsg:
		m.loading = false
		m.addLog(fmt.Sprintf("ERR: %s", msg.Err.Error()))
		// If no neurons loaded yet, fall back to mock data
		if len(m.neurons) == 0 {
			m.neurons = getMockNeurons()
			m.block = 0
		}

	case tickMsg:
		// Auto-refresh on tick
		if m.cfg.HasAPIKey() {
			m.loading = true
			return m, tea.Batch(
				network.FetchMetagraph(m.cfg.TaostatsAPIKey, m.netUID),
				tickCmd(),
			)
		}
		return m, tickCmd()
	}

	return m, nil
}

// --- View ---

func (m Model) View() string {
	title := titleStyle.Render(fmt.Sprintf(" TAO-DASH // SUBNET %d ", m.netUID))

	blockStr := "—"
	if m.block > 0 {
		blockStr = fmt.Sprintf("%d", m.block)
	}
	blockInfo := fmt.Sprintf("%s %s", labelStyle.Render("BLOCK:"), metricStyle.Render(blockStr))

	statusStr := lipgloss.NewStyle().Foreground(taoNeon).Render("● LIVE")
	if m.loading {
		statusStr = lipgloss.NewStyle().Foreground(yellow).Render("◌ SYNCING")
	}
	if !m.cfg.HasAPIKey() {
		statusStr = lipgloss.NewStyle().Foreground(redAlert).Render("● MOCK")
	}

	neuronCount := fmt.Sprintf("%s %s",
		labelStyle.Render("NEURONS:"),
		metricStyle.Render(fmt.Sprintf("%d", len(m.neurons))),
	)

	headerInfo := lipgloss.JoinHorizontal(lipgloss.Center, blockInfo, "   |   ", neuronCount, "   |   ", statusStr)
	header := lipgloss.JoinHorizontal(lipgloss.Bottom, title, "    ", headerInfo)

	// Table
	var table strings.Builder
	headerRow := fmt.Sprintf(" %-5s │ %-14s │ %-8s │ %-10s │ %-8s │ %s",
		"UID", "STAKE (τ)", "TRUST", "EMISSION", "DIVIDND", "PERMIT")
	fmt.Fprintf(&table, "%s\n", lipgloss.NewStyle().Bold(true).Foreground(white).Render(headerRow))
	table.WriteString(strings.Repeat("─", 72) + "\n")

	// Show at most 15 rows to fit the terminal
	displayCount := len(m.neurons)
	if displayCount > 15 {
		displayCount = 15
	}

	// Viewport offset for scrolling
	offset := 0
	if m.cursor >= displayCount {
		offset = m.cursor - displayCount + 1
	}

	for i := offset; i < offset+displayCount && i < len(m.neurons); i++ {
		n := m.neurons[i]
		permit := "✗"
		if n.ValidatorPermit {
			permit = "✓"
		}
		row := fmt.Sprintf(" %-5d │ %-14.2f │ %-8.4f │ %-10.4f │ %-8.4f │ %s",
			n.UID, n.Stake, n.Trust, n.Emission, n.Dividends, permit)

		if m.cursor == i {
			table.WriteString(selectedRowStyle.Render(row) + "\n")
		} else {
			table.WriteString(rowStyle.Render(row) + "\n")
		}
	}

	if len(m.neurons) > displayCount {
		scrollInfo := labelStyle.Render(fmt.Sprintf("  ↕ %d/%d neurons (scroll with j/k)", m.cursor+1, len(m.neurons)))
		table.WriteString(scrollInfo + "\n")
	}

	tablePane := paneStyle.Width(76).Height(20).Render(table.String())

	// Logs
	var logs strings.Builder
	logs.WriteString(lipgloss.NewStyle().Bold(true).Foreground(taoNeon).Render("LIVE LOGS:") + "\n")
	for _, l := range m.logs {
		logs.WriteString(labelStyle.Render(l) + "\n")
	}
	logPane := paneStyle.Width(76).Height(6).Render(logs.String())

	// Footer
	footer := labelStyle.Render("\n  [↑/k]: Up  [↓/j]: Down  [r]: Refresh  [q]: Quit")
	if !m.cfg.HasAPIKey() {
		footer += "\n" + warnStyle.Render("  ⚠  export TAOSTATS_API_KEY=<your_key> to enable live data")
	}

	ui := lipgloss.JoinVertical(lipgloss.Left, header, "\n", tablePane, logPane, footer)

	return lipgloss.NewStyle().Margin(1, 2).Render(ui)
}

// --- Helpers ---

func (m *Model) addLog(msg string) {
	entry := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg)
	m.logs = append(m.logs, entry)
	if len(m.logs) > 5 {
		m.logs = m.logs[len(m.logs)-5:]
	}
}

// getMockNeurons returns fallback data when no API key is available.
func getMockNeurons() []types.Neuron {
	return []types.Neuron{
		{UID: 0, Stake: 154320.50, Trust: 0.9854, Emission: 12.45, Dividends: 0.2534, ValidatorPermit: true},
		{UID: 14, Stake: 89040.20, Trust: 0.9120, Emission: 8.12, Dividends: 0.1832, ValidatorPermit: true},
		{UID: 42, Stake: 45000.00, Trust: 0.8500, Emission: 4.05, Dividends: 0.0921, ValidatorPermit: true},
		{UID: 128, Stake: 21000.75, Trust: 0.7200, Emission: 1.89, Dividends: 0.0412, ValidatorPermit: false},
		{UID: 256, Stake: 5000.10, Trust: 0.4500, Emission: 0.20, Dividends: 0.0050, ValidatorPermit: false},
	}
}
