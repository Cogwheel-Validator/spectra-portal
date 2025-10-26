package router

import (
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
	directRoutes      map[string]*BasicRoute              // "chainA->chainB->denom" 
	brokerRoutes      map[string]map[string]*BasicRoute    // brokerChainId -> toChainId -> BasicRoute
	chainToBrokerRoutes map[string]map[string]*BasicRoute    // chainId -> brokerChainId -> BasicRoute
	denomToTokenInfo  map[string]map[string]*TokenInfo    // chainId -> denom -> TokenInfo
	brokers           map[string]bool
}


func (ri *RouteIndex) BuildIndex(chains []SolverChain) {
	for _, chain := range chains {
		if ri.denomToTokenInfo[chain.Id] == nil {
			ri.denomToTokenInfo[chain.Id] = make(map[string]*TokenInfo)
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
	key := routeKey(req.ChainA, req.ChainB, req.TokenInDenom)
	if route, exists := ri.directRoutes[key]; exists {
		// Verify output denom matches (same token on both chains)
		tokenInfo := ri.denomToTokenInfo[req.ChainA][req.TokenInDenom]
		if tokenInfo != nil && tokenInfo.IbcDenom == req.TokenOutDenom {
			return route
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
		inboundRoute := ri.chainToBrokerRoutes[req.ChainA][brokerId]
		if inboundRoute == nil {
			continue
		}
		
		// Is input token allowed?
		tokenIn, tokenAllowed := inboundRoute.AllowedTokens[req.TokenInDenom]
		if !tokenAllowed {
			continue
		}
		
		// Can broker reach destination?
		outboundRoute := ri.brokerRoutes[brokerId][req.ChainB]
		if outboundRoute == nil {
			continue
		}
		
		// Is output token available on destination?
		tokenOut := ri.denomToTokenInfo[req.ChainB][req.TokenOutDenom]
		if tokenOut == nil {
			continue
		}
		
		// Check if output token is allowed on outbound route
		if _, allowed := outboundRoute.AllowedTokens[req.TokenOutDenom]; !allowed {
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
		directRoutes:     make(map[string]*BasicRoute),
		brokerRoutes:     make(map[string]map[string]*BasicRoute),
		chainToBrokerRoutes: make(map[string]map[string]*BasicRoute),
		denomToTokenInfo: make(map[string]map[string]*TokenInfo),
		brokers:          make(map[string]bool),
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
func (s *Solver) Solve(req models.RouteRequest) models.RouteResponse {
	// First, try to find a direct IBC route (no swap needed)
	directRoute := s.routeIndex.FindDirectRoute(req)
	if directRoute != nil {
		return s.buildDirectResponse(req, directRoute)
	}

	// If no direct route, try multi-hop routes through brokers
	multiHopRoutes := s.routeIndex.FindMultiHopRoute(req)
	if len(multiHopRoutes) == 0 {
		return models.RouteResponse{
			Success:      false,
			RouteType:    "impossible",
			ErrorMessage: "No route found between chains for the requested tokens",
		}
	}

	// Try each multi-hop route and query the broker for swap details
	for _, hopInfo := range multiHopRoutes {
		response, err := s.buildMultiHopResponse(req, hopInfo)
		if err == nil {
			return response
		}
		// If this broker failed, try the next one
	}

	// All brokers failed or returned no valid route
	return models.RouteResponse{
		Success:      false,
		RouteType:    "impossible",
		ErrorMessage: "Multi-hop route found but broker swap validation failed",
	}
}

// buildDirectResponse creates a RouteResponse for a direct IBC transfer
func (s *Solver) buildDirectResponse(req models.RouteRequest, route *BasicRoute) models.RouteResponse {
	// Create token mapping for the source token
	tokenMapping, err := s.denomResolver.CreateTokenMapping(req.ChainA, req.TokenInDenom)
	if err != nil {
		// Fallback to basic mapping if not found
		tokenMapping = &models.TokenMapping{
			ChainDenom:  req.TokenInDenom,
			BaseDenom:   req.TokenInDenom,
			OriginChain: req.ChainA,
			IsNative:    true,
		}
	}

	direct := &models.DirectRoute{
		Transfer: &models.IBCLeg{
			FromChain: req.ChainA,
			ToChain:   req.ChainB,
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

// buildMultiHopResponse creates a RouteResponse for a multi-hop route with swap
func (s *Solver) buildMultiHopResponse(req models.RouteRequest, hopInfo *MultiHopInfo) (models.RouteResponse, error) {
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

	// Build the multi-hop route information
	multiHop, err := s.buildMultiHopRoute(req, hopInfo, swapResult, brokerClient.GetBrokerType())
	if err != nil {
		return models.RouteResponse{}, fmt.Errorf("failed to build multi-hop route: %w", err)
	}

	return models.RouteResponse{
		Success:   true,
		RouteType: "multi_hop",
		MultiHop:  multiHop,
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

// buildMultiHopRoute creates the informative multi-hop route structure
func (s *Solver) buildMultiHopRoute(
	req models.RouteRequest,
	hopInfo *MultiHopInfo,
	swapResult *SwapResult,
	brokerType string,
) (*models.MultiHopRoute, error) {
	// Create token mapping for inbound leg (source chain token)
	inboundTokenMapping := &models.TokenMapping{
		ChainDenom:  hopInfo.TokenIn.ChainDenom,
		BaseDenom:   hopInfo.TokenIn.BaseDenom,
		OriginChain: hopInfo.TokenIn.OriginChain,
		IsNative:    s.denomResolver.IsTokenNativeToChain(hopInfo.TokenIn, req.ChainA),
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
		FromChain: req.ChainA,
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
		ToChain:   req.ChainB,
		Channel:   hopInfo.OutboundRoute.ChannelId,
		Port:      hopInfo.OutboundRoute.PortId,
		Token:     tokenOutOnBroker,
		Amount:    swapResult.AmountOut,
	}

	// Build the complete multi-hop route
	return &models.MultiHopRoute{
		Path:        []string{req.ChainA, hopInfo.BrokerChain, req.ChainB},
		InboundLeg:  inboundLeg,
		Swap:        swap,
		OutboundLeg: outboundLeg,
	}, nil
}

