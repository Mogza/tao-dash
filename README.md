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
* **Network (WIP):** JSON-RPC over WebSockets to query the Finney Mainnet.

## Current State
**Status: MVP / UI Engine Validation**
The core UI event loop and layout are implemented. The application currently uses a mock data state to validate the non-blocking architecture and UI responsiveness.

To run the current UI engine:
```bash
git clone [repo url]
cd tao-dash
go mod tidy
go run main.go
```
## Milestones & Roadmap

The development is structured in multiple sprints to replace mock data with real-time on-chain data.
- [x] Sprint 1: UI/UX Event Loop (Layout, Styling, Async architecture validation).
- [ ] Sprint 2: Substrate RPC Connection (Implement WebSocket client, fetch live block height).
- [ ] Sprint 3: Metagraph Ingestion (Fetch live Validator/Miner stats: UIDs, Stake, Trust, Emission for a specific Subnet).
- [ ] Sprint 4: State Management & Caching (Implement Redis to prevent RPC spamming and handle state across multiple UI panes).
- [ ] Sprint 5: Dynamic Navigation (Subnet selection, sorting by yield/stake).
