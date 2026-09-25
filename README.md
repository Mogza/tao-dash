# TAO-DASHBOARD

> A zero-latency, async Terminal HUD for the Bittensor (TAO) Network.

## Vision
Monitoring Bittensor subnets currently involves high-friction web dashboards or blocking Python CLI scripts (`btcli`). **TAO-DASH** brings the metagraph directly into the engineer's terminal. 

Built with **Go**, it leverages an event-driven architecture to stream Substrate RPC data asynchronously. The goal is to provide a 60-FPS, non-blocking Head-Up Display for Node Operators (Miners/Validators) who live in Tmux/SSH environments.

## Architecture and Tech Stack
This is not a synchronous script. It's built for high concurrency and low latency.
* **Core:** Go (Golang)
* **UI Engine:** The Elm Architecture via `bubbletea` & `lipgloss` (Charmbracelet).
* **Concurrency:** Strict separation between the UI render loop (Main Thread) and network fetching (Background Goroutines via `tea.Cmd`).
* **Network:** Taostats REST API (`api.taostats.io`) for indexed metagraph data. Raw Substrate WebSocket RPC planned for Sprint 4+.

## Current State
**Status: Sprint 3 Complete — Live Metagraph Ingestion**

The app fetches real-time neuron data (UID, Stake, Trust, Emission, Dividends, Validator Permit) for Subnet 1 directly from the Taostats API. Data refreshes automatically every 30 seconds. Without an API key, the app falls back to mock data gracefully.

## Getting Started

```bash
git clone [repo url]
cd tao-dash

# Configure your API key (get one free at https://taostats.io/pro)
cp .env.example .env
# Edit .env and set TAOSTATS_API_KEY=your_key

go mod tidy
go run ./cmd/tao-dash/
```

**Controls:** `↑/k` Up · `↓/j` Down · `r` Refresh · `q` Quit

## Milestones & Roadmap

The development is structured in multiple sprints to replace mock data with real-time on-chain data.
- [x] Sprint 1: UI/UX Event Loop (Layout, Styling, Async architecture validation).
- [x] Sprint 2: Live Block Height (fetched from metagraph response, absorbed into Sprint 3).
- [x] Sprint 3: Metagraph Ingestion (Fetch live Validator/Miner stats: UIDs, Stake, Trust, Emission for a specific Subnet).
- [ ] Sprint 4: State Management & Caching (Implement Redis to prevent API spamming and handle state across multiple UI panes).
- [ ] Sprint 5: Dynamic Navigation (Subnet selection, sorting by yield/stake).
