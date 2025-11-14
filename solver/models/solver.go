package models

// RouteRequest - POST body
type RouteRequest struct {
	ChainFrom         string `json:"chain_from"`         // e.g., "juno"
	TokenFromDenom   string `json:"token_from_denom"`   // e.g., "ujuno"
	AmountIn       string `json:"amount_in"`       // e.g., "1000000"
	ChainTo         string `json:"chain_to"`         // e.g., "cosmoshub"
	TokenToDenom  string `json:"token_to_denom"`  // e.g., "uatom"
	SenderAddress  string `json:"sender_address"`  // For validation
	ReceiverAddress string `json:"receiver_address"` // e.g., "cosmos1234567890"
}


// TokenMapping represents how a token transforms between chains
type TokenMapping struct {
	ChainDenom string `json:"chain_denom"` // Denom on this specific chain (native or IBC)
	BaseDenom  string `json:"base_denom"`  // Original native denom on origin chain
	OriginChain string `json:"origin_chain"` // Where this token originally comes from
	IsNative   bool   `json:"is_native"`   // True if this token is native to the current chain
}

// IBCLeg represents one IBC transfer leg in a route
type IBCLeg struct {
	FromChain   string        `json:"from_chain"`   // Chain ID
	ToChain     string        `json:"to_chain"`     // Chain ID
	Channel     string        `json:"channel_id"`     // Source channel
	Port        string        `json:"port"`        // Source port (usually "transfer")
	Token       *TokenMapping `json:"token_mapping"`       // Token info on source chain
	Amount      string        `json:"amount"`      // Amount to transfer
}

// DirectRoute represents a simple IBC transfer
type DirectRoute struct {
	Transfer *IBCLeg `json:"transfer"` // Single IBC transfer
}

// SwapQuote represents the swap information from a broker
type SwapQuote struct {
	Broker       string      `json:"broker"`       // Broker type (e.g., "osmosis")
	TokenIn      *TokenMapping `json:"token_in_mapping"`      // Input token on broker chain
	TokenOut     *TokenMapping `json:"token_out_mapping"`     // Output token on broker chain
	AmountIn     string      `json:"amount_in"`     // Actual input amount
	AmountOut    string      `json:"amount_out"`    // Estimated output amount
	PriceImpact  string      `json:"price_impact"`  // Price impact (e.g., "0.02" for 2%)
	EffectiveFee string      `json:"effective_fee"` // Total fees
	RouteData    any `json:"route_data"`    // Broker-specific route data (pools, hops, etc.)
}

// IndirectRoute represents a multi-hop route without swaps (same token across chains)
type IndirectRoute struct {
	Path            []string  `json:"path"`             // Chain IDs in order: [A, B, C, ...]
	Legs            []*IBCLeg `json:"legs"`             // IBC transfer legs
	SupportsPFM     bool      `json:"supports_pfm"`     // Can use PFM for single-tx execution
	PFMStartChain   string    `json:"pfm_start_chain,omitempty"`   // Chain where PFM forwarding starts
	PFMMemo         string    `json:"pfm_memo,omitempty"`          // IBC memo for PFM forwarding
}

// BrokerRoute represents a route that requires a swap on a broker chain
type BrokerRoute struct {
	Path                []string    `json:"path"`                  // Chain IDs in order: [sourceChain, brokerChain, destChain]
	InboundLeg          *IBCLeg     `json:"inbound_leg"`           // Source -> Broker
	Swap                *SwapQuote  `json:"swap"`                  // Swap on broker
	OutboundLeg         *IBCLeg     `json:"outbound_leg"`          // Broker -> Destination
	OutboundSupportsPFM bool        `json:"outbound_supports_pfm"` // Can broker forward to destination via PFM
	OutboundPFMMemo     string      `json:"outbound_pfm_memo,omitempty"` // PFM memo if supported
}

// RouteResponse - unified response for all route types (informative, not prescriptive)
type RouteResponse struct {
	Success      bool           `json:"success"`
	RouteType    string         `json:"route_type"` // "direct" | "indirect" | "broker_swap" | "impossible"
	ErrorMessage string         `json:"error_message,omitempty"`
	Direct       *DirectRoute   `json:"direct_route,omitempty"`
	Indirect     *IndirectRoute `json:"indirect_route,omitempty"`
	BrokerSwap   *BrokerRoute   `json:"broker_swap,omitempty"`
}

// DenomLookupRequest - request to lookup denom information
type DenomLookupRequest struct {
	Denom   string `json:"token_denom"`   // Can be native (uatom) or IBC (ibc/ABC123...)
	ChainID string `json:"chain_id"` // Which chain to resolve the denom for
}

// DenomInfo - detailed information about a token denom
type DenomInfo struct {
	ChainDenom  string `json:"chain_denom"`  // Denom on the specified chain
	BaseDenom   string `json:"base_denom"`   // Original native denom
	OriginChain string `json:"origin_chain"` // Chain where token is native
	IsNative    bool   `json:"is_native"`    // True if native to query chain
	IbcPath     string `json:"ibc_path"`     // IBC path if applicable (e.g., "transfer/channel-0")
}