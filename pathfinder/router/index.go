package router

import (
	"container/list"
	"fmt"

	models "github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/models"
)

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

/*
Pathfinder Chain is a type that represents one unit in the Pathfinder Graph.
*/
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

// BasicRoute is a type that represents a route between two chains.
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

func (ri *RouteIndex) BuildIndex(chains []PathfinderChain) error {
	if len(chains) == 0 {
		return fmt.Errorf("no chains to build index for")
	}

	// First pass: Register all brokers and PFM chains
	for _, chain := range chains {
		if chain.HasPFM {
			ri.pfmChains[chain.Id] = true
		}

		if chain.Broker {
			ri.brokers[chain.BrokerId] = true
			ri.brokerChains[chain.Id] = chain.BrokerId
		}
	}

	// Second pass: Build all route indices
	for _, chain := range chains {
		if ri.denomToTokenInfo[chain.Id] == nil {
			ri.denomToTokenInfo[chain.Id] = make(map[string]*TokenInfo)
		}

		// Track PFM support
		if chain.HasPFM {
			ri.pfmChains[chain.Id] = true
		}

		// Initialize chain routes map
		if ri.chainRoutes[chain.Id] == nil {
			ri.chainRoutes[chain.Id] = make(map[string]*BasicRoute)
		}

		// Add native tokens to denomToTokenInfo (even if they're not in any routes)
		// This ensures tokens with allowed_destinations=["none"] are still available on their native chain
		for _, nativeToken := range chain.NativeTokens {
			// Only add if not already present (routes take precedence as they have more complete info)
			if _, exists := ri.denomToTokenInfo[chain.Id][nativeToken.ChainDenom]; !exists {
				tokenInfoCopy := nativeToken
				ri.denomToTokenInfo[chain.Id][nativeToken.ChainDenom] = &tokenInfoCopy
			}
		}

		for _, route := range chain.Routes {
			// Index token info
			for denom, tokenInfo := range route.AllowedTokens {
				ri.denomToTokenInfo[chain.Id][denom] = &tokenInfo
			}

			// Index direct routes
			for denom := range route.AllowedTokens {
				key := routeKey(chain.Id, route.ToChainId, denom)
				ri.directRoutes[key] = &route
			}

			// Index all chain-to-chain routes
			if _, exists := ri.chainRoutes[chain.Id][route.ToChainId]; !exists {
				ri.chainRoutes[chain.Id][route.ToChainId] = &route
			}

			// Index broker routes - check if destination chain is a broker
			if brokerId, isBroker := ri.brokerChains[route.ToChainId]; isBroker {
				if ri.chainToBrokerRoutes[chain.Id] == nil {
					ri.chainToBrokerRoutes[chain.Id] = make(map[string]*BasicRoute)
				}
				ri.chainToBrokerRoutes[chain.Id][brokerId] = &route
			}
		}

		// Build broker-specific route maps
		if chain.Broker {
			if ri.brokerRoutes[chain.BrokerId] == nil {
				ri.brokerRoutes[chain.BrokerId] = make(map[string]*BasicRoute)
			}
			for _, route := range chain.Routes {
				if _, exists := ri.brokerRoutes[chain.BrokerId][route.ToChainId]; !exists {
					ri.brokerRoutes[chain.BrokerId][route.ToChainId] = &route
				}
			}
		}
	}

	return nil
}

func (ri *RouteIndex) FindDirectRoute(req models.RouteRequest) *BasicRoute {
	// Check if same token can go directly
	key := routeKey(req.ChainFrom, req.ChainTo, req.TokenFromDenom)
	if route, exists := ri.directRoutes[key]; exists {
		// Verify output denom matches (same token on both chains)
		tokenInfo := ri.denomToTokenInfo[req.ChainFrom][req.TokenFromDenom]
		if tokenInfo != nil && tokenInfo.IbcDenom == req.TokenToDenom {
			return route
		}
	}
	return nil
}

