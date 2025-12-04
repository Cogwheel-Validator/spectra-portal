package router

import (
	"container/list"
	"fmt"
	"os"
	"time"

	models "github.com/Cogwheel-Validator/spectra-ibc-hub/solver/models"
	"github.com/rs/zerolog"
)

var solverLog zerolog.Logger

func init() {
	out := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}
	solverLog = zerolog.New(out).With().Timestamp().Str("component", "solver").Logger()
}

// BrokerClient is an interface for querying different DEX protocols on broker chains
// Each broker (Osmosis, Neutron, etc.) implements this interface with their specific API
type BrokerClient interface {
	// QuerySwap queries the broker DEX for a swap route and returns standardized swap information
	// tokenInDenom: the denom of the input token on the broker chain (may be IBC denom)
	// tokenInAmount: the amount of input tokens
	// tokenOutDenom: the denom of the desired output token on the broker chain (may be IBC denom)
	QuerySwap(tokenInDenom, tokenInAmount, tokenOutDenom string) (*SwapResult, error)

	// GetBrokerType returns the type of broker (e.g., "osmosis-sqs", "astroport", etc.)
	GetBrokerType() string
}

// SwapResult contains standardized swap information from any broker DEX
type SwapResult struct {
	AmountIn     string      // Actual amount in (after any adjustments)
	AmountOut    string      // Expected amount out
	PriceImpact  string      // Price impact as string (e.g., "0.02" for 2%)
	EffectiveFee string      // Effective fee as string
	RouteData    interface{} // Broker-specific route data (pools, hops, etc.)
}

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
Solver Chain is a type that represents one unit in the Solver Graph.
*/
type SolverChain struct {
	Name string
	Id   string
	// if the chain is a broker, example: Osmosis, this will be true and the BrokerId will be the id of the broker
	HasPFM   bool // has package forwaring middleware
	Broker   bool
	BrokerId string
	Routes   []BasicRoute
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

func (ri *RouteIndex) BuildIndex(chains []SolverChain) error {
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
	OutboundRoute *BasicRoute // nil if destination is the broker itself
	TokenIn       *TokenInfo
	TokenOut      *TokenInfo
	// SwapOnly is true when the destination is the broker chain (no outbound IBC transfer)
	SwapOnly bool
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
// This also handles "swap-only" routes where the destination IS the broker chain.
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

		solverLog.Debug().
			Str("brokerId", brokerId).
			Str("brokerChainId", brokerChainId).
			Str("chainFrom", req.ChainFrom).
			Str("chainTo", req.ChainTo).
			Msg("Checking broker route")

		// Case 1: Destination IS the broker chain (swap-only, no outbound IBC)
		if req.ChainTo == brokerChainId {
			multiHopInfo := ri.findSwapOnlyRoute(req, brokerId, brokerChainId)
			if multiHopInfo != nil {
				solverLog.Debug().Msg("Found swap-only route (destination is broker)")
				multiHopInfos = append(multiHopInfos, multiHopInfo)
			}
			continue
		}

		// Case 2: Full broker route (source → broker → destination)
		multiHopInfo := ri.findFullBrokerRoute(req, brokerId, brokerChainId)
		if multiHopInfo != nil {
			solverLog.Debug().Msg("Found full broker route")
			multiHopInfos = append(multiHopInfos, multiHopInfo)
		}
	}

	solverLog.Debug().Int("count", len(multiHopInfos)).Msg("Found multi-hop routes")
	return multiHopInfos
}

