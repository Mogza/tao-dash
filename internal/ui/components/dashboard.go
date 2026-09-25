package components

import (
	"fmt"
	"strings"
	"time"

	"Mogza/TaoDash/internal/cache"
	"Mogza/TaoDash/internal/config"
	"Mogza/TaoDash/internal/network"
	"Mogza/TaoDash/internal/types"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Design system ---

var (
	taoNeon   = lipgloss.Color("#00FFAA")
	white     = lipgloss.Color("#FFFFFF")
	grayDark  = lipgloss.Color("#333333")
	grayLight = lipgloss.Color("#888888")
	redAlert  = lipgloss.Color("#FF3366")
	yellow    = lipgloss.Color("#FFD700")
	cyan      = lipgloss.Color("#00CFFF")
	amber     = lipgloss.Color("#FFA500")

	titleStyle  = lipgloss.NewStyle().Foreground(grayDark).Background(taoNeon).Bold(true).Padding(0, 2)
	metricStyle = lipgloss.NewStyle().Foreground(white).Bold(true)
	labelStyle  = lipgloss.NewStyle().Foreground(grayLight)
	warnStyle   = lipgloss.NewStyle().Foreground(yellow).Bold(true)

	paneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(grayDark).
			Padding(0, 1)

	rowStyle         = lipgloss.NewStyle().Foreground(grayLight)
	selectedRowStyle = lipgloss.NewStyle().Foreground(taoNeon).Bold(true).Background(grayDark)
	// ownKeyStyle highlights the operator's own hotkey row — amber/gold, distinct from green selection
	ownKeyStyle = lipgloss.NewStyle().Foreground(amber).Bold(true).Background(lipgloss.Color("#1a0f00"))
)

// --- Model ---

const (
	minNetUID = 1
	maxNetUID = 256 // Bittensor's subnet limit expanded beyond 64 with Dynamic Subnets
	maxRows   = 15
)

type Model struct {
	neurons        []types.Neuron
	cursor         int
	block          int
	logs           []string
	cfg            config.Config
	loading        bool
	netUID         int
	sortIndex      int  // index into network.SortOptions
	filterMode      network.FilterMode
	blockSub        network.BlockSub
	redisClient     *cache.Client
	lastCached      bool
	fetchDebounceID int // used to debounce rapid key presses
}

func InitialModel() Model {
	cfg := config.Load()

	var redisClient *cache.Client
	if cfg.HasRedis() {
		var err error
		redisClient, err = cache.New(cfg.RedisURL)
		if err != nil {
			redisClient = nil
		}
	}

	return Model{
		neurons:        nil,
		cursor:         0,
		block:          0,
		logs:           []string{fmt.Sprintf("[%s] TAO-DASH initialized.", ts())},
		cfg:            cfg,
		loading:        true,
		netUID:         cfg.DefaultNetUID,
		sortIndex:      0, // default: stake_desc
		filterMode:     network.FilterAll,
		blockSub:       network.NewBlockSub(),
		redisClient:    redisClient,
	}
}

// Init starts the WebSocket block watcher and fires the first metagraph fetch.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{}

	if m.cfg.HasRedis() && m.redisClient != nil {
		m.logs = append(m.logs, fmt.Sprintf("[%s] Redis connected (%s).", ts(), m.cfg.RedisURL))
	} else if m.cfg.HasRedis() {
		m.logs = append(m.logs, fmt.Sprintf("[%s] WARN: Redis unreachable, running without cache.", ts()))
	}
	if m.cfg.MyHotkey != "" {
		m.logs = append(m.logs, fmt.Sprintf("[%s] Tracking hotkey: %s", ts(), truncate(m.cfg.MyHotkey, 16)))
	}

	network.StartBlockWatcher(m.cfg.SubstrateWSURL, m.blockSub)
	cmds = append(cmds, network.WaitForBlock(m.blockSub))
	cmds = append(cmds, m.fetchCmd(0))

	return tea.Batch(cmds...)
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

		// Subnet navigation
		case "[", "left":
			if m.netUID > minNetUID {
				m.netUID--
				m.cursor = 0
				m.loading = true
				m.addLog(fmt.Sprintf("Switching to Subnet %d...", m.netUID))
				return m, m.debounceCmd()
			}

		case "]", "right":
			if m.netUID < maxNetUID {
				m.netUID++
				m.cursor = 0
				m.loading = true
				m.addLog(fmt.Sprintf("Switching to Subnet %d...", m.netUID))
				return m, m.debounceCmd()
			}

		// Cycle sort order
		case "s":
			m.sortIndex = (m.sortIndex + 1) % len(network.SortOptions)
			m.loading = true
			m.cursor = 0
			m.addLog(fmt.Sprintf("Sort: %s", network.SortOptions[m.sortIndex].Label))
			return m, m.debounceCmd()

		// Toggle validator filter
		case "v":
			switch m.filterMode {
			case network.FilterAll:
				m.filterMode = network.FilterValidators
			case network.FilterValidators:
				m.filterMode = network.FilterMiners
			case network.FilterMiners:
				m.filterMode = network.FilterAll
			}
			m.loading = true
			m.cursor = 0
			m.addLog(fmt.Sprintf("Filter: %s", m.filterMode))
			return m, m.debounceCmd()

		// Force refresh (bypasses cache)
		case "r":
			if m.cfg.HasAPIKey() {
				m.loading = true
				m.addLog("Manual refresh.")
				return m, m.fetchCmd(0)
			}
			m.addLog("Refresh unavailable: TAOSTATS_API_KEY not set.")
		}

	// New Substrate block received via WebSocket
	case network.NewBlockMsg:
		m.block = msg.Number
		m.loading = true
		m.addLog(fmt.Sprintf("Block #%d detected.", msg.Number))
		return m, tea.Batch(
			network.WaitForBlock(m.blockSub), // re-queue to keep listening
			m.fetchCmd(msg.Number),           // fetch with block hint for cache lookup
		)

	// Debounce timer fired
	case DebounceMsg:
		if msg.ID == m.fetchDebounceID {
			return m, m.fetchCmd(0) // actual fetch only if this was the last keypress
		}

	// Block watcher error (logged, no crash)
	case network.BlockWatchErrMsg:
		m.addLog(fmt.Sprintf("WS ERR: %s", msg.Err.Error()))
		return m, network.WaitForBlock(m.blockSub)

	// Metagraph data received
	case network.MetagraphMsg:
		m.loading = false
		m.neurons = msg.Neurons
		m.block = msg.Block
		m.lastCached = msg.FromCache
		source := "API"
		if msg.FromCache {
			source = "CACHE"
		}
		m.addLog(fmt.Sprintf("[%s] %d neurons @ block %d (Subnet %d).",
			source, len(msg.Neurons), msg.Block, msg.NetUID))

	// Network error
	case network.MetagraphErrMsg:
		m.loading = false
		m.addLog(fmt.Sprintf("ERR: %s", msg.Err.Error()))
		if len(m.neurons) == 0 {
			m.neurons = getMockNeurons()
		}
	}

	return m, nil
}

