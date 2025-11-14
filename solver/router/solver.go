package router

import (
	"container/list"
	"fmt"
	"time"

	models "github.com/Cogwheel-Validator/spectra-ibc-hub/solver/models"
)

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

type SolverChain struct {
	Name string
	Id string
	// if the chain is a broker, example: Osmosis, this will be true and the BrokerId will be the id of the broker
	HasPFM bool // has package forwaring middleware
	Broker bool
	BrokerId string
	Routes []BasicRoute
}

type BasicRoute struct {
	ToChain string
	ToChainId string
	ConnectionId string
	ChannelId string
	PortId string
	AllowedTokens map[string]TokenInfo
}

// TokenInfo contains comprehensive token information including origin tracking
type TokenInfo struct {
	// Denom on the current chain in the route context (native or IBC)
	ChainDenom  string
	// Denom on the destination chain (after IBC transfer)
	IbcDenom    string
	// Original native denom on the token's origin chain
	BaseDenom   string
	// Chain ID where this token is native
	OriginChain string
	// Number of decimal places
	Decimals    int
}

// RouteIndex with denom mapping should be internal logic for the router
type RouteIndex struct {
	directRoutes        map[string]*BasicRoute              // "chainA->chainB->denom" 
	brokerRoutes        map[string]map[string]*BasicRoute    // brokerChainId -> toChainId -> BasicRoute
	chainToBrokerRoutes map[string]map[string]*BasicRoute    // chainId -> brokerChainId -> BasicRoute
	denomToTokenInfo    map[string]map[string]*TokenInfo    // chainId -> denom -> TokenInfo
	brokers             map[string]bool                      // chainId -> is broker
	pfmChains           map[string]bool                      // chainId -> supports PFM
	chainRoutes         map[string]map[string]*BasicRoute    // chainId -> toChainId -> BasicRoute (all routes from a chain)
}


func (ri *RouteIndex) BuildIndex(chains []SolverChain) {
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

			// Index broker routes
			if ri.brokers[route.ToChainId] {
				if ri.chainToBrokerRoutes[chain.Id] == nil {
					ri.chainToBrokerRoutes[chain.Id] = make(map[string]*BasicRoute)
				}
				ri.chainToBrokerRoutes[chain.Id][route.ToChainId] = &route
			}
		}

		if chain.Broker {
			if ri.brokerRoutes[chain.BrokerId] == nil {
				ri.brokerRoutes[chain.BrokerId] = make(map[string]*BasicRoute)
			}
			for _, route := range chain.Routes {
				if _, exists := ri.brokerRoutes[chain.BrokerId][route.ToChainId]; !exists {
					ri.brokerRoutes[chain.BrokerId][route.ToChainId] = &route
				}
			}
			ri.brokers[chain.BrokerId] = true
		}
	}
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
	BrokerChain   string
	InboundRoute  *BasicRoute
	OutboundRoute *BasicRoute
	TokenIn       *TokenInfo
	TokenOut      *TokenInfo
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
func (ri *RouteIndex) FindMultiHopRoute(req models.RouteRequest) []*MultiHopInfo {
	multiHopInfos := []*MultiHopInfo{}
	// Check if we can route through a broker with token swap
	for brokerId := range ri.brokers {
		// Can we reach broker from source?
		inboundRoute := ri.chainToBrokerRoutes[req.ChainFrom][brokerId]
		if inboundRoute == nil {
			continue
		}
		
		// Is input token allowed?
		tokenIn, tokenAllowed := inboundRoute.AllowedTokens[req.TokenFromDenom]
		if !tokenAllowed {
			continue
		}
		
		// Can broker reach destination?
		outboundRoute := ri.brokerRoutes[brokerId][req.ChainTo]
		if outboundRoute == nil {
			continue
		}
		
		// Is output token available on destination?
		tokenOut := ri.denomToTokenInfo[req.ChainTo][req.TokenToDenom]
		if tokenOut == nil {
			continue
		}
		
		// Check if output token is allowed on outbound route
		if _, allowed := outboundRoute.AllowedTokens[req.TokenToDenom]; !allowed {
			continue
		}
		
		multiHopInfos = append(multiHopInfos, &MultiHopInfo{
			BrokerChain:   brokerId,
			InboundRoute:  inboundRoute,
			OutboundRoute: outboundRoute,
			TokenIn:       &tokenIn,
			TokenOut:      tokenOut,
		})
	}
	
	return multiHopInfos
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
		pfmChains:           make(map[string]bool),
		chainRoutes:         make(map[string]map[string]*BasicRoute),
	}
}