// FindIndirectRoute finds multi-hop paths without swaps using BFS
// It looks for paths where the same token (by origin) can travel through intermediate chains
func (ri *RouteIndex) FindIndirectRoute(req models.RouteRequest) *IndirectRouteInfo {
	// Get source and destination token info
	sourceToken := ri.denomToTokenInfo[req.ChainFrom][req.TokenFromDenom]
	destToken := ri.denomToTokenInfo[req.ChainTo][req.TokenToDenom]

	if sourceToken == nil || destToken == nil {
		return nil
	}

	// Must be the same underlying token (same origin chain and base denom)
	if sourceToken.OriginChain != destToken.OriginChain || sourceToken.BaseDenom != destToken.BaseDenom {
		return nil
	}

	// BFS to find shortest path
	type pathNode struct {
		chainId string
		route   *BasicRoute // route used to reach this chain
		prev    *pathNode
	}

	queue := list.New()
	queue.PushBack(&pathNode{chainId: req.ChainFrom, route: nil, prev: nil})
	visited := map[string]bool{req.ChainFrom: true}

	for queue.Len() > 0 {
		element := queue.Front()
		current := element.Value.(*pathNode)
		queue.Remove(element)

		// Check if we reached destination
		if current.chainId == req.ChainTo {
			// Reconstruct path
			path := []string{}
			routes := []*BasicRoute{}
			node := current

			for node != nil {
				path = append([]string{node.chainId}, path...)
				if node.route != nil {
					routes = append([]*BasicRoute{node.route}, routes...)
				}
				node = node.prev
			}

			return &IndirectRouteInfo{
				Path:   path,
				Routes: routes,
				Token:  sourceToken,
			}
		}

		// Explore neighbors
		for nextChainId, route := range ri.chainRoutes[current.chainId] {
			if visited[nextChainId] {
				continue
			}

			// Check if our token can travel on this route
			// The token needs to be in AllowedTokens on the current chain
			currentToken := ri.denomToTokenInfo[current.chainId][req.TokenFromDenom]
			if current.chainId != req.ChainFrom {
				// For intermediate chains, find the token by origin
				currentToken = ri.findTokenByOrigin(current.chainId, sourceToken.OriginChain, sourceToken.BaseDenom)
			}

			if currentToken == nil {
				continue
			}

			// Check if this token is allowed on the route
			if _, allowed := route.AllowedTokens[currentToken.ChainDenom]; !allowed {
				continue
			}

			visited[nextChainId] = true
			queue.PushBack(&pathNode{
				chainId: nextChainId,
				route:   route,
				prev:    current,
			})
		}
	}

	return nil
}

// findTokenByOrigin finds a token on a chain by its origin chain and base denom
func (ri *RouteIndex) findTokenByOrigin(chainId, originChain, baseDenom string) *TokenInfo {
	for _, tokenInfo := range ri.denomToTokenInfo[chainId] {
		if tokenInfo.OriginChain == originChain && tokenInfo.BaseDenom == baseDenom {
			return tokenInfo
		}
	}
	return nil
}

