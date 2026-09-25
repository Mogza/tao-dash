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
	"golang.org/x/sync/singleflight"
)

// --- Configuration ---

// taostatsBaseURL is a var (not const) to allow mock injection in tests.
var taostatsBaseURL = "https://api.taostats.io/api"

// SetBaseURL overrides the API base URL — reserved for tests and the stress test.
func SetBaseURL(url string) { taostatsBaseURL = url }

const (
	defaultLimit   = 256
	requestTimeout = 10 * time.Second
)

// sfGroup collapses concurrent requests for the same key into a single real call.
var sfGroup singleflight.Group

// --- Sort options ---

// SortOption describes a column sort configuration.
type SortOption struct {
	APIValue    string // value for the Taostats API `order` query param
	Label       string // display label shown in the table header
	ColumnIndex int    // which column (0-based) gets the sort indicator arrow
}

// SortOptions is the ordered list of available sorts, cycled with the [s] key.
var SortOptions = []SortOption{
	{"stake_desc", "STAKE ↓", 2},
	{"emission_desc", "EMISSION ↓", 4},
	{"consensus_desc", "CONSENSUS ↓", 3},
	{"dividends_desc", "DIVIDND ↓", 5},
	{"uid_asc", "UID ↑", 0},
}

// --- Fetch options ---

// FetchOptions bundles all parameters that define a unique metagraph query.
// Changing any field produces a distinct cache key.
type FilterMode string

const (
	FilterAll        FilterMode = "ALL"
	FilterValidators FilterMode = "VALIDATORS"
	FilterMiners     FilterMode = "MINERS"
)

type FetchOptions struct {
	NetUID      int
	BlockNumber int    // 0 = skip cache, force API call
	SortOrder   string // must match a SortOption.APIValue
	Filter      FilterMode
}

// CacheKey returns the Redis key for a given set of fetch options.
// Exported so the stress test can flush the right keys between scenarios.
func CacheKey(opts FetchOptions) string {
	return fmt.Sprintf("taodash:metagraph:%d:%d:%s:%s",
		opts.NetUID, opts.BlockNumber, opts.SortOrder, opts.Filter)
}

// --- Bubbletea messages ---

// MetagraphMsg carries fetched neuron data to the UI Update().
type MetagraphMsg struct {
	Neurons   []types.Neuron
	NetUID    int
	Block     int
	FromCache bool // true = served from Redis
}

// MetagraphErrMsg carries a network error to the UI logs.
type MetagraphErrMsg struct {
	Err error
}

// --- Internal singleflight result ---

type sfResult struct {
	neurons []types.Neuron
	block   int
}

// --- Public API ---

// FetchMetagraph returns a tea.Cmd that:
//  1. Checks Redis (cache hit → immediate return, no singleflight overhead)
//  2. Via singleflight: collapses concurrent requests for the same key into 1 API call
//  3. Stores the result in Redis keyed by the block number the API returns
//
// opts.BlockNumber = 0 → skip cache, force an API call (startup, subnet switch, filter change)
func FetchMetagraph(apiKeyStr string, opts FetchOptions, c *cache.Client) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		// 1. Cache check — before singleflight to avoid lock overhead on hits
		if c != nil && opts.BlockNumber > 0 {
			if neurons, ok := c.Get(ctx, CacheKey(opts)); ok {
				return MetagraphMsg{
					Neurons:   neurons,
					NetUID:    opts.NetUID,
					Block:     opts.BlockNumber,
					FromCache: true,
				}
			}
		}

		// 2. Singleflight — one API call even if 50 goroutines arrive simultaneously
		sfKey := fmt.Sprintf("%d:%d:%s:%s", opts.NetUID, opts.BlockNumber, opts.SortOrder, opts.Filter)
		val, err, _ := sfGroup.Do(sfKey, func() (any, error) {
			neurons, block, err := fetchMetagraphFromAPI(apiKeyStr, opts)
			if err != nil {
				return nil, err
			}
			// Store with the block number returned by the API as key
			if c != nil {
				storeOpts := opts
				storeOpts.BlockNumber = block
				_ = c.Set(ctx, CacheKey(storeOpts), neurons)
			}
			return &sfResult{neurons: neurons, block: block}, nil
		})

		if err != nil {
			return MetagraphErrMsg{Err: err}
		}

		r := val.(*sfResult)
		return MetagraphMsg{
			Neurons:   r.neurons,
			NetUID:    opts.NetUID,
			Block:     r.block,
			FromCache: false,
		}
	}
}

// --- Internal: HTTP fetch + parse ---

func fetchMetagraphFromAPI(apiKeyStr string, opts FetchOptions) ([]types.Neuron, int, error) {
	validatorsParam := ""
	if opts.Filter == FilterValidators {
		validatorsParam = "&validator_permit=true"
	} else if opts.Filter == FilterMiners {
		validatorsParam = "&validator_permit=false"
	}

	url := fmt.Sprintf(
		"%s/metagraph/latest/v1?netuid=%d&order=%s&limit=%d%s",
		taostatsBaseURL, opts.NetUID, opts.SortOrder, defaultLimit, validatorsParam,
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

// raoToTao converts a RAO string (1 TAO = 1e9 RAO) to TAO float64.
func raoToTao(rao string) float64 {
	val, err := strconv.ParseFloat(rao, 64)
	if err != nil {
		return 0
	}
	return val / 1e9
}

// parseFloat parses a float string, returns 0 on error.
func parseFloat(s string) float64 {
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return val
}

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
