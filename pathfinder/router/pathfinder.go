package router

import (
	"fmt"
	"os"
	"time"

	models "github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/models"
	"github.com/rs/zerolog"
)

var pathfinderLog zerolog.Logger

func init() {
	out := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}
	pathfinderLog = zerolog.New(out).With().Timestamp().Str("component", "pathfinder").Logger()
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

// Pathfinder orchestrates route finding and integrates with broker DEX APIs
type Pathfinder struct {
	chainsMap        map[string]PathfinderChain  // mapped chainId -> PathfinderChain
	routeIndex       *RouteIndex             // routeIndex from which all routes are found
	brokerClients    map[string]BrokerClient // mapped brokerId -> broker client interface
	denomResolver    *DenomResolver          // denomResolver for resolving denoms across chains
	addressConverter *AddressConverter       // addressConverter for converting addresses across chains
	maxRetries       int                     // maximum number of retries for broker queries
	retryDelay       time.Duration           // delay between retries for broker queries
}

// NewPathfinder creates a new Pathfinder with the given route index and broker clients
func NewPathfinder(chains []PathfinderChain, routeIndex *RouteIndex, brokerClients map[string]BrokerClient) *Pathfinder {
	chainMap := make(map[string]PathfinderChain, len(chains))
	for _, chain := range chains {
		chainMap[chain.Id] = chain
	}
	return &Pathfinder{
		chainsMap:        chainMap,
		routeIndex:       routeIndex,
		brokerClients:    brokerClients,
		denomResolver:    NewDenomResolver(routeIndex),
		addressConverter: NewAddressConverter(chains),
		maxRetries:       3,
		retryDelay:       500 * time.Millisecond,
	}
}

// FindPath attempts to find a route for the given request and returns execution details
// Priority order: 1) Direct route, 2) Indirect route (no swap), 3) Broker swap route
func (s *Pathfinder) FindPath(req models.RouteRequest) models.RouteResponse {
	pathfinderLog.Info().
		Str("chainFrom", req.ChainFrom).
		Str("chainTo", req.ChainTo).
		Str("tokenFrom", req.TokenFromDenom).
		Str("tokenTo", req.TokenToDenom).
		Str("amount", req.AmountIn).
		Msg("Solving route")

	// First, try to find a direct IBC route (no swap needed)
	directRoute := s.routeIndex.FindDirectRoute(req)
	if directRoute != nil {
		pathfinderLog.Info().Msg("Found direct route")
		return s.buildDirectResponse(req, directRoute)
	}
	pathfinderLog.Debug().Msg("No direct route found")

	// Second, try to find an indirect route (multi-hop without swap)
	indirectRoute := s.routeIndex.FindIndirectRoute(req)
	if indirectRoute != nil {
		pathfinderLog.Info().Int("hops", len(indirectRoute.Path)-1).Msg("Found indirect route")
		return s.buildIndirectResponse(req, indirectRoute)
	}
	pathfinderLog.Debug().Msg("No indirect route found")

	// Third, try multi-hop routes through brokers with swap
	brokerRoutes := s.routeIndex.FindMultiHopRoute(req)
	if len(brokerRoutes) == 0 {
		pathfinderLog.Warn().Msg("No route found")
		return models.RouteResponse{
			Success:      false,
			RouteType:    "impossible",
			ErrorMessage: "No route found between chains for the requested tokens",
		}
	}

	pathfinderLog.Info().Int("candidates", len(brokerRoutes)).Msg("Found broker route candidates")

	// Try each broker route and query the broker for swap details
	var lastErr error
	for i, hopInfo := range brokerRoutes {
		pathfinderLog.Debug().
			Int("attempt", i+1).
			Str("broker", hopInfo.BrokerChain).
			Bool("swapOnly", hopInfo.SwapOnly).
			Msg("Trying broker route")

		response, err := s.buildBrokerSwapResponse(req, hopInfo)
		if err == nil {
			pathfinderLog.Info().Str("broker", hopInfo.BrokerChain).Msg("Broker route succeeded")
			return response
		}
		lastErr = err
		pathfinderLog.Debug().Err(err).Str("broker", hopInfo.BrokerChain).Msg("Broker route failed, trying next")
	}

	// All brokers failed or returned no valid route
	errMsg := "Broker swap route found but broker query failed"
	if lastErr != nil {
		errMsg = fmt.Sprintf("Broker swap route found but query failed: %v", lastErr)
	}
	pathfinderLog.Warn().Err(lastErr).Msg("All broker routes failed")
	return models.RouteResponse{
		Success:      false,
		RouteType:    "impossible",
		ErrorMessage: errMsg,
	}
}

