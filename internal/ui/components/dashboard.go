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

// --- DESIGN SYSTEM ---
var (
	taoNeon   = lipgloss.Color("#00FFAA")
	white     = lipgloss.Color("#FFFFFF")
	grayDark  = lipgloss.Color("#333333")
	grayLight = lipgloss.Color("#888888")
	redAlert  = lipgloss.Color("#FF3366")
	yellow    = lipgloss.Color("#FFD700")
	cyan      = lipgloss.Color("#00CFFF")

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
)

// --- Model ---

type Model struct {
	neurons      []types.Neuron
	cursor       int
	block        int
	logs         []string
	cfg          config.Config
	loading      bool
	netUID       int
	blockSub     network.BlockSub  // channel partagé avec la goroutine WebSocket
	redisClient  *cache.Client     // nil si Redis absent
	lastCached   bool              // true si le dernier fetch vient du cache
}

func InitialModel() Model {
	cfg := config.Load()

	// Connexion Redis optionnelle : dégradation gracieuse si absent ou mal configuré
	var redisClient *cache.Client
	if cfg.HasRedis() {
		var err error
		redisClient, err = cache.New(cfg.RedisURL)
		if err != nil {
			// On loggue l'erreur plus bas dans Init() — pas de panic
			redisClient = nil
		}
	}

	return Model{
		neurons:     nil,
		cursor:      0,
		block:       0,
		logs:        []string{fmt.Sprintf("[%s] TAO-DASH initialized.", time.Now().Format("15:04:05"))},
		cfg:         cfg,
		loading:     true,
		netUID:      cfg.DefaultNetUID,
		blockSub:    network.NewBlockSub(),
		redisClient: redisClient,
	}
}