// findSwapOnlyRoute finds a route where the destination is the broker (IBC transfer + swap, no outbound)
func (ri *RouteIndex) findSwapOnlyRoute(req models.RouteRequest, brokerId, brokerChainId string) *MultiHopInfo {
	// Can we reach broker from source?
	inboundRoute := ri.chainToBrokerRoutes[req.ChainFrom][brokerId]
	if inboundRoute == nil {
		solverLog.Debug().Str("chainFrom", req.ChainFrom).Str("brokerId", brokerId).Msg("No inbound route to broker")
		return nil
	}

	// Is input token allowed on inbound route?
	tokenIn, tokenAllowed := inboundRoute.AllowedTokens[req.TokenFromDenom]
	if !tokenAllowed {
		solverLog.Debug().Str("tokenFromDenom", req.TokenFromDenom).Msg("Input token not allowed on inbound route")
		return nil
	}

	// Is output token available on the broker chain?
	tokenOut := ri.denomToTokenInfo[brokerChainId][req.TokenToDenom]
	if tokenOut == nil {
		solverLog.Debug().
			Str("tokenToDenom", req.TokenToDenom).
			Str("brokerChainId", brokerChainId).
			Msg("Output token not found on broker chain")
		return nil
	}

	solverLog.Debug().
		Str("tokenIn", tokenIn.ChainDenom).
		Str("tokenOut", tokenOut.ChainDenom).
		Msg("Swap-only route validated")

	return &MultiHopInfo{
		BrokerChain:   brokerId,
		BrokerChainId: brokerChainId,
		InboundRoute:  inboundRoute,
		OutboundRoute: nil, // No outbound route needed
		TokenIn:       &tokenIn,
		TokenOut:      tokenOut,
		SwapOnly:      true,
	}
}

