// Package input defines the human-readable configuration types that blockchain
// developers and IBC relayers write. These configs are intentionally simple and
// focused on what humans can easily provide - the config_manager handles the rest
// by computing IBC denoms from the defined paths.
package input

// ChainInput is the human-readable chain configuration that developers write.
// This is parsed from TOML files in the chain_configs/ directory.
type ChainInput struct {
	Chain          ChainMeta       `toml:"chain"`
	Tokens         []TokenMeta     `toml:"token"`
	ReceivedTokens []ReceivedToken `toml:"received_token"`
}

// ChainMeta contains the basic chain identification and metadata.
// Most of this can be filled in by hand without querying any APIs.
type ChainMeta struct {
	// Required: Human-readable name (e.g., "Osmosis", "Cosmos Hub")
	Name string `toml:"name"`

	// Required: Chain ID (e.g., "osmosis-1", "cosmoshub-4")
	ID string `toml:"id"`

	// Required: Chain type - currently only "cosmos" is supported
	Type string `toml:"type"`

	// Required: Directory name in the cosmos chain-registry (e.g., "osmosis", "cosmoshub")
	// Used to fetch IBC channel data from github.com/cosmos/chain-registry
	Registry string `toml:"registry"`

	// Required: Block explorer URL for this chain
	ExplorerURL string `toml:"explorer_url"`

	// Required: SLIP-44 coin type (typically 118 for Cosmos chains)
	Slip44 int `toml:"slip44"`

	// Required: Bech32 address prefix (e.g., "osmo", "cosmos", "atone")
	Bech32Prefix string `toml:"bech32_prefix"`

	// Optional: Set to true if this chain is a DEX broker (e.g., Osmosis, Neutron(Astroport))
	IsBroker bool `toml:"is_broker,omitempty"`

	// Optional: Broker identifier, required if IsBroker is true (e.g., "osmosis-sqs")
	BrokerID string `toml:"broker_id,omitempty"`

	// Optional: Set to true if this chain supports Packet Forward Middleware
	// If not set, will be auto-detected during config generation
	HasPFM *bool `toml:"has_pfm,omitempty"`

	// RPC and REST endpoints
	RPCs []APIEndpoint `toml:"rpcs"`
	Rest []APIEndpoint `toml:"rest"`
}

// APIEndpoint represents an RPC or REST API endpoint.
type APIEndpoint struct {
	// Required: Full URL of the endpoint
	URL string `toml:"url"`

	// Optional: Provider name (e.g., "Cogwheel", "Polkachu")
	Provider string `toml:"provider,omitempty"`
}

// TokenMeta contains information about a NATIVE token on this chain.
// Only define tokens that are native to this chain unless it is a multi hop(might change in the future).
// IBC tokens will be computed based on routes and other chain configs.
type TokenMeta struct {
	// Required: The on-chain denom (e.g., "uosmo", "uatone")
	Denom string `toml:"denom"`

	// Required: Human-readable name (e.g., "Osmosis", "Atom One")
	Name string `toml:"name"`

	// Required: Human recognizable symbol (e.g., "OSMO", "ATONE")
	Symbol string `toml:"symbol"`

	// Required: Decimal places (typically 6 for Cosmos tokens)
	Exponent int `toml:"exponent"`

	// Required: URL or path to the token icon
	Icon string `toml:"icon"`

	// Optional: CoinGecko ID for price data
	CoinGeckoID string `toml:"coingecko_id,omitempty"`

	// Optional: Restrict this token to specific destination chains.
	// If empty, token can be sent to all connected chains.
	// Use chain IDs (e.g., ["osmosis-1", "cosmoshub-4"])
	AllowedDestinations []string `toml:"allowed_destinations,omitempty"`
}

// ReceivedToken defines a token that this chain receives from another chain
// through a multi-hop path. Use this for tokens that don't come directly
// from their origin chain.
//
// Example: Sei tokens on Noble that come via Osmosis
//
//	[[received_token]]
//	origin_denom = "usei"
//	origin_chain = "pacific-1"
//	via_chains = ["osmosis-1"]
type ReceivedToken struct {
	// Required: The original denom on the origin chain
	OriginDenom string `toml:"origin_denom"`

	// Required: Chain ID where this token is native
	OriginChain string `toml:"origin_chain"`

	// Required: Chain IDs this token travels through to reach us (in order)
	// For direct transfers, leave empty or omit.
	// For multi-hop: ["intermediate-chain-1", "intermediate-chain-2"]
	ViaChains []string `toml:"via_chains"`

	// Optional: Override display name for this token on our chain
	DisplayName string `toml:"display_name,omitempty"`

	// Optional: Override symbol for this token on our chain
	DisplaySymbol string `toml:"display_symbol,omitempty"`
}
