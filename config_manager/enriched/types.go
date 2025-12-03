// Package enriched defines the intermediate configuration types that contain
// data enriched from RPC/REST endpoints and the IBC chain registry.
// These types serve as the canonical source of truth after processing
// human-readable configs and fetching all required blockchain data.
package enriched

// ChainConfig is the fully enriched chain configuration after processing.
// It contains all data needed to generate both backend and frontend configs.
type ChainConfig struct {
	// Basic chain identification (from input config)
	Name         string `json:"name"`
	ID           string `json:"id"`
	Type         string `json:"type"`
	Registry     string `json:"registry"`
	ExplorerURL  string `json:"explorer_url"`
	Slip44       int    `json:"slip44"`
	Bech32Prefix string `json:"bech32_prefix"`

	// Broker capabilities
	IsBroker bool   `json:"is_broker"`
	BrokerID string `json:"broker_id,omitempty"`

	// Middleware support (detected from chain or manually set)
	HasPFM bool `json:"has_pfm"`

	// Verified endpoints (after health checks)
	HealthyRPCs  []Endpoint `json:"healthy_rpcs"`
	HealthyRests []Endpoint `json:"healthy_rests"`

	// Native tokens on this chain
	NativeTokens []TokenConfig `json:"native_tokens"`

	// IBC tokens received from other chains
	IBCTokens []IBCTokenConfig `json:"ibc_tokens"`

	// IBC routes to other chains
	Routes []RouteConfig `json:"routes"`
}

// Endpoint represents a verified RPC or REST endpoint.
type Endpoint struct {
	URL      string `json:"url"`
	Provider string `json:"provider,omitempty"`
	Healthy  bool   `json:"healthy"`
}

// TokenConfig contains comprehensive token information.
type TokenConfig struct {
	// On-chain denom (e.g., "uosmo", "uatom")
	Denom string `json:"denom"`

	// Human-readable name (e.g., "Osmosis", "Cosmos")
	Name string `json:"name"`

	// Trading symbol (e.g., "OSMO", "ATOM")
	Symbol string `json:"symbol"`

	// Decimal places (typically 6)
	Decimals int `json:"decimals"`

	// URL to token icon
	Icon string `json:"icon"`

	// CoinGecko ID for price data (optional)
	CoinGeckoID string `json:"coingecko_id,omitempty"`

	// Chain ID where this token is native
	OriginChain string `json:"origin_chain"`

	// Allowed destination chains (empty means all)
	AllowedDestinations []string `json:"allowed_destinations,omitempty"`
}

// IBCTokenConfig represents a token received via IBC from another chain.
type IBCTokenConfig struct {
	// The IBC denom on this chain (e.g., "ibc/ABC123...")
	IBCDenom string `json:"ibc_denom"`

	// Original denom on the origin chain (e.g., "uatom")
	BaseDenom string `json:"base_denom"`

	// Human-readable name
	Name string `json:"name"`

	// Trading symbol
	Symbol string `json:"symbol"`

	// Decimal places
	Decimals int `json:"decimals"`

	// URL to token icon
	Icon string `json:"icon"`

	// Chain ID where this token is native
	OriginChain string `json:"origin_chain"`

	// IBC path used to reach this chain (e.g., "transfer/channel-0")
	IBCPath string `json:"ibc_path"`

	// Channel ID this token arrived on
	SourceChannel string `json:"source_channel"`
}

// RouteConfig represents an IBC route from this chain to another.
type RouteConfig struct {
	// Destination chain ID
	ToChainID string `json:"to_chain_id"`

	// Destination chain name
	ToChainName string `json:"to_chain_name"`

	// IBC connection ID on this chain
	ConnectionID string `json:"connection_id"`

	// IBC channel ID on this chain
	ChannelID string `json:"channel_id"`

	// IBC port (typically "transfer")
	PortID string `json:"port_id"`

	// Counterparty channel on the destination chain
	CounterpartyChannelID string `json:"counterparty_channel_id"`

	// Channel ordering (typically "unordered")
	Ordering string `json:"ordering"`

	// Channel status
	State string `json:"state"`

	// ICS version
	Version string `json:"version"`

	// Tokens that can be sent on this route
	AllowedTokens []RouteTokenInfo `json:"allowed_tokens"`
}

// RouteTokenInfo contains token information specific to a route.
type RouteTokenInfo struct {
	// Denom on the source chain (current chain)
	SourceDenom string `json:"source_denom"`

	// What the denom becomes on the destination chain
	DestinationDenom string `json:"destination_denom"`

	// Original base denom (for IBC tokens)
	BaseDenom string `json:"base_denom"`

	// Chain where token is native
	OriginChain string `json:"origin_chain"`

	// Decimal places
	Decimals int `json:"decimals"`
}

// RegistryConfig holds the complete enriched configuration for all chains.
type RegistryConfig struct {
	// Version of the config format
	Version string `json:"version"`

	// When this config was generated
	GeneratedAt string `json:"generated_at"`

	// All chains in the registry
	Chains map[string]*ChainConfig `json:"chains"`
}
