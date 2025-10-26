package models

// RouteRequest - API POST body
type RouteRequest struct {
	ChainA         string `json:"chainA"`         // e.g., "juno"
	TokenInDenom   string `json:"tokenInDenom"`   // e.g., "ujuno"
	AmountIn       string `json:"amountIn"`       // e.g., "1000000"
	ChainB         string `json:"chainB"`         // e.g., "cosmoshub"
	TokenOutDenom  string `json:"tokenOutDenom"`  // e.g., "uatom"
	SenderAddress  string `json:"senderAddress"`  // For validation
	ReceiverAddress string `json:"receiverAddress"`
}


// TokenMapping represents how a token transforms between chains
type TokenMapping struct {
	ChainDenom string `json:"chainDenom"` // Denom on this specific chain (native or IBC)
	BaseDenom  string `json:"baseDenom"`  // Original native denom on origin chain
	OriginChain string `json:"originChain"` // Where this token originally comes from
	IsNative   bool   `json:"isNative"`   // True if this token is native to the current chain
}

// IBCLeg represents one IBC transfer leg in a route
type IBCLeg struct {
	FromChain   string        `json:"fromChain"`   // Chain ID
	ToChain     string        `json:"toChain"`     // Chain ID
	Channel     string        `json:"channel"`     // Source channel
	Port        string        `json:"port"`        // Source port (usually "transfer")
	Token       *TokenMapping `json:"token"`       // Token info on source chain
	Amount      string        `json:"amount"`      // Amount to transfer
}

// DirectRoute represents a simple IBC transfer
type DirectRoute struct {
	Transfer *IBCLeg `json:"transfer"` // Single IBC transfer
}

// SwapQuote represents the swap information from a broker
type SwapQuote struct {
	Broker       string      `json:"broker"`       // Broker type (e.g., "osmosis")
	TokenIn      *TokenMapping `json:"tokenIn"`      // Input token on broker chain
	TokenOut     *TokenMapping `json:"tokenOut"`     // Output token on broker chain
	AmountIn     string      `json:"amountIn"`     // Actual input amount
	AmountOut    string      `json:"amountOut"`    // Estimated output amount
	PriceImpact  string      `json:"priceImpact"`  // Price impact (e.g., "0.02" for 2%)
	EffectiveFee string      `json:"effectiveFee"` // Total fees
	RouteData    interface{} `json:"routeData"`    // Broker-specific route data (pools, hops, etc.)
}

// MultiHopRoute represents a route that requires a swap on a broker chain
type MultiHopRoute struct {
	Path        []string    `json:"path"`        // Chain IDs in order: [sourceChain, brokerChain, destChain]
	InboundLeg  *IBCLeg     `json:"inboundLeg"`  // Source -> Broker
	Swap        *SwapQuote  `json:"swap"`        // Swap on broker
	OutboundLeg *IBCLeg     `json:"outboundLeg"` // Broker -> Destination
}

// RouteResponse - unified response for all route types (informative, not prescriptive)
type RouteResponse struct {
	Success      bool           `json:"success"`
	RouteType    string         `json:"routeType"` // "direct" | "multi_hop" | "impossible"
	ErrorMessage string         `json:"errorMessage,omitempty"`
	Direct       *DirectRoute   `json:"direct,omitempty"`
	MultiHop     *MultiHopRoute `json:"multiHop,omitempty"`
}

// DenomLookupRequest - request to lookup denom information
type DenomLookupRequest struct {
	Denom   string `json:"denom"`   // Can be native (uatom) or IBC (ibc/ABC123...)
	ChainID string `json:"chainId"` // Which chain to resolve the denom for
}

// DenomInfo - detailed information about a token denom
type DenomInfo struct {
	ChainDenom  string `json:"chainDenom"`  // Denom on the specified chain
	BaseDenom   string `json:"baseDenom"`   // Original native denom
	OriginChain string `json:"originChain"` // Chain where token is native
	IsNative    bool   `json:"isNative"`    // True if native to query chain
	IbcPath     string `json:"ibcPath"`     // IBC path if applicable (e.g., "transfer/channel-0")
}