// Init démarre le block watcher WebSocket et lance le premier fetch metagraph.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{}

	// Redis status log
	if m.cfg.HasRedis() && m.redisClient != nil {
		m.logs = append(m.logs, fmt.Sprintf("[%s] Redis connected (%s).", time.Now().Format("15:04:05"), m.cfg.RedisURL))
	} else if m.cfg.HasRedis() && m.redisClient == nil {
		m.logs = append(m.logs, fmt.Sprintf("[%s] WARN: Redis unreachable, running without cache.", time.Now().Format("15:04:05")))
	}

	// Démarrage du block watcher (goroutine long-lived, reconnexion automatique)
	network.StartBlockWatcher(m.cfg.SubstrateWSURL, m.blockSub)
	cmds = append(cmds, network.WaitForBlock(m.blockSub))

	// Premier fetch metagraph (blockNumber=0 → force API, pas de cache check)
	if m.cfg.HasAPIKey() {
		cmds = append(cmds, network.FetchMetagraph(m.cfg.TaostatsAPIKey, m.netUID, 0, m.redisClient))
	} else {
		cmds = append(cmds, func() tea.Msg {
			return network.MetagraphErrMsg{Err: fmt.Errorf("TAOSTATS_API_KEY non définie — mode mock actif")}
		})
	}

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
		case "r":
			if m.cfg.HasAPIKey() {
				m.loading = true
				m.addLog("Refresh manuel déclenché.")
				// blockNumber=0 → force un appel API, ignore le cache
				return m, network.FetchMetagraph(m.cfg.TaostatsAPIKey, m.netUID, 0, m.redisClient)
			}
			m.addLog("Refresh impossible : TAOSTATS_API_KEY non définie.")
		}

	// Nouveau bloc Substrate reçu via WebSocket
	case network.NewBlockMsg:
		m.block = msg.Number
		m.loading = true
		m.addLog(fmt.Sprintf("Bloc #%d détecté.", msg.Number))
		return m, tea.Batch(
			network.WaitForBlock(m.blockSub), // re-queue pour continuer à écouter
			network.FetchMetagraph(m.cfg.TaostatsAPIKey, m.netUID, msg.Number, m.redisClient),
		)

	// Erreur du block watcher (affichée dans les logs, pas de crash)
	case network.BlockWatchErrMsg:
		m.addLog(fmt.Sprintf("WS ERR: %s", msg.Err.Error()))
		return m, network.WaitForBlock(m.blockSub) // toujours re-queue pour la reconnexion

	// Données metagraph reçues
	case network.MetagraphMsg:
		m.loading = false
		m.neurons = msg.Neurons
		m.block = msg.Block
		m.lastCached = msg.FromCache
		source := "API"
		if msg.FromCache {
			source = "CACHE"
		}
		m.addLog(fmt.Sprintf("[%s] %d neurons @ bloc %d (Subnet %d).", source, len(msg.Neurons), msg.Block, msg.NetUID))

	// Erreur API metagraph
	case network.MetagraphErrMsg:
		m.loading = false
		m.addLog(fmt.Sprintf("ERR: %s", msg.Err.Error()))
		if len(m.neurons) == 0 {
			m.neurons = getMockNeurons()
			m.block = 0
		}
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

	// Indicateur de statut
	statusStr := lipgloss.NewStyle().Foreground(taoNeon).Render("● LIVE")
	if m.loading {
		statusStr = lipgloss.NewStyle().Foreground(yellow).Render("◌ SYNCING")
	} else if m.lastCached {
		statusStr = lipgloss.NewStyle().Foreground(cyan).Render("◈ CACHED")
	}
	if !m.cfg.HasAPIKey() {
		statusStr = lipgloss.NewStyle().Foreground(redAlert).Render("● MOCK")
	}

	// Indicateur Redis
	cacheStr := lipgloss.NewStyle().Foreground(redAlert).Render("CACHE: OFF")
	if m.redisClient != nil {
		cacheStr = lipgloss.NewStyle().Foreground(taoNeon).Render("CACHE: ON")
	}

	neuronCount := fmt.Sprintf("%s %s",
		labelStyle.Render("NEURONS:"),
		metricStyle.Render(fmt.Sprintf("%d", len(m.neurons))),
	)

	headerInfo := lipgloss.JoinHorizontal(lipgloss.Center,
		blockInfo, "   |   ", neuronCount, "   |   ", statusStr, "   |   ", cacheStr,
	)
	header := lipgloss.JoinHorizontal(lipgloss.Bottom, title, "    ", headerInfo)

	// Table
	var table strings.Builder
	headerRow := fmt.Sprintf(" %-5s │ %-14s │ %-8s │ %-10s │ %-8s │ %s",
		"UID", "STAKE (τ)", "TRUST", "EMISSION", "DIVIDND", "PERMIT")
	fmt.Fprintf(&table, "%s\n", lipgloss.NewStyle().Bold(true).Foreground(white).Render(headerRow))
	table.WriteString(strings.Repeat("─", 72) + "\n")

	displayCount := len(m.neurons)
	if displayCount > 15 {
		displayCount = 15
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
		row := fmt.Sprintf(" %-5d │ %-14.2f │ %-8.4f │ %-10.4f │ %-8.4f │ %s",
			n.UID, n.Stake, n.Trust, n.Emission, n.Dividends, permit)

		if m.cursor == i {
			table.WriteString(selectedRowStyle.Render(row) + "\n")
		} else {
			table.WriteString(rowStyle.Render(row) + "\n")
		}
	}

	if len(m.neurons) > displayCount {
		table.WriteString(labelStyle.Render(fmt.Sprintf(
			"  ↕ %d/%d neurons (scroll with j/k)", m.cursor+1, len(m.neurons))) + "\n")
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
	footer := labelStyle.Render("\n  [↑/k]: Up  [↓/j]: Down  [r]: Force Refresh  [q]: Quit")
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

func getMockNeurons() []types.Neuron {
	return []types.Neuron{
		{UID: 0, Stake: 154320.50, Trust: 0.9854, Emission: 12.45, Dividends: 0.2534, ValidatorPermit: true},
		{UID: 14, Stake: 89040.20, Trust: 0.9120, Emission: 8.12, Dividends: 0.1832, ValidatorPermit: true},
		{UID: 42, Stake: 45000.00, Trust: 0.8500, Emission: 4.05, Dividends: 0.0921, ValidatorPermit: true},
		{UID: 128, Stake: 21000.75, Trust: 0.7200, Emission: 1.89, Dividends: 0.0412, ValidatorPermit: false},
		{UID: 256, Stake: 5000.10, Trust: 0.4500, Emission: 0.20, Dividends: 0.0050, ValidatorPermit: false},
	}
}