// Solver orchestrates route finding and integrates with broker DEX APIs
type Solver struct {
	routeIndex    *RouteIndex
	brokerClients map[string]BrokerClient // brokerId -> broker client interface
	denomResolver *DenomResolver
	maxRetries    int
	retryDelay    time.Duration
}

// NewSolver creates a new Solver with the given route index and broker clients
func NewSolver(routeIndex *RouteIndex, brokerClients map[string]BrokerClient) *Solver {
	return &Solver{
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
	// First, try to find a direct IBC route (no swap needed)
	directRoute := s.routeIndex.FindDirectRoute(req)
	if directRoute != nil {
		return s.buildDirectResponse(req, directRoute)
	}

	// Second, try to find an indirect route (multi-hop without swap)
	indirectRoute := s.routeIndex.FindIndirectRoute(req)
	if indirectRoute != nil {
		return s.buildIndirectResponse(req, indirectRoute)
	}

	// Third, try multi-hop routes through brokers with swap
	brokerRoutes := s.routeIndex.FindMultiHopRoute(req)
	if len(brokerRoutes) == 0 {
		return models.RouteResponse{
			Success:      false,
			RouteType:    "impossible",
			ErrorMessage: "No route found between chains for the requested tokens",
		}
	}

	// Try each broker route and query the broker for swap details
	for _, hopInfo := range brokerRoutes {
		response, err := s.buildBrokerSwapResponse(req, hopInfo)
		if err == nil {
			return response
		}
		// If this broker failed, try the next one
	}

	// All brokers failed or returned no valid route
	return models.RouteResponse{
		Success:      false,
		RouteType:    "impossible",
		ErrorMessage: "Broker swap route found but broker query failed",
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
		return models.RouteResponse{}, fmt.Errorf("no client configured for broker %s", hopInfo.BrokerChain)
	}

	// Determine the correct denoms to use on the broker chain
	tokenInDenomOnBroker := s.denomResolver.GetDenomOnChain(hopInfo.TokenIn, hopInfo.BrokerChain)
	tokenOutDenomOnBroker := s.denomResolver.GetDenomOnChain(hopInfo.TokenOut, hopInfo.BrokerChain)

	// Query with retry logic
	swapResult, err := s.queryBrokerWithRetry(brokerClient, req.AmountIn, tokenInDenomOnBroker, tokenOutDenomOnBroker)
	if err != nil {
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
		ChainDenom:  s.denomResolver.GetDenomOnChain(hopInfo.TokenIn, hopInfo.BrokerChain),
		BaseDenom:   hopInfo.TokenIn.BaseDenom,
		OriginChain: hopInfo.TokenIn.OriginChain,
		IsNative:    s.denomResolver.IsTokenNativeToChain(hopInfo.TokenIn, hopInfo.BrokerChain),
	}

	// Create token mapping for token out on broker (after swap)
	tokenOutOnBroker := &models.TokenMapping{
		ChainDenom:  s.denomResolver.GetDenomOnChain(hopInfo.TokenOut, hopInfo.BrokerChain),
		BaseDenom:   hopInfo.TokenOut.BaseDenom,
		OriginChain: hopInfo.TokenOut.OriginChain,
		IsNative:    s.denomResolver.IsTokenNativeToChain(hopInfo.TokenOut, hopInfo.BrokerChain),
	}

	// Build the inbound IBC leg
	inboundLeg := &models.IBCLeg{
		FromChain: req.ChainFrom,
		ToChain:   hopInfo.BrokerChain,
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

	// Build the outbound IBC leg
	outboundLeg := &models.IBCLeg{
		FromChain: hopInfo.BrokerChain,
		ToChain:   req.ChainTo,
		Channel:   hopInfo.OutboundRoute.ChannelId,
		Port:      hopInfo.OutboundRoute.PortId,
		Token:     tokenOutOnBroker,
		Amount:    swapResult.AmountOut,
	}
	
	// Check if broker supports PFM for outbound forwarding
	// This allows the swap result to be automatically forwarded to the destination
	supportsPFM := s.routeIndex.pfmChains[hopInfo.BrokerChain]
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
		Path:                []string{req.ChainFrom, hopInfo.BrokerChain, req.ChainTo},
		InboundLeg:          inboundLeg,
		Swap:                swap,
		OutboundLeg:         outboundLeg,
		OutboundSupportsPFM: supportsPFM,
		OutboundPFMMemo:     pfmMemo,
	}, nil
}