// findFullBrokerRoute finds a route with source → broker → destination
func (ri *RouteIndex) findFullBrokerRoute(req models.RouteRequest, brokerId, brokerChainId string) *MultiHopInfo {
	// Can we reach broker from source?
	inboundRoute := ri.chainToBrokerRoutes[req.ChainFrom][brokerId]
	if inboundRoute == nil {
		solverLog.Debug().Str("chainFrom", req.ChainFrom).Str("brokerId", brokerId).Msg("No inbound route to broker")
		return nil
	}

	// Is input token allowed?
	tokenIn, tokenAllowed := inboundRoute.AllowedTokens[req.TokenFromDenom]
	if !tokenAllowed {
		solverLog.Debug().Str("tokenFromDenom", req.TokenFromDenom).Msg("Input token not allowed on inbound route")
		return nil
	}

	// Can broker reach destination?
	outboundRoute := ri.brokerRoutes[brokerId][req.ChainTo]
	if outboundRoute == nil {
		solverLog.Debug().Str("brokerId", brokerId).Str("chainTo", req.ChainTo).Msg("No outbound route from broker")
		return nil
	}

	// Is output token available on destination?
	tokenOut := ri.denomToTokenInfo[req.ChainTo][req.TokenToDenom]
	if tokenOut == nil {
		solverLog.Debug().Str("tokenToDenom", req.TokenToDenom).Str("chainTo", req.ChainTo).Msg("Output token not found on destination")
		return nil
	}

	// Check if there's a token in the outbound route that will become the desired token on destination
	// We need to find a token in AllowedTokens whose IbcDenom matches req.TokenToDenom
	var matchingToken *TokenInfo
	for _, tokenInfo := range outboundRoute.AllowedTokens {
		// Check if this token becomes the desired denom on the destination
		// The IbcDenom field indicates what the token becomes after IBC transfer
		if tokenInfo.IbcDenom == req.TokenToDenom {
			matchingToken = &tokenInfo
			break
		}
	}

	if matchingToken == nil {
		solverLog.Debug().Str("tokenToDenom", req.TokenToDenom).Msg("No matching token in outbound route AllowedTokens")
		return nil
	}

	solverLog.Debug().
		Str("tokenIn", tokenIn.ChainDenom).
		Str("tokenOut", tokenOut.ChainDenom).
		Msg("Full broker route validated")

	return &MultiHopInfo{
		BrokerChain:   brokerId,
		BrokerChainId: brokerChainId,
		InboundRoute:  inboundRoute,
		OutboundRoute: outboundRoute,
		TokenIn:       &tokenIn,
		TokenOut:      tokenOut,
		SwapOnly:      false,
	}
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

// Solver orchestrates route finding and integrates with broker DEX APIs
type Solver struct {
	chainsMap     map[string]SolverChain
	routeIndex    *RouteIndex
	brokerClients map[string]BrokerClient // brokerId -> broker client interface
	denomResolver *DenomResolver
	maxRetries    int
	retryDelay    time.Duration
}

// NewSolver creates a new Solver with the given route index and broker clients
func NewSolver(chains []SolverChain, routeIndex *RouteIndex, brokerClients map[string]BrokerClient) *Solver {
	chainMap := make(map[string]SolverChain, len(chains))
	for _, chain := range chains {
		chainMap[chain.Id] = chain
	}
	return &Solver{
		chainsMap:     chainMap,
		routeIndex:    routeIndex,
		brokerClients: brokerClients,
		denomResolver: NewDenomResolver(routeIndex),
		maxRetries:    3,
		retryDelay:    500 * time.Millisecond,
	}
}

// Solve attempts to find a route for the given request and returns execution details
// Priority order: 1) Direct route, 2) Indirect route (no swap), 3) Broker swap route
func (s *Solver) Solve(req models.RouteRequest) models.RouteResponse {
	solverLog.Info().
		Str("chainFrom", req.ChainFrom).
		Str("chainTo", req.ChainTo).
		Str("tokenFrom", req.TokenFromDenom).
		Str("tokenTo", req.TokenToDenom).
		Str("amount", req.AmountIn).
		Msg("Solving route")

	// First, try to find a direct IBC route (no swap needed)
	directRoute := s.routeIndex.FindDirectRoute(req)
	if directRoute != nil {
		solverLog.Info().Msg("Found direct route")
		return s.buildDirectResponse(req, directRoute)
	}
	solverLog.Debug().Msg("No direct route found")

	// Second, try to find an indirect route (multi-hop without swap)
	indirectRoute := s.routeIndex.FindIndirectRoute(req)
	if indirectRoute != nil {
		solverLog.Info().Int("hops", len(indirectRoute.Path)-1).Msg("Found indirect route")
		return s.buildIndirectResponse(req, indirectRoute)
	}
	solverLog.Debug().Msg("No indirect route found")

	// Third, try multi-hop routes through brokers with swap
	brokerRoutes := s.routeIndex.FindMultiHopRoute(req)
	if len(brokerRoutes) == 0 {
		solverLog.Warn().Msg("No route found")
		return models.RouteResponse{
			Success:      false,
			RouteType:    "impossible",
			ErrorMessage: "No route found between chains for the requested tokens",
		}
	}

	solverLog.Info().Int("candidates", len(brokerRoutes)).Msg("Found broker route candidates")

	// Try each broker route and query the broker for swap details
	var lastErr error
	for i, hopInfo := range brokerRoutes {
		solverLog.Debug().
			Int("attempt", i+1).
			Str("broker", hopInfo.BrokerChain).
			Bool("swapOnly", hopInfo.SwapOnly).
			Msg("Trying broker route")

		response, err := s.buildBrokerSwapResponse(req, hopInfo)
		if err == nil {
			solverLog.Info().Str("broker", hopInfo.BrokerChain).Msg("Broker route succeeded")
			return response
		}
		lastErr = err
		solverLog.Debug().Err(err).Str("broker", hopInfo.BrokerChain).Msg("Broker route failed, trying next")
	}

	// All brokers failed or returned no valid route
	errMsg := "Broker swap route found but broker query failed"
	if lastErr != nil {
		errMsg = fmt.Sprintf("Broker swap route found but query failed: %v", lastErr)
	}
	solverLog.Warn().Err(lastErr).Msg("All broker routes failed")
	return models.RouteResponse{
		Success:      false,
		RouteType:    "impossible",
		ErrorMessage: errMsg,
	}
}

// buildDirectResponse creates a RouteResponse for a direct IBC transfer
func (s *Solver) buildDirectResponse(req models.RouteRequest, route *BasicRoute) models.RouteResponse {
	// Create token mapping for the source token
	tokenMapping, err := s.denomResolver.CreateTokenMapping(req.ChainFrom, req.TokenFromDenom)
	if err != nil {
		// Fallback to basic mapping if not found
		tokenMapping = &models.TokenMapping{
			ChainDenom:  req.TokenFromDenom,
			BaseDenom:   req.TokenFromDenom,
			OriginChain: req.ChainFrom,
			IsNative:    true,
		}
	}

	direct := &models.DirectRoute{
		Transfer: &models.IBCLeg{
			FromChain: req.ChainFrom,
			ToChain:   req.ChainTo,
			Channel:   route.ChannelId,
			Port:      route.PortId,
			Token:     tokenMapping,
			Amount:    req.AmountIn,
		},
	}

	return models.RouteResponse{
		Success:   true,
		RouteType: "direct",
		Direct:    direct,
	}
}

// buildIndirectResponse creates a RouteResponse for a multi-hop route without swaps
func (s *Solver) buildIndirectResponse(req models.RouteRequest, routeInfo *IndirectRouteInfo) models.RouteResponse {
	// Build IBC legs for each hop
	legs := []*models.IBCLeg{}
	currentDenom := req.TokenFromDenom
	amount := req.AmountIn

	for i, route := range routeInfo.Routes {
		fromChain := routeInfo.Path[i]
		toChain := routeInfo.Path[i+1]

		// Get token info on the current chain
		var tokenInfo *TokenInfo
		if i == 0 {
			tokenInfo = routeInfo.Token
		} else {
			tokenInfo = s.routeIndex.findTokenByOrigin(fromChain, routeInfo.Token.OriginChain, routeInfo.Token.BaseDenom)
		}

		if tokenInfo == nil {
			// Fallback
			tokenInfo = &TokenInfo{
				ChainDenom:  currentDenom,
				BaseDenom:   routeInfo.Token.BaseDenom,
				OriginChain: routeInfo.Token.OriginChain,
			}
		}

		tokenMapping := &models.TokenMapping{
			ChainDenom:  tokenInfo.ChainDenom,
			BaseDenom:   tokenInfo.BaseDenom,
			OriginChain: tokenInfo.OriginChain,
			IsNative:    s.denomResolver.IsTokenNativeToChain(tokenInfo, fromChain),
		}

		leg := &models.IBCLeg{
			FromChain: fromChain,
			ToChain:   toChain,
			Channel:   route.ChannelId,
			Port:      route.PortId,
			Token:     tokenMapping,
			Amount:    amount,
		}

		legs = append(legs, leg)
		currentDenom = tokenInfo.IbcDenom
	}

	// Check PFM support - all intermediate chains must support PFM
	supportsPFM := s.checkPFMSupport(routeInfo.Path)
	pfmMemo := ""

	if supportsPFM && len(routeInfo.Path) > 2 {
		pfmMemo = s.generatePFMMemo(legs, req.ReceiverAddress)
	}

	indirect := &models.IndirectRoute{
		Path:          routeInfo.Path,
		Legs:          legs,
		SupportsPFM:   supportsPFM,
		PFMStartChain: req.ChainFrom,
		PFMMemo:       pfmMemo,
	}

	return models.RouteResponse{
		Success:   true,
		RouteType: "indirect",
		Indirect:  indirect,
	}
}

// checkPFMSupport checks if all intermediate chains in the path support PFM
// For a path A -> B -> C, only B needs PFM support (the forwarding chain)
func (s *Solver) checkPFMSupport(path []string) bool {
	if len(path) <= 2 {
		return false // No intermediate chains
	}

	// Check all intermediate chains (exclude first and last)
	for i := 1; i < len(path)-1; i++ {
		if !s.routeIndex.pfmChains[path[i]] {
			return false
		}
	}

	return true
}

// generatePFMMemo generates an IBC memo for PFM forwarding
// Format: {"forward":{"receiver":"<addr>","port":"transfer","channel":"<channel>"}}
// For multi-hop, we nest the forward messages
func (s *Solver) generatePFMMemo(legs []*models.IBCLeg, finalReceiver string) string {
	if len(legs) == 0 {
		return ""
	}

	// Build memo from the last leg backwards
	var buildMemo func(legIndex int, receiver string) string
	buildMemo = func(legIndex int, receiver string) string {
		if legIndex >= len(legs) {
			return ""
		}

		leg := legs[legIndex]

		if legIndex == len(legs)-1 {
			// Last leg - use final receiver
			return fmt.Sprintf(`{"forward":{"receiver":"%s","port":"%s","channel":"%s"}}`,
				receiver, leg.Port, leg.Channel)
		}

		// Intermediate leg - nest the next memo
		nextMemo := buildMemo(legIndex+1, receiver)
		// For intermediate hops, the receiver should be the intermediate chain's module account
		// But in PFM, the memo itself handles forwarding, so we use a placeholder
		return fmt.Sprintf(`{"forward":{"receiver":"%s","port":"%s","channel":"%s","next":%s}}`,
			receiver, leg.Port, leg.Channel, nextMemo)
	}

	// Start from the first leg (after source chain)
	return buildMemo(1, finalReceiver)
}

// buildBrokerSwapResponse creates a RouteResponse for a broker swap route
func (s *Solver) buildBrokerSwapResponse(
	req models.RouteRequest,
	hopInfo *MultiHopInfo,
) (models.RouteResponse, error) {
	// Get the broker client for this broker chain
	brokerClient, exists := s.brokerClients[hopInfo.BrokerChain]
	if !exists {
		solverLog.Error().
			Str("brokerId", hopInfo.BrokerChain).
			Strs("availableBrokers", getMapKeys(s.brokerClients)).
			Msg("No client configured for broker")
		return models.RouteResponse{}, fmt.Errorf("no client configured for broker %s", hopInfo.BrokerChain)
	}

	// Determine the correct denoms to use on the broker chain
	// For the input token: use IbcDenom (what it becomes after IBC transfer to broker)
	tokenInDenomOnBroker := hopInfo.TokenIn.IbcDenom
	// For the output token: use ChainDenom (the denom on the broker chain)
	tokenOutDenomOnBroker := hopInfo.TokenOut.ChainDenom

	solverLog.Debug().
		Str("tokenIn", tokenInDenomOnBroker).
		Str("tokenOut", tokenOutDenomOnBroker).
		Str("amount", req.AmountIn).
		Bool("swapOnly", hopInfo.SwapOnly).
		Msg("Querying broker for swap")

	// Query with retry logic
	swapResult, err := s.queryBrokerWithRetry(brokerClient, req.AmountIn, tokenInDenomOnBroker, tokenOutDenomOnBroker)
	if err != nil {
		solverLog.Error().Err(err).Msg("Broker query failed")
		return models.RouteResponse{}, fmt.Errorf("broker query failed: %w", err)
	}

	// Build the broker swap route information
	brokerRoute, err := s.buildBrokerRoute(req, hopInfo, swapResult, brokerClient.GetBrokerType())
	if err != nil {
		return models.RouteResponse{}, fmt.Errorf("failed to build broker route: %w", err)
	}

	return models.RouteResponse{
		Success:    true,
		RouteType:  "broker_swap",
		BrokerSwap: brokerRoute,
	}, nil
}

// getMapKeys returns the keys of a map as a slice
func getMapKeys(m map[string]BrokerClient) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// queryBrokerWithRetry queries any broker DEX with exponential backoff retry logic
func (s *Solver) queryBrokerWithRetry(
	client BrokerClient,
	amountIn string,
	tokenInDenom string,
	tokenOutDenom string,
) (*SwapResult, error) {
	var lastErr error
	delay := s.retryDelay

	for attempt := 0; attempt <= s.maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(delay)
			delay *= 2 // Exponential backoff
		}

		// Query broker for the swap route
		result, err := client.QuerySwap(tokenInDenom, amountIn, tokenOutDenom)
		if err == nil {
			return result, nil
		}

		lastErr = err
	}

	return nil, fmt.Errorf("%s query failed after %d attempts: %w", client.GetBrokerType(), s.maxRetries+1, lastErr)
}