// --- View ---

func (m Model) View() string {
	// ── Header ──────────────────────────────────────────────────────────────
	title := titleStyle.Render(fmt.Sprintf(" TAO-DASH // SUBNET [%d] ", m.netUID))

	blockStr := "—"
	if m.block > 0 {
		blockStr = fmt.Sprintf("%d", m.block)
	}
	blockInfo := fmt.Sprintf("%s %s", labelStyle.Render("BLOCK:"), metricStyle.Render(blockStr))

	statusStr := lipgloss.NewStyle().Foreground(taoNeon).Render("● LIVE")
	if m.loading {
		statusStr = lipgloss.NewStyle().Foreground(yellow).Render("◌ SYNCING")
	} else if m.lastCached {
		statusStr = lipgloss.NewStyle().Foreground(cyan).Render("◈ CACHED")
	}
	if !m.cfg.HasAPIKey() {
		statusStr = lipgloss.NewStyle().Foreground(redAlert).Render("● MOCK")
	}

	cacheStr := lipgloss.NewStyle().Foreground(redAlert).Render("CACHE: OFF")
	if m.redisClient != nil {
		cacheStr = lipgloss.NewStyle().Foreground(taoNeon).Render("CACHE: ON")
	}

	neuronCount := fmt.Sprintf("%s %s",
		labelStyle.Render("NEURONS:"), metricStyle.Render(fmt.Sprintf("%d", len(m.neurons))))

	filterStr := labelStyle.Render("ALL")
	if m.filterMode == network.FilterValidators {
		filterStr = lipgloss.NewStyle().Foreground(cyan).Render("VALIDATORS")
	} else if m.filterMode == network.FilterMiners {
		filterStr = lipgloss.NewStyle().Foreground(cyan).Render("MINERS")
	}

	headerInfo := lipgloss.JoinHorizontal(lipgloss.Center,
		blockInfo, "   |   ", neuronCount, "   |   ", statusStr, "   |   ", cacheStr, "   |   ", filterStr,
	)
	header := lipgloss.JoinHorizontal(lipgloss.Bottom, title, "    ", headerInfo)

	// ── Table ────────────────────────────────────────────────────────────────
	headers := buildHeaders(m.sortIndex)
	var table strings.Builder
	fmt.Fprintf(&table, "%s\n", lipgloss.NewStyle().Bold(true).Foreground(white).Render(headers))
	table.WriteString(strings.Repeat("─", 87) + "\n")

	displayCount := len(m.neurons)
	if displayCount > maxRows {
		displayCount = maxRows
	}

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

		prefix := " "
		if m.cursor == i {
			prefix = "▶"
		}

		conStr := fmt.Sprintf("%-8.4f", n.Consensus)
		if n.ValidatorPermit && n.Consensus == 0 {
			conStr = "VAL     "
		}

		row := fmt.Sprintf("%s%-4d │ %-8s │ %-14.2f │ %-8s │ %-10.4f │ %-8.4f │ %s",
			prefix, n.UID, truncate(n.HotkeySS58, 8), n.Stake, conStr, n.Emission, n.Dividends, permit)

		switch {
		case m.cfg.MyHotkey != "" && n.HotkeySS58 == m.cfg.MyHotkey:
			// Own node — amber highlight, takes priority over cursor
			table.WriteString(ownKeyStyle.Render(row+"  ★") + "\n")
		case m.cursor == i:
			table.WriteString(selectedRowStyle.Render(row) + "\n")
		default:
			table.WriteString(rowStyle.Render(row) + "\n")
		}
	}

	if len(m.neurons) > displayCount {
		table.WriteString(labelStyle.Render(fmt.Sprintf(
			"  ↕ %d/%d neurons (scroll with j/k)", m.cursor+1, len(m.neurons))) + "\n")
	}

	tablePane := paneStyle.Width(90).Height(20).Render(table.String())

	// ── Logs ─────────────────────────────────────────────────────────────────
	var logs strings.Builder
	logs.WriteString(lipgloss.NewStyle().Bold(true).Foreground(taoNeon).Render("LIVE LOGS:") + "\n")
	for _, l := range m.logs {
		logs.WriteString(labelStyle.Render(l) + "\n")
	}
	logPane := paneStyle.Width(90).Height(6).Render(logs.String())

	// ── Footer ───────────────────────────────────────────────────────────────
	sort := network.SortOptions[m.sortIndex].Label
	footer := labelStyle.Render(fmt.Sprintf(
		"\n  [←/→]: Subnet   [s]: Sort (%s)   [v]: Filter   [↑/k][↓/j]: Scroll   [r]: Refresh   [q]: Quit",
		sort,
	))
	if !m.cfg.HasAPIKey() {
		footer += "\n" + warnStyle.Render("  ⚠  export TAOSTATS_API_KEY=<your_key> to enable live data")
	}

	ui := lipgloss.JoinVertical(lipgloss.Left, header, "\n", tablePane, logPane, footer)
	return lipgloss.NewStyle().Margin(1, 2).Render(ui)
}