type MultiHopInfo struct {
	BrokerChain   string // Broker ID (e.g., "osmosis-sqs")
	BrokerChainId string // Broker chain ID (e.g., "osmosis-1")
	InboundRoute  *BasicRoute
	// OutboundRoutes contains one or more routes for outbound transfers
	// Single route for direct 3-chain, multiple for 4+ chain routes
	// Can also be a nil if destination is broker
	OutboundRoutes []*BasicRoute
	TokenIn        *TokenInfo // Token info from source chain perspective
	TokenOut       *TokenInfo // Token info from destination chain perspective
	// TokenOutOnBroker is the token on the broker chain that will be swapped to
	// For full routes: this is the token on broker that becomes TokenOut after IBC transfer
	// For swap-only: same as TokenOut (since destination is broker)
	TokenOutOnBroker *TokenInfo
	// IntermediateTokens holds token info for each hop after the swap
	// For 4-chain: [tokenOnIntermediate, tokenOnDest]
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

// The purpose of the FindMultiHopRoute is to confirm that the tokenIn and tokenOut are possible
// to be reached to the broker and that the broker can reach the destination chain.
//
// In the future there might be more than one broker that can reach the destination chain.
// So we need to return a list of possible multi-hop routes.
//
// Handles these cases:
// - Case 1: Same-chain swap (source == broker == destination) - just swap, no IBC
// - Case 2: Swap-only (source != broker, destination == broker) - inbound IBC + swap
// - Case 3: Source-is-broker (source == broker, destination != broker) - swap + outbound IBC
// - Case 4: Full broker route (source != broker != destination) - inbound IBC + swap + outbound IBC
func (ri *RouteIndex) FindMultiHopRoute(req models.RouteRequest) []*MultiHopInfo {
	multiHopInfos := []*MultiHopInfo{}

	// Check if we can route through a broker with token swap
	for brokerId := range ri.brokers {
		// Find the chain ID for this broker
		var brokerChainId string
		for chainId, bid := range ri.brokerChains {
			if bid == brokerId {
				brokerChainId = chainId
				break
			}
		}
		if brokerChainId == "" {
			continue // Should not happen, but defensive check
		}

		pathfinderLog.Debug().
			Str("brokerId", brokerId).
			Str("brokerChainId", brokerChainId).
			Str("chainFrom", req.ChainFrom).
			Str("chainTo", req.ChainTo).
			Msg("Checking broker route")

		sourceIsBroker := req.ChainFrom == brokerChainId
		destIsBroker := req.ChainTo == brokerChainId

		// Case 1: Same-chain swap (source == broker == destination)
		if sourceIsBroker && destIsBroker {
			multiHopInfo := ri.findSameChainSwapRoute(req, brokerId, brokerChainId)
			if multiHopInfo != nil {
				pathfinderLog.Debug().Msg("Found same-chain swap route")
				multiHopInfos = append(multiHopInfos, multiHopInfo)
			}
			continue
		}

		// Case 2: Swap-only (destination is broker, source is not)
		if destIsBroker && !sourceIsBroker {
			multiHopInfo := ri.findSwapOnlyRoute(req, brokerId, brokerChainId)
			if multiHopInfo != nil {
				pathfinderLog.Debug().Msg("Found swap-only route (destination is broker)")
				multiHopInfos = append(multiHopInfos, multiHopInfo)
			}
			continue
		}

		// Case 3: Source-is-broker (source is broker, destination is not)
		if sourceIsBroker && !destIsBroker {
			multiHopInfo := ri.findBrokerAsSourceRoute(req, brokerId, brokerChainId)
			if multiHopInfo != nil {
				pathfinderLog.Debug().Msg("Found broker-as-source route")
				multiHopInfos = append(multiHopInfos, multiHopInfo)
			}
			continue
		}

		// Case 4: Full broker route (source → broker → destination)
		multiHopInfo := ri.findFullBrokerRoute(req, brokerId, brokerChainId)
		if multiHopInfo != nil {
			pathfinderLog.Debug().Msg("Found full broker route")
			multiHopInfos = append(multiHopInfos, multiHopInfo)
		}
	}

	pathfinderLog.Debug().Int("count", len(multiHopInfos)).Msg("Found multi-hop routes")
	return multiHopInfos
}

// findSwapOnlyRoute finds a route where the destination is the broker (IBC transfer + swap, no outbound)
func (ri *RouteIndex) findSwapOnlyRoute(req models.RouteRequest, brokerId, brokerChainId string) *MultiHopInfo {
	// Can we reach broker from source?
	inboundRoute := ri.chainToBrokerRoutes[req.ChainFrom][brokerId]
	if inboundRoute == nil {
		pathfinderLog.Debug().Str("chainFrom", req.ChainFrom).Str("brokerId", brokerId).Msg("No inbound route to broker")
		return nil
	}

	// Is input token allowed on inbound route?
	tokenIn, tokenAllowed := inboundRoute.AllowedTokens[req.TokenFromDenom]
	if !tokenAllowed {
		pathfinderLog.Debug().Str("tokenFromDenom", req.TokenFromDenom).Msg("Input token not allowed on inbound route")
		return nil
	}

	// Is output token available on the broker chain?
	tokenOut := ri.denomToTokenInfo[brokerChainId][req.TokenToDenom]
	if tokenOut == nil {
		pathfinderLog.Debug().
			Str("tokenToDenom", req.TokenToDenom).
			Str("brokerChainId", brokerChainId).
			Msg("Output token not found on broker chain")
		return nil
	}

	pathfinderLog.Debug().
		Str("tokenIn", tokenIn.ChainDenom).
		Str("tokenOut", tokenOut.ChainDenom).
		Msg("Swap-only route validated")

	return &MultiHopInfo{
		BrokerChain:      brokerId,
		BrokerChainId:    brokerChainId,
		InboundRoute:     inboundRoute,
		OutboundRoutes:   nil, // No outbound route needed
		TokenIn:          &tokenIn,
		TokenOut:         tokenOut,
		TokenOutOnBroker: tokenOut, // Same as TokenOut since destination is broker
		SwapOnly:         true,
	}
}

// findSameChainSwapRoute finds a route where source and destination are both the broker (just swap)
func (ri *RouteIndex) findSameChainSwapRoute(req models.RouteRequest, brokerId, brokerChainId string) *MultiHopInfo {
	// Is input token available on the broker chain?
	tokenIn := ri.denomToTokenInfo[brokerChainId][req.TokenFromDenom]
	if tokenIn == nil {
		pathfinderLog.Debug().
			Str("tokenFromDenom", req.TokenFromDenom).
			Str("brokerChainId", brokerChainId).
			Msg("Input token not found on broker chain for same-chain swap")
		return nil
	}

	// Is output token available on the broker chain?
	tokenOut := ri.denomToTokenInfo[brokerChainId][req.TokenToDenom]
	if tokenOut == nil {
		pathfinderLog.Debug().
			Str("tokenToDenom", req.TokenToDenom).
			Str("brokerChainId", brokerChainId).
			Msg("Output token not found on broker chain for same-chain swap")
		return nil
	}

	pathfinderLog.Debug().
		Str("tokenIn", tokenIn.ChainDenom).
		Str("tokenOut", tokenOut.ChainDenom).
		Msg("Same-chain swap route validated")

	return &MultiHopInfo{
		BrokerChain:      brokerId,
		BrokerChainId:    brokerChainId,
		InboundRoute:     nil, // No inbound route needed (source is broker)
		OutboundRoutes:   nil, // No outbound route needed (dest is broker)
		TokenIn:          tokenIn,
		TokenOut:         tokenOut,
		TokenOutOnBroker: tokenOut, // Same as TokenOut since staying on broker
		SwapOnly:         true,     // Technically just a swap
		SourceIsBroker:   true,     // New flag to distinguish from regular swap-only
	}
}

// findBrokerAsSourceRoute finds a route where source is the broker (swap + outbound IBC)
func (ri *RouteIndex) findBrokerAsSourceRoute(req models.RouteRequest, brokerId, brokerChainId string) *MultiHopInfo {
	// Is input token available on the broker chain?
	tokenIn := ri.denomToTokenInfo[brokerChainId][req.TokenFromDenom]
	if tokenIn == nil {
		pathfinderLog.Debug().
			Str("tokenFromDenom", req.TokenFromDenom).
			Str("brokerChainId", brokerChainId).
			Msg("Input token not found on broker chain")
		return nil
	}

	// Can broker reach destination?
	outboundRoute := ri.brokerRoutes[brokerId][req.ChainTo]
	if outboundRoute == nil {
		pathfinderLog.Debug().Str("brokerId", brokerId).Str("chainTo", req.ChainTo).Msg("No outbound route from broker")
		return nil
	}

	// Is output token available on destination?
	tokenOut := ri.denomToTokenInfo[req.ChainTo][req.TokenToDenom]
	if tokenOut == nil {
		pathfinderLog.Debug().Str("tokenToDenom", req.TokenToDenom).Str("chainTo", req.ChainTo).Msg("Output token not found on destination")
		return nil
	}

	// Check if there's a token in the outbound route that will become the desired token on destination
	var matchingToken *TokenInfo
	for _, tokenInfo := range outboundRoute.AllowedTokens {
		if tokenInfo.IbcDenom == req.TokenToDenom {
			matchingToken = &tokenInfo
			break
		}
	}

	if matchingToken == nil {
		pathfinderLog.Debug().Str("tokenToDenom", req.TokenToDenom).Msg("No matching token in outbound route AllowedTokens")
		return nil
	}

	pathfinderLog.Debug().
		Str("tokenIn", tokenIn.ChainDenom).
		Str("tokenOutOnBroker", matchingToken.ChainDenom).
		Str("tokenOutOnDest", tokenOut.ChainDenom).
		Msg("Broker-as-source route validated")

	return &MultiHopInfo{
		BrokerChain:      brokerId,
		BrokerChainId:    brokerChainId,
		InboundRoute:     nil, // No inbound route needed (source is broker)
		OutboundRoutes:   []*BasicRoute{outboundRoute},
		TokenIn:          tokenIn,
		TokenOut:         tokenOut,
		TokenOutOnBroker: matchingToken,
		SwapOnly:         false,
		SourceIsBroker:   true,
	}
}

// findFullBrokerRoute finds a route with source → broker → destination
// Also handles 4-chain routes where the swap output needs to go through an intermediate chain
func (ri *RouteIndex) findFullBrokerRoute(req models.RouteRequest, brokerId, brokerChainId string) *MultiHopInfo {
	// Can we reach broker from source?
	inboundRoute := ri.chainToBrokerRoutes[req.ChainFrom][brokerId]
	if inboundRoute == nil {
		pathfinderLog.Debug().Str("chainFrom", req.ChainFrom).Str("brokerId", brokerId).Msg("No inbound route to broker")
		return nil
	}

	// Is input token allowed?
	tokenIn, tokenAllowed := inboundRoute.AllowedTokens[req.TokenFromDenom]
	if !tokenAllowed {
		pathfinderLog.Debug().Str("tokenFromDenom", req.TokenFromDenom).Msg("Input token not allowed on inbound route")
		return nil
	}

	// Is output token available on destination?
	tokenOut := ri.denomToTokenInfo[req.ChainTo][req.TokenToDenom]
	if tokenOut == nil {
		pathfinderLog.Debug().Str("tokenToDenom", req.TokenToDenom).Str("chainTo", req.ChainTo).Msg("Output token not found on destination")
		return nil
	}

	// Find the token on broker that will become tokenOut
	// First, check if there's a direct route from broker to destination
	directOutbound := ri.brokerRoutes[brokerId][req.ChainTo]
	if directOutbound != nil {
		// Check if the desired token can be sent directly
		for _, tokenInfo := range directOutbound.AllowedTokens {
			if tokenInfo.IbcDenom == req.TokenToDenom {
				pathfinderLog.Debug().
					Str("tokenIn", tokenIn.ChainDenom).
					Str("tokenOutOnBroker", tokenInfo.ChainDenom).
					Str("tokenOutOnDest", tokenOut.ChainDenom).
					Msg("Full broker route validated (direct)")

				return &MultiHopInfo{
					BrokerChain:      brokerId,
					BrokerChainId:    brokerChainId,
					InboundRoute:     inboundRoute,
					OutboundRoutes:   []*BasicRoute{directOutbound},
					TokenIn:          &tokenIn,
					TokenOut:         tokenOut,
					TokenOutOnBroker: &tokenInfo,
					SwapOnly:         false,
				}
			}
		}
	}

	// No direct route works - check if we need 4-chain route
	// This happens when the token's origin is different from both broker and destination
	// e.g., USDC from Noble: broker=osmosis, origin=noble, dest=juno
	// Route: source -> broker -> noble (unwind) -> juno (forward)
	originChain := tokenOut.OriginChain
	if originChain != brokerChainId && originChain != req.ChainTo {
		pathfinderLog.Debug().
			Str("originChain", originChain).
			Str("brokerChain", brokerChainId).
			Str("destChain", req.ChainTo).
			Msg("Checking 4-chain route through origin")

		// Check if broker can reach the origin chain
		brokerToOrigin := ri.brokerRoutes[brokerId][originChain]
		if brokerToOrigin == nil {
			pathfinderLog.Debug().Msg("No route from broker to token origin chain")
			return nil
		}

		// Check if the token can be sent from broker to origin (should unwind to native)
		var tokenOnBroker *TokenInfo
		for _, tokenInfo := range brokerToOrigin.AllowedTokens {
			// The token should unwind to native on origin chain
			if tokenInfo.BaseDenom == tokenOut.BaseDenom && tokenInfo.OriginChain == originChain {
				tokenOnBroker = &tokenInfo
				break
			}
		}
		if tokenOnBroker == nil {
			pathfinderLog.Debug().Msg("Token cannot be sent from broker to origin")
			return nil
		}

		// Check if origin chain can reach destination
		originToDest := ri.findRouteFromChain(originChain, req.ChainTo)
		if originToDest == nil {
			pathfinderLog.Debug().Msg("No route from origin to destination")
			return nil
		}

		// Check if token can be forwarded from origin to destination
		var tokenOnOrigin *TokenInfo
		for _, tokenInfo := range originToDest.AllowedTokens {
			if tokenInfo.IbcDenom == req.TokenToDenom {
				tokenOnOrigin = &tokenInfo
				break
			}
		}
		if tokenOnOrigin == nil {
			pathfinderLog.Debug().Msg("Token cannot be forwarded from origin to destination")
			return nil
		}

		pathfinderLog.Debug().
			Str("tokenIn", tokenIn.ChainDenom).
			Str("tokenOutOnBroker", tokenOnBroker.ChainDenom).
			Str("tokenOnOrigin", tokenOnOrigin.ChainDenom).
			Str("tokenOutOnDest", tokenOut.ChainDenom).
			Msg("4-chain broker route validated")

		return &MultiHopInfo{
			BrokerChain:        brokerId,
			BrokerChainId:      brokerChainId,
			InboundRoute:       inboundRoute,
			OutboundRoutes:     []*BasicRoute{brokerToOrigin, originToDest},
			TokenIn:            &tokenIn,
			TokenOut:           tokenOut,
			TokenOutOnBroker:   tokenOnBroker,
			IntermediateTokens: []*TokenInfo{tokenOnOrigin},
			SwapOnly:           false,
		}
	}

	pathfinderLog.Debug().Str("tokenToDenom", req.TokenToDenom).Msg("No valid broker route found")
	return nil
}

// findRouteFromChain finds a route from a chain to another chain
func (ri *RouteIndex) findRouteFromChain(fromChain, toChain string) *BasicRoute {
	routes := ri.chainRoutes[fromChain]
	for _, route := range routes {
		if route.ToChainId == toChain {
			return route
		}
	}
	return nil
}

// routeKey is a helper function to create a unique key for a route
func routeKey(fromChain, toChain, denom string) string {
	return fmt.Sprintf("%s->%s:%s", fromChain, toChain, denom)
}

// NewRouteIndex creates a new RouteIndex with initialized maps
func NewRouteIndex() *RouteIndex {
	return &RouteIndex{
		directRoutes:        make(map[string]*BasicRoute),
		brokerRoutes:        make(map[string]map[string]*BasicRoute),
		chainToBrokerRoutes: make(map[string]map[string]*BasicRoute),
		denomToTokenInfo:    make(map[string]map[string]*TokenInfo),
		brokers:             make(map[string]bool),
		brokerChains:        make(map[string]string),
		pfmChains:           make(map[string]bool),
		chainRoutes:         make(map[string]map[string]*BasicRoute),
	}
}