// buildBrokerRoute creates the broker swap route structure with PFM support
func (s *Solver) buildBrokerRoute(
	req models.RouteRequest,
	hopInfo *MultiHopInfo,
	swapResult *SwapResult,
	brokerType string,
) (*models.BrokerRoute, error) {
	// Create token mapping for inbound leg (source chain token)
	inboundTokenMapping := &models.TokenMapping{
		ChainDenom:  hopInfo.TokenIn.ChainDenom,
		BaseDenom:   hopInfo.TokenIn.BaseDenom,
		OriginChain: hopInfo.TokenIn.OriginChain,
		IsNative:    s.denomResolver.IsTokenNativeToChain(hopInfo.TokenIn, req.ChainFrom),
	}

	// Create token mapping for token on broker (after inbound IBC)
	tokenInOnBroker := &models.TokenMapping{
		ChainDenom:  hopInfo.TokenIn.IbcDenom,
		BaseDenom:   hopInfo.TokenIn.BaseDenom,
		OriginChain: hopInfo.TokenIn.OriginChain,
		IsNative:    s.denomResolver.IsTokenNativeToChain(hopInfo.TokenIn, hopInfo.BrokerChainId),
	}

	// Create token mapping for token out on broker (after swap)
	tokenOutOnBroker := &models.TokenMapping{
		ChainDenom:  hopInfo.TokenOut.ChainDenom,
		BaseDenom:   hopInfo.TokenOut.BaseDenom,
		OriginChain: hopInfo.TokenOut.OriginChain,
		IsNative:    s.denomResolver.IsTokenNativeToChain(hopInfo.TokenOut, hopInfo.BrokerChainId),
	}

	// Build the inbound IBC leg
	inboundLeg := &models.IBCLeg{
		FromChain: req.ChainFrom,
		ToChain:   hopInfo.BrokerChainId,
		Channel:   hopInfo.InboundRoute.ChannelId,
		Port:      hopInfo.InboundRoute.PortId,
		Token:     inboundTokenMapping,
		Amount:    req.AmountIn,
	}

	// Build the swap quote
	swap := &models.SwapQuote{
		Broker:       brokerType,
		TokenIn:      tokenInOnBroker,
		TokenOut:     tokenOutOnBroker,
		AmountIn:     swapResult.AmountIn,
		AmountOut:    swapResult.AmountOut,
		PriceImpact:  swapResult.PriceImpact,
		EffectiveFee: swapResult.EffectiveFee,
		RouteData:    swapResult.RouteData,
	}

	// Handle swap-only case (destination is the broker chain)
	if hopInfo.SwapOnly {
		solverLog.Debug().Msg("Building swap-only route (no outbound leg)")
		return &models.BrokerRoute{
			Path:                []string{req.ChainFrom, hopInfo.BrokerChainId},
			InboundLeg:          inboundLeg,
			Swap:                swap,
			OutboundLeg:         nil, // No outbound leg
			OutboundSupportsPFM: false,
			OutboundPFMMemo:     "",
		}, nil
	}

	// Build the outbound IBC leg
	outboundLeg := &models.IBCLeg{
		FromChain: hopInfo.BrokerChainId,
		ToChain:   req.ChainTo,
		Channel:   hopInfo.OutboundRoute.ChannelId,
		Port:      hopInfo.OutboundRoute.PortId,
		Token:     tokenOutOnBroker,
		Amount:    swapResult.AmountOut,
	}

	// Check if broker supports PFM for outbound forwarding
	// This allows the swap result to be automatically forwarded to the destination
	supportsPFM := s.routeIndex.pfmChains[hopInfo.BrokerChainId]
	pfmMemo := ""

	if supportsPFM {
		// Generate PFM memo for the outbound leg
		pfmMemo = fmt.Sprintf(`{"forward":{"receiver":"%s","port":"%s","channel":"%s"}}`,
			req.ReceiverAddress,
			outboundLeg.Port,
			outboundLeg.Channel,
		)
	}

	// Build the complete broker route
	return &models.BrokerRoute{
		Path:                []string{req.ChainFrom, hopInfo.BrokerChainId, req.ChainTo},
		InboundLeg:          inboundLeg,
		Swap:                swap,
		OutboundLeg:         outboundLeg,
		OutboundSupportsPFM: supportsPFM,
		OutboundPFMMemo:     pfmMemo,
	}, nil
}

/*
GetChainInfo returns the information about a specific chain

Parameters:
- chainId: the id of the chain to get information for

Returns:
- SolverChain: the information about the chain
- error: if the chain is not found
*/
func (s *Solver) GetChainInfo(chainId string) (SolverChain, error) {
	chain, exists := s.chainsMap[chainId]
	if !exists {
		return SolverChain{}, fmt.Errorf("chain %s not found", chainId)
	}
	return chain, nil
}

/*
GetAllChains returns the list of all chains

Returns:
- []string: the list of all chain ids
*/
func (s *Solver) GetAllChains() []string {
	chains := make([]string, 0, len(s.chainsMap))
	for chainId := range s.chainsMap {
		chains = append(chains, chainId)
	}
	return chains
}
