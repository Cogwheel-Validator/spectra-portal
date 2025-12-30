// Package output defines the generated configuration types for both the
// backend (Pathfinder) and frontend (Client) applications.
package output

// PathfinderConfig contains all configuration needed by the pathfinder backend.
// This is the top-level config that gets loaded at startup.
type PathfinderConfig struct {
	// Version of the config format
	Version string `json:"version" toml:"version"`

	// When this config was generated
	GeneratedAt string `json:"generated_at" toml:"generated_at"`

	// All chains available for routing
	Chains []PathfinderChain `json:"chains" toml:"chains"`
}

// PathfinderChain represents a chain in the pathfinder's routing graph.
// This maps directly to the router.PathfinderChain type.
type PathfinderChain struct {
	// Human-readable chain name
	Name string `json:"name" toml:"name"`

	// Chain ID (e.g., "osmosis-1")
	ID string `json:"id" toml:"id"`

	// Whether this chain supports Packet Forward Middleware
	HasPFM bool `json:"has_pfm" toml:"has_pfm"`

	// Whether this chain is a DEX broker (for swap routing)
	Broker bool `json:"broker" toml:"broker"`

	// Broker identifier (e.g., "osmosis-sqs") - required if Broker is true
	BrokerID string `json:"broker_id,omitempty" toml:"broker_id,omitempty"`

	// IBC hooks contract address for swap operations (e.g., Osmosis entry point contract)
	// This is used to build the wasm memo for swap_and_action operations
	IBCHooksContract string `json:"ibc_hooks_contract,omitempty" toml:"ibc_hooks_contract,omitempty"`

	// Bech32 prefix for addresses on this chain (e.g., "osmo", "cosmos")
	Bech32Prefix string `json:"bech32_prefix,omitempty" toml:"bech32_prefix,omitempty"`

	// Available IBC routes from this chain
	Routes []PathfinderRoute `json:"routes" toml:"routes"`
}

// PathfinderRoute represents an IBC route in the pathfinder's routing graph.
// This maps directly to the router.BasicRoute type.
type PathfinderRoute struct {
	// Destination chain name
	ToChain string `json:"to_chain" toml:"to_chain"`

	// Destination chain ID
	ToChainID string `json:"to_chain_id" toml:"to_chain_id"`

	// IBC connection ID on source chain
	ConnectionID string `json:"connection_id" toml:"connection_id"`

	// IBC channel ID on source chain
	ChannelID string `json:"channel_id" toml:"channel_id"`

	// IBC port (typically "transfer")
	PortID string `json:"port_id" toml:"port_id"`

	// Tokens that can be transferred on this route
	AllowedTokens map[string]PathfinderTokenInfo `json:"allowed_tokens" toml:"allowed_tokens"`
}

// PathfinderTokenInfo contains token information for routing decisions.
// This maps directly to the router.TokenInfo type.
type PathfinderTokenInfo struct {
	// Denom on the source chain in the route context
	ChainDenom string `json:"chain_denom" toml:"chain_denom"`

	// Denom on the destination chain (after IBC transfer)
	IBCDenom string `json:"ibc_denom" toml:"ibc_denom"`

	// Original native denom on the token's origin chain
	BaseDenom string `json:"base_denom" toml:"base_denom"`

	// Chain ID where this token is native
	OriginChain string `json:"origin_chain" toml:"origin_chain"`

	// Human-readable symbol (e.g., "ATOM", "OSMO")
	Symbol string `json:"symbol,omitempty" toml:"symbol,omitempty"`

	// Number of decimal places
	Decimals int `json:"decimals" toml:"decimals"`
}