// --- Helpers ---

// DebounceMsg is sent after a short delay following a keypress to prevent API spam.
type DebounceMsg struct {
	ID int
}

func (m *Model) debounceCmd() tea.Cmd {
	m.fetchDebounceID++
	id := m.fetchDebounceID
	return tea.Tick(300*time.Millisecond, func(_ time.Time) tea.Msg {
		return DebounceMsg{ID: id}
	})
}

// fetchCmd builds a FetchMetagraph tea.Cmd from current model state.
// blockNumber = 0 forces an API call (skips cache) — used on subnet switch, sort change, etc.
func (m Model) fetchCmd(blockNumber int) tea.Cmd {
	if !m.cfg.HasAPIKey() {
		return func() tea.Msg {
			return network.MetagraphErrMsg{
				Err: fmt.Errorf("TAOSTATS_API_KEY not set — mock mode active"),
			}
		}
	}
	return network.FetchMetagraph(m.cfg.TaostatsAPIKey, network.FetchOptions{
		NetUID:      m.netUID,
		BlockNumber: blockNumber,
		SortOrder:   network.SortOptions[m.sortIndex].APIValue,
		Filter:      m.filterMode,
	}, m.redisClient)
}

// buildHeaders returns the formatted table header row with sort indicator on the active column.
func buildHeaders(sortIndex int) string {
	cols := []string{"UID", "HOTKEY", "STAKE (τ)", "CONSENSUS", "EMISSION", "DIVIDND", "PERMIT"}
	opt := network.SortOptions[sortIndex]
	cols[opt.ColumnIndex] = lipgloss.NewStyle().
		Foreground(taoNeon).Bold(true).
		Render(cols[opt.ColumnIndex] + " " + arrow(opt.APIValue))

	return fmt.Sprintf(" %-5s │ %-8s │ %-14s │ %-9s │ %-10s │ %-8s │ %s",
		cols[0], cols[1], cols[2], cols[3], cols[4], cols[5], cols[6])
}

