package output

import (
	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/keplr"
)

// ClientConfig contains all configuration needed by the frontend client application.
// This focuses on user-facing data like display names, icons, and explorer URLs.
type ClientConfig struct {
	// Version of the config format
	Version string `json:"version" toml:"version"`

	// When this config was generated
	GeneratedAt string `json:"generated_at" toml:"generated_at"`

	// All chains available in the app
	Chains []ClientChain `json:"chains" toml:"chains"`

	// Quick lookup: all unique tokens across all chains
	AllTokens []ClientTokenSummary `json:"all_tokens" toml:"all_tokens"`
}

// ClientChain contains chain information for the frontend.
type ClientChain struct {
	// Human-readable chain name
	Name string `json:"name" toml:"name"`

	// Chain ID (e.g., "osmosis-1")
	ID string `json:"id" toml:"id"`

	// Bech32 address prefix
	Bech32Prefix string `json:"bech32_prefix" toml:"bech32_prefix"`

	// SLIP-44 coin type
	Slip44 int `json:"slip44" toml:"slip44"`

	// Block explorer details
	ExplorerDetails ExplorerDetails `json:"explorer_details" toml:"explorer_details"`

	// URL/path to chain logo
	ChainLogo string `json:"chain_logo,omitempty" toml:"chain_logo,omitempty"`

	// Whether this chain is a DEX (for UI hints)
	IsDEX bool `json:"is_dex" toml:"is_dex"`

	// Cosmos SDK version
	CosmosSdkVersion string `json:"cosmos_sdk_version" toml:"cosmos_sdk_version"`

	// RPC endpoints for wallet connections
	RPCEndpoints []ClientEndpoint `json:"rpc_endpoints" toml:"rpc_endpoints"`

	// REST endpoints for queries
	RESTEndpoints []ClientEndpoint `json:"rest_endpoints" toml:"rest_endpoints"`

	// Native tokens on this chain
	NativeTokens []ClientToken `json:"native_tokens" toml:"native_tokens"`

	// IBC tokens available on this chain
	IBCTokens []ClientToken `json:"ibc_tokens" toml:"ibc_tokens"`

	// Keplr data for wallets
	KeplrChainConfig keplr.KeplrChainConfig `json:"keplr_chain_config" toml:"keplr_chain_config"`

	// Chains this chain can send tokens to
	ConnectedChains []ConnectedChainInfo `json:"connected_chains" toml:"connected_chains"`
}

// Explorer details for the client app such as url link to account and transaction
type ExplorerDetails struct {
	BaseUrl         string `json:"base_url" toml:"base_url"`
	AccountPath     string `json:"account_path" toml:"account_path"`
	TransactionPath string `json:"transaction_path" toml:"transaction_path"`
}

// ClientEndpoint represents an API endpoint for the frontend.
type ClientEndpoint struct {
	URL      string `json:"url" toml:"url"`
	Provider string `json:"provider,omitempty" toml:"provider,omitempty"`
}

// ClientToken contains token display information for the frontend.
type ClientToken struct {
	// On-chain denom (native or IBC hash)
	Denom string `json:"denom" toml:"denom"`

	// Human-readable name
	Name string `json:"name" toml:"name"`

	// Trading symbol
	Symbol string `json:"symbol" toml:"symbol"`

	// Decimal places
	Decimals int `json:"decimals" toml:"decimals"`

	// URL to token icon
	Icon string `json:"icon" toml:"icon"`

	// Chain ID where this token is native (for display purposes)
	OriginChain string `json:"origin_chain" toml:"origin_chain"`

	// Origin chain name (for display)
	OriginChainName string `json:"origin_chain_name,omitempty" toml:"origin_chain_name,omitempty"`

	// CoinGecko ID for price lookups (optional)
	CoinGeckoID string `json:"coingecko_id,omitempty" toml:"coingecko_id,omitempty"`

	// Whether this is a native token on the current chain
	IsNative bool `json:"is_native" toml:"is_native"`

	// Base denom (for IBC tokens, the original denom)
	BaseDenom string `json:"base_denom,omitempty" toml:"base_denom,omitempty"`
}

// ConnectedChainInfo contains minimal info about connected chains for UI.
type ConnectedChainInfo struct {
	// Chain ID
	ID string `json:"id" toml:"id"`

	// Chain name for display
	Name string `json:"name" toml:"name"`

	// Chain logo
	Logo string `json:"logo,omitempty" toml:"logo,omitempty"`

	// Tokens that can be sent to this chain
	SendableTokens []string `json:"sendable_tokens" toml:"sendable_tokens"`
}

// ClientTokenSummary provides a quick reference for all unique tokens.
type ClientTokenSummary struct {
	// Base denom (original native denom)
	BaseDenom string `json:"base_denom" toml:"base_denom"`

	// Trading symbol
	Symbol string `json:"symbol" toml:"symbol"`

	// Human-readable name
	Name string `json:"name" toml:"name"`

	// URL to token icon
	Icon string `json:"icon" toml:"icon"`

	// Chain ID where this token is native
	OriginChain string `json:"origin_chain" toml:"origin_chain"`

	// Origin chain name
	OriginChainName string `json:"origin_chain_name" toml:"origin_chain_name"`

	// Chain IDs where this token is available
	AvailableOn []string `json:"available_on" toml:"available_on"`

	// CoinGecko ID for price lookups
	CoinGeckoID string `json:"coingecko_id,omitempty" toml:"coingecko_id,omitempty"`
}
