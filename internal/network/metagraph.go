package network

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"Mogza/TaoDash/internal/types"

	tea "github.com/charmbracelet/bubbletea"
)

// --- Configuration ---

const (
	taostatsBaseURL = "https://api.taostats.io/api"
	defaultNetUID   = 1
	defaultLimit    = 256  // Max neurons per page (API caps at 1024)
	requestTimeout  = 10 * time.Second
)

// --- Taostats API JSON shapes ---
// These are internal to the network package. We parse them, then convert to our clean domain types.

type apiKey struct {
	Hex  string `json:"hex"`
	SS58 string `json:"ss58"`
}

type apiNeuron struct {
	UID             int    `json:"uid"`
	Netuid          int    `json:"netuid"`
	Active          bool   `json:"active"`
	Stake           string `json:"stake"`
	Trust           string `json:"trust"`
	Consensus       string `json:"consensus"`
	Incentive       string `json:"incentive"`
	Dividends       string `json:"dividends"`
	Emission        string `json:"emission"`
	ValidatorPermit bool   `json:"validator_permit"`
	ValidatorTrust  string `json:"validator_trust"`
	Rank            int    `json:"rank"`
	Hotkey          apiKey `json:"hotkey"`
	Coldkey         apiKey `json:"coldkey"`
	BlockNumber     int    `json:"block_number"`
	DailyReward     string `json:"daily_reward"`
	Updated         int    `json:"updated"`
	IsImmunityPeriod bool  `json:"is_immunity_period"`
	AlphaStake      string `json:"alpha_stake"`
	RootStake       string `json:"root_stake"`
	TotalAlphaStake string `json:"total_alpha_stake"`
}

type apiResponse struct {
	Data       []apiNeuron   `json:"data"`
	Pagination apiPagination `json:"pagination"`
}

type apiPagination struct {
	CurrentPage int  `json:"current_page"`
	NextPage    *int `json:"next_page"`
	PerPage     int  `json:"per_page"`
	TotalItems  int  `json:"total_items"`
	TotalPages  int  `json:"total_pages"`
}

// --- Bubbletea Messages ---

// MetagraphMsg carries the fetched metagraph data into the Bubbletea Update loop.
type MetagraphMsg struct {
	Neurons []types.Neuron
	NetUID  int
	Block   int
}

// MetagraphErrMsg carries a network error into the UI for display in the log pane.
type MetagraphErrMsg struct {
	Err error
}

// --- The tea.Cmd factory ---
// This is the public API. The UI calls FetchMetagraph() which returns a tea.Cmd.
// The Cmd runs in a goroutine (managed by Bubbletea runtime), fetches data,
// and returns a Msg that flows back into Update(). Zero channel plumbing needed.

// FetchMetagraph returns a tea.Cmd that fetches the metagraph for the given subnet.
// apiKey is the Taostats API key (passed as Authorization header, no "Bearer" prefix).
func FetchMetagraph(apiKeyStr string, netUID int) tea.Cmd {
	return func() tea.Msg {
		neurons, block, err := fetchMetagraphFromAPI(apiKeyStr, netUID)
		if err != nil {
			return MetagraphErrMsg{Err: err}
		}
		return MetagraphMsg{
			Neurons: neurons,
			NetUID:  netUID,
			Block:   block,
		}
	}
}

// --- Internal: HTTP fetch + parse ---

func fetchMetagraphFromAPI(apiKeyStr string, netUID int) ([]types.Neuron, int, error) {
	url := fmt.Sprintf(
		"%s/metagraph/latest/v1?netuid=%d&order=stake_desc&limit=%d",
		taostatsBaseURL, netUID, defaultLimit,
	)

	client := &http.Client{Timeout: requestTimeout}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("metagraph: build request: %w", err)
	}
	req.Header.Set("Authorization", apiKeyStr)

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("metagraph: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("metagraph: API returned %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("metagraph: read body: %w", err)
	}

	var apiResp apiResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, 0, fmt.Errorf("metagraph: decode JSON: %w", err)
	}

	neurons := make([]types.Neuron, 0, len(apiResp.Data))
	block := 0
	for _, n := range apiResp.Data {
		neuron := types.Neuron{
			UID:             n.UID,
			Netuid:          n.Netuid,
			Active:          n.Active,
			Stake:           raoToTao(n.Stake),
			Trust:           parseFloat(n.Trust),
			Consensus:       parseFloat(n.Consensus),
			Incentive:       parseFloat(n.Incentive),
			Dividends:       parseFloat(n.Dividends),
			Emission:        raoToTao(n.Emission),
			ValidatorPermit: n.ValidatorPermit,
			ValidatorTrust:  parseFloat(n.ValidatorTrust),
			Rank:            n.Rank,
			HotkeySS58:      n.Hotkey.SS58,
			ColdkeySS58:     n.Coldkey.SS58,
			BlockNumber:     n.BlockNumber,
			DailyReward:     raoToTao(n.DailyReward),
			Updated:         n.Updated,
			IsImmunity:      n.IsImmunityPeriod,
		}
		neurons = append(neurons, neuron)
		if n.BlockNumber > block {
			block = n.BlockNumber
		}
	}

	return neurons, block, nil
}

// raoToTao converts a RAO string (integer, 1 TAO = 1e9 RAO) to TAO float64.
func raoToTao(rao string) float64 {
	val, err := strconv.ParseFloat(rao, 64)
	if err != nil {
		return 0
	}
	return val / 1e9
}

// parseFloat safely parses a string float, returning 0 on failure.
func parseFloat(s string) float64 {
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return val
}
