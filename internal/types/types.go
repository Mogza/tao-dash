package types

// Neuron represents a single neuron in a Bittensor subnet metagraph.
// Fields map to the Taostats API response (api/metagraph/latest/v1).
// Stake/Emission values come as string RAO from the API and are converted to float64 TAO.
type Neuron struct {
	UID             int     `json:"uid"`
	Netuid          int     `json:"netuid"`
	Active          bool    `json:"active"`
	Stake           float64 // Converted from RAO string → TAO (÷1e9)
	Trust           float64 // Parsed from string "0.xxxxx"
	Consensus       float64
	Incentive       float64
	Dividends       float64
	Emission        float64 // Converted from RAO string → TAO (÷1e9)
	ValidatorPermit bool    `json:"validator_permit"`
	ValidatorTrust  float64
	Rank            int     `json:"rank"`
	HotkeySS58      string
	ColdkeySS58     string
	BlockNumber     int     `json:"block_number"`
	DailyReward     float64 // Converted from RAO string → TAO (÷1e9)
	Updated         int     `json:"updated"`
	IsImmunity      bool    `json:"is_immunity_period"`
}
