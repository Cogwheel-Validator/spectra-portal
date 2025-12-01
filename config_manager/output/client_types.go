package output

// ClientConfig contains all configuration needed by the frontend client application.
// This focuses on user-facing data like display names, icons, and explorer URLs.
type ClientConfig struct {
	// Version of the config format
	Version string `json:"version"`

	// When this config was generated
	GeneratedAt string `json:"generated_at"`

	// All chains available in the app
	Chains []ClientChain `json:"chains"`

	// Quick lookup: all unique tokens across all chains
	AllTokens []ClientTokenSummary `json:"all_tokens"`
}

// ClientChain contains chain information for the frontend.
type ClientChain struct {
	// Human-readable chain name
	Name string `json:"name"`

	// Chain ID (e.g., "osmosis-1")
	ID string `json:"id"`

	// Bech32 address prefix
	Bech32Prefix string `json:"bech32_prefix"`

	// SLIP-44 coin type
	Slip44 int `json:"slip44"`

	// Block explorer URL
	ExplorerURL string `json:"explorer_url"`

	// URL/path to chain logo
	ChainLogo string `json:"chain_logo,omitempty"`

	// Whether this chain is a DEX (for UI hints)
	IsDEX bool `json:"is_dex"`

	// RPC endpoints for wallet connections
	RPCEndpoints []ClientEndpoint `json:"rpc_endpoints"`

	// REST endpoints for queries
	RESTEndpoints []ClientEndpoint `json:"rest_endpoints"`

	// Native tokens on this chain
	NativeTokens []ClientToken `json:"native_tokens"`

	// IBC tokens available on this chain
	IBCTokens []ClientToken `json:"ibc_tokens"`

	// Chains this chain can send tokens to
	ConnectedChains []ConnectedChainInfo `json:"connected_chains"`
}

// ClientEndpoint represents an API endpoint for the frontend.
type ClientEndpoint struct {
	URL      string `json:"url"`
	Provider string `json:"provider,omitempty"`
}

// ClientToken contains token display information for the frontend.
type ClientToken struct {
	// On-chain denom (native or IBC hash)
	Denom string `json:"denom"`

	// Human-readable name
	Name string `json:"name"`

	// Trading symbol
	Symbol string `json:"symbol"`

	// Decimal places
	Decimals int `json:"decimals"`

	// URL to token icon
	Icon string `json:"icon"`

	// Chain ID where this token is native (for display purposes)
	OriginChain string `json:"origin_chain"`

	// Origin chain name (for display)
	OriginChainName string `json:"origin_chain_name,omitempty"`

	// CoinGecko ID for price lookups (optional)
	CoinGeckoID string `json:"coingecko_id,omitempty"`

	// Whether this is a native token on the current chain
	IsNative bool `json:"is_native"`

	// Base denom (for IBC tokens, the original denom)
	BaseDenom string `json:"base_denom,omitempty"`
}

// ConnectedChainInfo contains minimal info about connected chains for UI.
type ConnectedChainInfo struct {
	// Chain ID
	ID string `json:"id"`

	// Chain name for display
	Name string `json:"name"`

	// Chain logo
	Logo string `json:"logo,omitempty"`

	// Tokens that can be sent to this chain
	SendableTokens []string `json:"sendable_tokens"`
}

// ClientTokenSummary provides a quick reference for all unique tokens.
type ClientTokenSummary struct {
	// Base denom (original native denom)
	BaseDenom string `json:"base_denom"`

	// Trading symbol
	Symbol string `json:"symbol"`

	// Human-readable name
	Name string `json:"name"`

	// URL to token icon
	Icon string `json:"icon"`

	// Chain ID where this token is native
	OriginChain string `json:"origin_chain"`

	// Origin chain name
	OriginChainName string `json:"origin_chain_name"`

	// Chain IDs where this token is available
	AvailableOn []string `json:"available_on"`

	// CoinGecko ID for price lookups
	CoinGeckoID string `json:"coingecko_id,omitempty"`
}

