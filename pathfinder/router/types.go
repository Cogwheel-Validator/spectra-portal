package router

// ChainMapId maps chain names to their integer IDs
type ChainMapId map[string]int

func NewChainMapId(chains []string) ChainMapId {
	chainMapId := make(ChainMapId)
	for i, chain := range chains {
		chainMapId[chain] = i
	}
	return chainMapId
}

func (cmi *ChainMapId) GetId(chain string) int {
	return (*cmi)[chain]
}

func (cmi *ChainMapId) GetChain(id int) string {
	for chain, mapId := range *cmi {
		if mapId == id {
			return chain
		}
	}
	return ""
}

// PathfinderChain represents one unit in the Pathfinder Graph.
type PathfinderChain struct {
	Name string
	Id   string
	// if the chain is a broker, example: Osmosis, this will be true and the BrokerId will be the id of the broker
	HasPFM   bool // has packet forwarding middleware
	Broker   bool
	BrokerId string
	// IBCHooksContract is the wasm contract address for swap operations (e.g., Osmosis entry point)
	IBCHooksContract string
	// Bech32Prefix for address conversion (e.g., "osmo", "cosmos")
	Bech32Prefix string
	NativeTokens []TokenInfo
	Routes       []BasicRoute
}

// BasicRoute represents a route between two chains.
type BasicRoute struct {
	ToChain       string
	ToChainId     string
	ConnectionId  string
	ChannelId     string
	PortId        string
	AllowedTokens map[string]TokenInfo
}

// TokenInfo contains comprehensive token information including origin tracking
type TokenInfo struct {
	// Denom on the current chain in the route context (native or IBC)
	ChainDenom string
	// Denom on the destination chain (after IBC transfer)
	IbcDenom string
	// Original native denom on the token's origin chain
	BaseDenom string
	// Chain ID where this token is native
	OriginChain string
	// Human-readable symbol (e.g., "ATOM", "OSMO")
	Symbol string
	// Number of decimal places
	Decimals int
}

// RouteIndex with denom mapping should be internal logic for the router
type RouteIndex struct {
	directRoutes        map[string]*BasicRoute            // "chainA->chainB->denom"
	brokerRoutes        map[string]map[string]*BasicRoute // brokerChainId -> toChainId -> BasicRoute
	chainToBrokerRoutes map[string]map[string]*BasicRoute // chainId -> brokerChainId -> BasicRoute
	denomToTokenInfo    map[string]map[string]*TokenInfo  // chainId -> denom -> TokenInfo
	brokers             map[string]bool                   // brokerId -> is broker
	brokerChains        map[string]string                 // chainId -> brokerId (for chains that are brokers)
	pfmChains           map[string]bool                   // chainId -> supports PFM
	chainRoutes         map[string]map[string]*BasicRoute // chainId -> toChainId -> BasicRoute (all routes from a chain)
}

// MultiHopInfo represents a route that goes through a broker with token swaps
type MultiHopInfo struct {
	BrokerChain   string // Broker ID (e.g., "osmosis-sqs")
	BrokerChainId string // Broker chain ID (e.g., "osmosis-1")
	// InboundRoutes contains one or more routes for inbound transfers to reach the broker
	// Single route for direct source->broker, multiple for multi-hop (e.g., source->intermediate->broker)
	// Nil if source is the broker chain
	InboundRoutes []*BasicRoute
	// InboundPath contains the chain IDs for the inbound path (including source, excluding broker)
	// e.g., ["neutron-1", "cosmoshub-4"] for neutron -> cosmos hub -> osmosis
	InboundPath []string
	// OutboundRoutes contains one or more routes for outbound transfers
	// Single route for direct 3-chain, multiple for 4+ chain routes
	// Can also be nil if destination is broker
	OutboundRoutes []*BasicRoute
	TokenIn        *TokenInfo // Token info from source chain perspective
	TokenOut       *TokenInfo // Token info from destination chain perspective
	// TokenOutOnBroker is the token on the broker chain that will be swapped to
	// For full routes: this is the token on broker that becomes TokenOut after IBC transfer
	// For swap-only: same as TokenOut (since destination is broker)
	TokenOutOnBroker *TokenInfo
	// InboundIntermediateTokens holds token info for each inbound hop (excluding source)
	// For 2-inbound: [tokenOnIntermediate] (token as it arrives on intermediate chain)
	InboundIntermediateTokens []*TokenInfo
	// IntermediateTokens holds token info for each outbound hop after the swap
	// For 4-chain outbound: [tokenOnIntermediate, tokenOnDest]
	IntermediateTokens []*TokenInfo
	// SwapOnly is true when the destination is the broker chain (no outbound IBC transfer)
	SwapOnly bool
	// SourceIsBroker is true when the source chain is the broker (no inbound IBC transfer)
	SourceIsBroker bool
}

// IndirectRouteInfo represents a multi-hop path without swaps
type IndirectRouteInfo struct {
	Path   []string      // Chain IDs in order
	Routes []*BasicRoute // Routes between consecutive chains
	Token  *TokenInfo    // Token that travels through all chains
}

// MultiHopInboundResult contains the result of finding a multi-hop inbound route
type MultiHopInboundResult struct {
	Routes             []*BasicRoute // Routes in order (source->intermediate, intermediate->broker)
	Path               []string      // Chain IDs in order (source, intermediate) - not including broker
	TokenIn            *TokenInfo    // Token info on source chain
	IntermediateTokens []*TokenInfo  // Token info on intermediate chains
}