func arrow(apiValue string) string {
	if strings.HasSuffix(apiValue, "_asc") {
		return "↑"
	}
	return "↓"
}

func (m *Model) addLog(msg string) {
	m.logs = append(m.logs, fmt.Sprintf("[%s] %s", ts(), msg))
	if len(m.logs) > 5 {
		m.logs = m.logs[len(m.logs)-5:]
	}
}

func ts() string { return time.Now().Format("15:04:05") }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func getMockNeurons() []types.Neuron {
	return []types.Neuron{
		{UID: 0, HotkeySS58: "5F3sa2TJ", Stake: 154320.50, Trust: 0.9854, Emission: 12.45, Dividends: 0.2534, ValidatorPermit: true},
		{UID: 14, HotkeySS58: "5DqrUa2z", Stake: 89040.20, Trust: 0.9120, Emission: 8.12, Dividends: 0.1832, ValidatorPermit: true},
		{UID: 42, HotkeySS58: "5EKrpcqV", Stake: 45000.00, Trust: 0.8500, Emission: 4.05, Dividends: 0.0921, ValidatorPermit: true},
		{UID: 128, HotkeySS58: "5FpsgU3J", Stake: 21000.75, Trust: 0.7200, Emission: 1.89, Dividends: 0.0412, ValidatorPermit: false},
		{UID: 255, HotkeySS58: "5GgMeLFN", Stake: 5000.10, Trust: 0.4500, Emission: 0.20, Dividends: 0.0050, ValidatorPermit: false},
	}
}
