// Package input defines the human-readable configuration types that blockchain
// developers and IBC relayers write. These configs are intentionally simple and
// focused on what humans can easily provide - the config_manager handles the rest
// by computing IBC denoms from the defined paths.
package input

import "github.com/Cogwheel-Validator/spectra-portal/config_manager/keplr"

// ChainInput is the human-readable chain configuration that developers write.
// This is parsed from TOML files in the chain_configs/ directory.
type ChainInput struct {
	Chain  ChainMeta   `toml:"chain"`
	Tokens []TokenMeta `toml:"token"`
}

// ChainMeta contains the basic chain identification and metadata.
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

	// Required: Keplr json file name that is loacted in the chainapsis github repository
	// keplr-chain-registry/cosmos/chainname.json,
	// it is requried to generate the config for the client config
	//
	// However in the events there is some need to overwrite keplr config, or it
	// is not yet approved by the keplr team or pushed to the keplr github repo
	// there will be a field that will allow
	KeplrJSONFileName *string `toml:"keplr_json"`

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

	// Optional: IBC hooks contract address for swap operations
	// This is the entry point contract for wasm swap_and_action (e.g., Osmosis entry point)
	// Required if IsBroker is true and you want to generate executable swap memos
	IBCHooksContract string `toml:"ibc_hooks_contract,omitempty"`

	// RPC and REST endpoints
	RPCs []APIEndpoint `toml:"rpcs"`
	Rest []APIEndpoint `toml:"rest"`

	// Optional: Keplr chain config that will be used to overwrite the existing keplr config if this is the case
	//
	// Usage:
	//
	// Mostly will be used for the adding custom chains to the wallets
	KeplrChainConfig *keplr.KeplrChainConfig `toml:"keplr_chain_config,omitempty"`
}

// APIEndpoint represents an RPC or REST API endpoint.
type APIEndpoint struct {
	// Required: Full URL of the endpoint
	URL string `toml:"url"`

	// Optional: Provider name (e.g., "Cogwheel", "Polkachu")
	Provider string `toml:"provider,omitempty"`
}

// TokenMeta contains information about a token on this chain.
//
// There are two types of tokens you can define:
//
// 1. NATIVE tokens - tokens that originate on this chain:
//
//		[[token]]
//		denom = "uatone"
//		name = "Atone"
//		symbol = "ATONE"
//		exponent = 6
//		allowed_destinations = ["osmosis-1", "stargaze-1"]
//
//	 2. ROUTABLE IBC tokens - IBC tokens received from another chain that you want
//	    to forward to other destinations (multi-hop support):
//
//	    [[token]]
//	    denom = "ibc/ABC123..."      # The IBC denom ON THIS CHAIN
//	    name = "Atone"
//	    symbol = "ATONE"
//	    exponent = 6
//	    origin_chain = "atomone-1"   # Where the token is truly native
//	    origin_denom = "uatone"      # Native denom on origin chain
//	    allowed_destinations = ["osmosis-1"]  # Can ONLY forward to these chains
//
// This allows explicit control over multi-hop routing.
type TokenMeta struct {
	// Required: The on-chain denom
	// For native tokens: "uatone", "uosmo"
	// For routable IBC tokens: "ibc/ABC123..." (the IBC hash on this chain)
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

	// === Multi-hop support ===
	// Set these fields for IBC tokens that you want to forward (route through this chain)

	// Optional: Chain ID where this token is truly native.
	// If set, this token is treated as a "routable IBC token" not a native token.
	// Example: "atomone-1" for ATONE
	OriginChain string `toml:"origin_chain,omitempty"`

	// Optional: The native denom on the origin chain.
	// Required if OriginChain is set.
	// Example: "uatone"
	OriginDenom string `toml:"origin_denom,omitempty"`
}

// IsNative returns true if this token is native to the chain (not an IBC token being forwarded)
func (t *TokenMeta) IsNative() bool {
	return t.OriginChain == ""
}

// IsRoutableIBC returns true if this is an IBC token that can be forwarded to other chains
func (t *TokenMeta) IsRoutableIBC() bool {
	return t.OriginChain != ""
}

type ExplorerMeta struct {
	AllowedExplorers []AllowedExplorer `toml:"allowed_explorers"`
}

type AllowedExplorer struct {
	Name                 string `toml:"name"`
	BaseURL              string `toml:"base_url"`
	MultiChainSupport    bool   `toml:"multi_chain_support"`
	TransactionPath      string `toml:"transaction_path"`
	AccountPath          string `toml:"account_path"`
	IsFork               bool   `toml:"is_fork"`
	ForkName             string `toml:"fork_name"`
	ForkGithubRepository string `toml:"fork_github_repository"`
	ValidatorOrChainName string `toml:"validator_or_chain_name"`
	WebsiteURL           string `toml:"website_url"`
	ContactEmail         string `toml:"contact_email"`
	GithubProfile        string `toml:"github_profile"`
}

type IbcParams struct {
	Params struct {
		SendEnabled    bool `json:"send_enabled"`
		ReceiveEnabled bool `json:"receive_enabled"`
	} `json:"params"`
}