// buildDirectResponse creates a RouteResponse for a direct IBC transfer
func (s *Pathfinder) buildDirectResponse(req models.RouteRequest, route *BasicRoute) models.RouteResponse {
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
func (s *Pathfinder) buildIndirectResponse(req models.RouteRequest, routeInfo *IndirectRouteInfo) models.RouteResponse {
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
func (s *Pathfinder) checkPFMSupport(path []string) bool {
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
func (s *Pathfinder) generatePFMMemo(legs []*models.IBCLeg, finalReceiver string) string {
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
func (s *Pathfinder) buildBrokerSwapResponse(
	req models.RouteRequest,
	hopInfo *MultiHopInfo,
) (models.RouteResponse, error) {
	// Get the broker client for this broker chain
	brokerClient, exists := s.brokerClients[hopInfo.BrokerChain]
	if !exists {
		pathfinderLog.Error().
			Str("brokerId", hopInfo.BrokerChain).
			Strs("availableBrokers", getMapKeys(s.brokerClients)).
			Msg("No client configured for broker")
		return models.RouteResponse{}, fmt.Errorf("no client configured for broker %s", hopInfo.BrokerChain)
	}

	// Determine the correct denoms to use on the broker chain
	var tokenInDenomOnBroker string
	if hopInfo.SourceIsBroker {
		// Source is the broker - token is already on broker, use ChainDenom directly
		tokenInDenomOnBroker = hopInfo.TokenIn.ChainDenom
	} else {
		// Source is not broker - token will arrive via IBC, use IbcDenom
		tokenInDenomOnBroker = hopInfo.TokenIn.IbcDenom
	}
	// For the output token: use TokenOutOnBroker.ChainDenom (the denom on the broker chain)
	tokenOutDenomOnBroker := hopInfo.TokenOutOnBroker.ChainDenom

	pathfinderLog.Debug().
		Str("tokenIn", tokenInDenomOnBroker).
		Str("tokenOut", tokenOutDenomOnBroker).
		Str("amount", req.AmountIn).
		Bool("swapOnly", hopInfo.SwapOnly).
		Bool("sourceIsBroker", hopInfo.SourceIsBroker).
		Msg("Querying broker for swap")

	// Query with retry logic
	swapResult, err := s.queryBrokerWithRetry(brokerClient, req.AmountIn, tokenInDenomOnBroker, tokenOutDenomOnBroker)
	if err != nil {
		pathfinderLog.Error().Err(err).Msg("Broker query failed")
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
func (s *Pathfinder) queryBrokerWithRetry(
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

// buildBrokerRoute creates the broker swap route structure with support for multiple legs.
// Handles:
// - Same-chain swap: no inbound/outbound legs
// - Source is broker: no inbound legs
// - Destination is broker: no outbound legs
// - Full route: both inbound and outbound legs
func (s *Pathfinder) buildBrokerRoute(
	req models.RouteRequest,
	hopInfo *MultiHopInfo,
	swapResult *SwapResult,
	brokerType string,
) (*models.BrokerRoute, error) {
	// Validate hopInfo has required fields
	if hopInfo == nil {
		return nil, fmt.Errorf("hopInfo is nil")
	}
	if hopInfo.TokenIn == nil {
		return nil, fmt.Errorf("hopInfo.TokenIn is nil")
	}
	if hopInfo.TokenOutOnBroker == nil {
		return nil, fmt.Errorf("hopInfo.TokenOutOnBroker is nil")
	}
	if swapResult == nil {
		return nil, fmt.Errorf("swapResult is nil")
	}

	// Get broker chain info for ibc-hooks contract
	brokerChain, brokerExists := s.chainsMap[hopInfo.BrokerChainId]

	// Build path based on route type
	var path []string
	if hopInfo.SourceIsBroker && hopInfo.SwapOnly {
		// Same-chain swap
		path = []string{hopInfo.BrokerChainId}
	} else if hopInfo.SourceIsBroker {
		// Source is broker, has outbound
		path = []string{hopInfo.BrokerChainId, req.ChainTo}
	} else if hopInfo.SwapOnly {
		// Dest is broker, has inbound
		path = []string{req.ChainFrom, hopInfo.BrokerChainId}
	} else {
		// Full route
		path = []string{req.ChainFrom, hopInfo.BrokerChainId, req.ChainTo}
	}

	// Build inbound leg (nil if source is broker)
	var inboundLeg *models.IBCLeg
	var tokenInOnBroker *models.TokenMapping

	if hopInfo.SourceIsBroker {
		// Source is broker - token is already on broker chain
		tokenInOnBroker = &models.TokenMapping{
			ChainDenom:  hopInfo.TokenIn.ChainDenom,
			BaseDenom:   hopInfo.TokenIn.BaseDenom,
			OriginChain: hopInfo.TokenIn.OriginChain,
			IsNative:    s.denomResolver.IsTokenNativeToChain(hopInfo.TokenIn, hopInfo.BrokerChainId),
		}
	} else {
		// Build inbound leg
		inboundTokenMapping := &models.TokenMapping{
			ChainDenom:  hopInfo.TokenIn.ChainDenom,
			BaseDenom:   hopInfo.TokenIn.BaseDenom,
			OriginChain: hopInfo.TokenIn.OriginChain,
			IsNative:    s.denomResolver.IsTokenNativeToChain(hopInfo.TokenIn, req.ChainFrom),
		}

		inboundLeg = &models.IBCLeg{
			FromChain: req.ChainFrom,
			ToChain:   hopInfo.BrokerChainId,
			Channel:   hopInfo.InboundRoute.ChannelId,
			Port:      hopInfo.InboundRoute.PortId,
			Token:     inboundTokenMapping,
			Amount:    req.AmountIn,
		}

		// Token on broker after IBC transfer
		tokenInOnBroker = &models.TokenMapping{
			ChainDenom:  hopInfo.TokenIn.IbcDenom,
			BaseDenom:   hopInfo.TokenIn.BaseDenom,
			OriginChain: hopInfo.TokenIn.OriginChain,
			IsNative:    s.denomResolver.IsTokenNativeToChain(hopInfo.TokenIn, hopInfo.BrokerChainId),
		}
	}

	// Create token mapping for token out on broker (after swap)
	tokenOutOnBroker := &models.TokenMapping{
		ChainDenom:  hopInfo.TokenOutOnBroker.ChainDenom,
		BaseDenom:   hopInfo.TokenOutOnBroker.BaseDenom,
		OriginChain: hopInfo.TokenOutOnBroker.OriginChain,
		IsNative:    s.denomResolver.IsTokenNativeToChain(hopInfo.TokenOutOnBroker, hopInfo.BrokerChainId),
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

	// Build outbound legs (empty if destination is broker)
	var outboundLegs []*models.IBCLeg
	supportsPFM := false

	if !hopInfo.SwapOnly && len(hopInfo.OutboundRoutes) > 0 {
		// Build leg for each outbound route
		currentAmount := swapResult.AmountOut
		currentToken := tokenOutOnBroker
		prevChain := hopInfo.BrokerChainId

		for i, route := range hopInfo.OutboundRoutes {
			// Determine the token for this leg
			var legToken *models.TokenMapping
			if i == 0 {
				legToken = currentToken
			} else if i < len(hopInfo.IntermediateTokens) {
				// Use intermediate token info
				intToken := hopInfo.IntermediateTokens[i-1]
				legToken = &models.TokenMapping{
					ChainDenom:  intToken.ChainDenom,
					BaseDenom:   intToken.BaseDenom,
					OriginChain: intToken.OriginChain,
					IsNative:    s.denomResolver.IsTokenNativeToChain(intToken, prevChain),
				}
			} else {
				legToken = currentToken
			}

			outboundLeg := &models.IBCLeg{
				FromChain: prevChain,
				ToChain:   route.ToChainId,
				Channel:   route.ChannelId,
				Port:      route.PortId,
				Token:     legToken,
				Amount:    currentAmount,
			}
			outboundLegs = append(outboundLegs, outboundLeg)
			prevChain = route.ToChainId
		}

		// PFM support check - all intermediate chains must support PFM
		supportsPFM = true
		for i := 0; i < len(hopInfo.OutboundRoutes)-1; i++ {
			if !s.routeIndex.pfmChains[hopInfo.OutboundRoutes[i].ToChainId] {
				supportsPFM = false
				break
			}
		}
	}

	// Build execution data based on route type
	var execution *models.BrokerExecutionData
	var err error

	if hopInfo.SourceIsBroker && hopInfo.SwapOnly {
		// Same-chain swap - just need swap data, no IBC memo
		pathfinderLog.Debug().Msg("Building same-chain swap route (no IBC)")
		execution = &models.BrokerExecutionData{
			UsesWasm:        false,
			Description:     "Same-chain swap on " + hopInfo.BrokerChainId,
			MinOutputAmount: swapResult.AmountOut, // TODO: Apply slippage
		}
	} else if hopInfo.SourceIsBroker {
		// Source is broker - need swap + outbound IBC
		pathfinderLog.Debug().Msg("Building broker-as-source route (swap + outbound)")
		// For now, use swap-only execution since no ibc-hooks needed when starting from broker
		execution = &models.BrokerExecutionData{
			UsesWasm:        false,
			Description:     "Swap on " + hopInfo.BrokerChainId + " then IBC to " + req.ChainTo,
			MinOutputAmount: swapResult.AmountOut,
		}
	} else if hopInfo.SwapOnly {
		// Destination is broker - need inbound IBC + swap
		pathfinderLog.Debug().Msg("Building swap-only route (inbound + swap)")
		execution, err = s.buildSwapOnlyExecution(req, hopInfo, swapResult, brokerChain, brokerExists)
		if err != nil {
			pathfinderLog.Warn().Err(err).Msg("Failed to build execution data, route still usable")
		}
	} else {
		// Full route - need inbound IBC + swap + outbound IBC
		pathfinderLog.Debug().Int("outboundHops", len(outboundLegs)).Msg("Building full broker route (inbound + swap + outbound)")
		execution, err = s.buildSwapAndForwardExecution(req, hopInfo, swapResult, outboundLegs, brokerChain, brokerExists)
		if err != nil {
			pathfinderLog.Warn().Err(err).Msg("Failed to build execution data, route still usable")
		}
	}

	return &models.BrokerRoute{
		Path:                path,
		InboundLeg:          inboundLeg,
		Swap:                swap,
		OutboundLegs:        outboundLegs,
		OutboundSupportsPFM: supportsPFM,
		Execution:           execution,
	}, nil
}

// buildSwapOnlyExecution builds execution data for swap-only routes (destination is broker)
func (s *Pathfinder) buildSwapOnlyExecution(
	req models.RouteRequest,
	hopInfo *MultiHopInfo,
	swapResult *SwapResult,
	brokerChain PathfinderChain,
	brokerExists bool,
) (*models.BrokerExecutionData, error) {
	if !brokerExists || brokerChain.IBCHooksContract == "" {
		return nil, fmt.Errorf("ibc-hooks contract not configured for broker")
	}

	// Derive addresses
	addresses, err := s.addressConverter.DeriveRouteAddresses(req.SenderAddress, hopInfo.BrokerChainId, req.ReceiverAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to derive addresses: %w", err)
	}

	// Calculate minimum output with 1% slippage
	minOutput, err := CalculateMinOutput(swapResult.AmountOut, 100) // 100 bps = 1%
	if err != nil {
		return nil, fmt.Errorf("failed to calculate min output: %w", err)
	}

	// Get route data
	routeData, ok := swapResult.RouteData.(*OsmosisRouteData)
	if !ok {
		return nil, fmt.Errorf("invalid route data type")
	}

	// Build wasm memo
	memoBuilder := NewWasmMemoBuilder(brokerChain.IBCHooksContract)
	memo, err := memoBuilder.BuildSwapMemo(SwapMemoParams{
		TokenInDenom:     hopInfo.TokenIn.IbcDenom,
		TokenOutDenom:    hopInfo.TokenOutOnBroker.ChainDenom,
		MinOutputAmount:  minOutput,
		RouteData:        routeData,
		TimeoutTimestamp: DefaultTimeoutTimestamp(),
		RecoverAddress:   addresses.BrokerAddress,
		PostSwapAction: PostSwapAction{
			TransferTo: addresses.DestinationAddress, // For swap-only, receiver is on broker chain
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build wasm memo: %w", err)
	}

	return &models.BrokerExecutionData{
		Memo:            memo,
		IBCReceiver:     brokerChain.IBCHooksContract,
		RecoverAddress:  addresses.BrokerAddress,
		MinOutputAmount: minOutput,
		UsesWasm:        true,
		Description:     fmt.Sprintf("IBC transfer with swap on %s", hopInfo.BrokerChainId),
	}, nil
}

// buildSwapAndForwardExecution builds execution data for swap+forward routes
// Supports multi-hop outbound via nested PFM memos
func (s *Pathfinder) buildSwapAndForwardExecution(
	req models.RouteRequest,
	hopInfo *MultiHopInfo,
	swapResult *SwapResult,
	outboundLegs []*models.IBCLeg,
	brokerChain PathfinderChain,
	brokerExists bool,
) (*models.BrokerExecutionData, error) {
	if !brokerExists || brokerChain.IBCHooksContract == "" {
		return nil, fmt.Errorf("ibc-hooks contract not configured for broker")
	}

	if len(outboundLegs) == 0 {
		return nil, fmt.Errorf("no outbound legs provided")
	}

	// Derive addresses
	addresses, err := s.addressConverter.DeriveRouteAddresses(req.SenderAddress, hopInfo.BrokerChainId, req.ReceiverAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to derive addresses: %w", err)
	}

	// Calculate minimum output with 1% slippage
	minOutput, err := CalculateMinOutput(swapResult.AmountOut, 100) // 100 bps = 1%
	if err != nil {
		return nil, fmt.Errorf("failed to calculate min output: %w", err)
	}

	// Get route data
	routeData, ok := swapResult.RouteData.(*OsmosisRouteData)
	if !ok {
		return nil, fmt.Errorf("invalid route data type")
	}

	// Build the PFM forward memo for hops after the first one (if any)
	// For multi-hop: wasm swap_and_action → ibc_transfer with nested PFM forward
	forwardMemo := ""
	if len(outboundLegs) > 1 {
		// Build nested forward memo from the last hop backwards
		forwardMemo = s.buildNestedForwardMemo(outboundLegs[1:], req.ReceiverAddress)
		pathfinderLog.Debug().Str("forwardMemo", forwardMemo).Msg("Built nested forward memo for multi-hop")
	}

	// Determine the receiver for the first IBC transfer (after swap)
	// If there are more hops, receiver should be on the intermediate chain
	firstHopReceiver := addresses.DestinationAddress
	if len(outboundLegs) > 1 {
		// For multi-hop, the receiver on the first intermediate chain
		// We need to derive the address for the intermediate chain
		intermediateChain := outboundLegs[0].ToChain
		intermediateAddr, err := s.addressConverter.ConvertAddress(req.ReceiverAddress, intermediateChain)
		if err != nil {
			// Fallback to destination address (PFM will use it anyway)
			pathfinderLog.Warn().Err(err).Str("chain", intermediateChain).Msg("Failed to derive intermediate address")
			intermediateAddr = addresses.DestinationAddress
		}
		firstHopReceiver = intermediateAddr
	}

	// Build wasm memo with IBC forwarding
	memoBuilder := NewWasmMemoBuilder(brokerChain.IBCHooksContract)
	memo, err := memoBuilder.BuildSwapMemo(SwapMemoParams{
		TokenInDenom:     hopInfo.TokenIn.IbcDenom,
		TokenOutDenom:    hopInfo.TokenOutOnBroker.ChainDenom,
		MinOutputAmount:  minOutput,
		RouteData:        routeData,
		TimeoutTimestamp: DefaultTimeoutTimestamp(),
		RecoverAddress:   addresses.BrokerAddress,
		PostSwapAction: PostSwapAction{
			IBCTransfer: &IBCTransferInfo{
				SourceChannel: outboundLegs[0].Channel,
				Receiver:      firstHopReceiver,
				Memo:          forwardMemo,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build wasm memo: %w", err)
	}

	// Build description
	description := fmt.Sprintf("IBC transfer with swap on %s", hopInfo.BrokerChainId)
	if len(outboundLegs) == 1 {
		description += fmt.Sprintf(" and forward to %s", req.ChainTo)
	} else {
		description += fmt.Sprintf(" and forward via %d hops to %s", len(outboundLegs), req.ChainTo)
	}

	return &models.BrokerExecutionData{
		Memo:            memo,
		IBCReceiver:     brokerChain.IBCHooksContract,
		RecoverAddress:  addresses.BrokerAddress,
		MinOutputAmount: minOutput,
		UsesWasm:        true,
		Description:     description,
	}, nil
}

// buildNestedForwardMemo builds a nested PFM forward memo for multi-hop forwarding
// legs should be the remaining legs after the first hop (from intermediate chains onwards)
func (s *Pathfinder) buildNestedForwardMemo(legs []*models.IBCLeg, finalReceiver string) string {
	if len(legs) == 0 {
		return ""
	}

	// Build from the last leg backwards
	var buildForward func(legIndex int) string
	buildForward = func(legIndex int) string {
		leg := legs[legIndex]

		if legIndex == len(legs)-1 {
			// Last hop - receiver is the final destination
			return fmt.Sprintf(`{"forward":{"receiver":"%s","port":"%s","channel":"%s"}}`,
				finalReceiver, leg.Port, leg.Channel)
		}

		// Intermediate hop - need to nest the next forward
		// For intermediate hops, receiver should be the address on the next chain
		// but PFM will override this, so we use finalReceiver
		nextMemo := buildForward(legIndex + 1)
		return fmt.Sprintf(`{"forward":{"receiver":"%s","port":"%s","channel":"%s","next":%s}}`,
			finalReceiver, leg.Port, leg.Channel, nextMemo)
	}

	return buildForward(0)
}

/*
GetChainInfo returns the information about a specific chain

Parameters:
- chainId: the id of the chain to get information for

Returns:
- PathfinderChain: the information about the chain
- error: if the chain is not found
*/
func (s *Pathfinder) GetChainInfo(chainId string) (PathfinderChain, error) {
	chain, exists := s.chainsMap[chainId]
	if !exists {
		return PathfinderChain{}, fmt.Errorf("chain %s not found", chainId)
	}
	return chain, nil
}

/*
GetAllChains returns the list of all chains

Returns:
- []string: the list of all chain ids
*/
func (s *Pathfinder) GetAllChains() []string {
	chains := make([]string, 0, len(s.chainsMap))
	for chainId := range s.chainsMap {
		chains = append(chains, chainId)
	}
	return chains
}
