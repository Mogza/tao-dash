package network

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"Mogza/TaoDash/internal/cache"
	"Mogza/TaoDash/internal/types"

	tea "github.com/charmbracelet/bubbletea"
)

// --- Configuration ---

const (
	taostatsBaseURL = "https://api.taostats.io/api"
	defaultLimit    = 256
	requestTimeout  = 10 * time.Second
)

// --- Taostats API JSON shapes ---

type apiKey struct {
	Hex  string `json:"hex"`
	SS58 string `json:"ss58"`
}

type apiNeuron struct {
	UID              int    `json:"uid"`
	Netuid           int    `json:"netuid"`
	Active           bool   `json:"active"`
	Stake            string `json:"stake"`
	Trust            string `json:"trust"`
	Consensus        string `json:"consensus"`
	Incentive        string `json:"incentive"`
	Dividends        string `json:"dividends"`
	Emission         string `json:"emission"`
	ValidatorPermit  bool   `json:"validator_permit"`
	ValidatorTrust   string `json:"validator_trust"`
	Rank             int    `json:"rank"`
	Hotkey           apiKey `json:"hotkey"`
	Coldkey          apiKey `json:"coldkey"`
	BlockNumber      int    `json:"block_number"`
	DailyReward      string `json:"daily_reward"`
	Updated          int    `json:"updated"`
	IsImmunityPeriod bool   `json:"is_immunity_period"`
	AlphaStake       string `json:"alpha_stake"`
	RootStake        string `json:"root_stake"`
	TotalAlphaStake  string `json:"total_alpha_stake"`
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

// MetagraphMsg transporte les données du metagraph vers le Update() de l'UI.
type MetagraphMsg struct {
	Neurons   []types.Neuron
	NetUID    int
	Block     int
	FromCache bool // true = servi depuis Redis, false = frais depuis l'API
}

// MetagraphErrMsg transporte une erreur réseau vers les logs de l'UI.
type MetagraphErrMsg struct {
	Err error
}

// --- Public API ---

// FetchMetagraph retourne un tea.Cmd qui :
// 1. Vérifie le cache Redis si disponible (clé = netuid + blockNumber)
// 2. En cas de miss, tape l'API Taostats
// 3. Stocke le résultat dans Redis avec une TTL de sécurité
//
// blockNumber = 0 → skip le check cache, force un appel API (utile au démarrage)
func FetchMetagraph(apiKeyStr string, netUID int, blockNumber int, c *cache.Client) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		// Cache check : seulement si on connaît le numéro de bloc courant
		if c != nil && blockNumber > 0 {
			if neurons, ok := c.GetMetagraph(ctx, netUID, blockNumber); ok {
				return MetagraphMsg{
					Neurons:   neurons,
					NetUID:    netUID,
					Block:     blockNumber,
					FromCache: true,
				}
			}
		}

		// Cache miss ou pas de cache : appel API
		neurons, block, err := fetchMetagraphFromAPI(apiKeyStr, netUID)
		if err != nil {
			return MetagraphErrMsg{Err: err}
		}

		// Stockage dans Redis (keyed par le bloc retourné par l'API, pas par blockNumber)
		if c != nil {
			_ = c.SetMetagraph(ctx, netUID, block, neurons)
		}

		return MetagraphMsg{
			Neurons:   neurons,
			NetUID:    netUID,
			Block:     block,
			FromCache: false,
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

// raoToTao convertit un RAO string (1 TAO = 1e9 RAO) en TAO float64.
func raoToTao(rao string) float64 {
	val, err := strconv.ParseFloat(rao, 64)
	if err != nil {
		return 0
	}
	return val / 1e9
}

// parseFloat parse un string float, retourne 0 en cas d'erreur.
func parseFloat(s string) float64 {
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return val
}
