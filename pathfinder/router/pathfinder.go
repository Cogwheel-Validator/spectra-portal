package router

import (
	"fmt"
	"os"
	"time"

	models "github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/models"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/router/brokers"
	ibcmemo "github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/router/ibc_memo"
	"github.com/rs/zerolog"
)

var pathfinderLog zerolog.Logger

func init() {
	out := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}
	pathfinderLog = zerolog.New(out).With().Timestamp().Str("component", "pathfinder").Logger()
}

// Pathfinder orchestrates route finding and integrates with broker DEX APIs
type Pathfinder struct {
	chainsMap        map[string]PathfinderChain      // mapped chainId -> PathfinderChain
	routeIndex       *RouteIndex                     // routeIndex from which all routes are found
	brokerClients    map[string]brokers.BrokerClient // mapped brokerId -> broker client interface
	denomResolver    *DenomResolver                  // denomResolver for resolving denoms across chains
	addressConverter *AddressConverter               // addressConverter for converting addresses across chains
	maxRetries       int                             // maximum number of retries for broker queries
	retryDelay       time.Duration                   // delay between retries for broker queries
}

// NewPathfinder creates a new Pathfinder with the given route index and broker clients
func NewPathfinder(chains []PathfinderChain, routeIndex *RouteIndex, brokerClients map[string]brokers.BrokerClient) *Pathfinder {
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

			// Validate that routeInfo.Token is not nil before using it
			if routeInfo.Token == nil {
				pathfinderLog.Error().Msg("Token information missing in route")
				return models.RouteResponse{
					Success:      false,
					RouteType:    "impossible",
					ErrorMessage: "Token information missing in route",
				}
			}

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

	// Determine the correct denoms to use on the broker chain (Osmosis SQS expects broker-chain denoms)
	var tokenInDenomOnBroker string
	if hopInfo.SourceIsBroker {
		// Source is the broker - token is already on broker, use ChainDenom directly
		tokenInDenomOnBroker = hopInfo.TokenIn.ChainDenom
	} else if len(hopInfo.InboundIntermediateTokens) > 0 {
		// Multi-hop inbound (e.g. Cosmos Hub → Noble → Osmosis): token arrives at broker via last hop.
		// TokenIn.IbcDenom is the denom on the first hop's destination (e.g. uusdc on Noble), not on the broker.
		// Use the last intermediate token's IbcDenom, which is the denom when the token lands on the broker.
		lastIntToken := hopInfo.InboundIntermediateTokens[len(hopInfo.InboundIntermediateTokens)-1]
		tokenInDenomOnBroker = lastIntToken.IbcDenom
	} else {
		// Single-hop inbound - token goes source → broker, use TokenIn.IbcDenom (denom on broker)
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
	swapResult, err := s.queryBrokerWithRetry(
		brokerClient, req.AmountIn, tokenInDenomOnBroker, tokenOutDenomOnBroker, req.SmartRoute)
	if err != nil {
		pathfinderLog.Error().Err(err).Msg("Broker query failed")
		return models.RouteResponse{}, fmt.Errorf("broker query failed: %w", err)
	}

	// Build the broker swap route information
	brokerRoute, err := s.buildBrokerRoute(req, hopInfo, swapResult, brokerClient)
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
func getMapKeys(m map[string]brokers.BrokerClient) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// queryBrokerWithRetry queries any broker DEX with exponential backoff retry logic
func (s *Pathfinder) queryBrokerWithRetry(
	client brokers.BrokerClient,
	amountIn string,
	tokenInDenom string,
	tokenOutDenom string,
	singleRoute *bool,
) (*brokers.SwapResult, error) {
	var lastErr error
	delay := s.retryDelay

	for attempt := 0; attempt <= s.maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(delay)
			delay *= 2 // Exponential backoff
		}

		// Query broker for the swap route
		result, err := client.QuerySwap(tokenInDenom, amountIn, tokenOutDenom, singleRoute)
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
	swapResult *brokers.SwapResult,
	brokerClient brokers.BrokerClient,
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

	// Leave it like this only for the tests... The proto will ALWAYS provide value
	// TODO: Refactor this in the future, it is not needed for program to function but tests rely on it
	if req.SlippageBps == nil {
		defaultSlippage := uint32(100) // 1% default slippage
		req.SlippageBps = &defaultSlippage
	}

	if *req.SlippageBps > 10000 {
		return nil, fmt.Errorf("slippage bps must be less than 10000")
	}

	// Get broker chain info and memo builder
	brokerChain, brokerExists := s.chainsMap[hopInfo.BrokerChainId]
	memoBuilder := brokerClient.GetMemoBuilder()
	brokerType := brokerClient.GetBrokerType()

	// Build path based on route type
	// Include intermediate chains in the path if there are multi-hop inbound routes
	var path []string
	if hopInfo.SourceIsBroker && hopInfo.SwapOnly {
		// Same-chain swap
		path = []string{hopInfo.BrokerChainId}
	} else if hopInfo.SourceIsBroker {
		// Source is broker, has outbound
		path = []string{hopInfo.BrokerChainId, req.ChainTo}
	} else if hopInfo.SwapOnly {
		// Dest is broker, has inbound (possibly multi-hop)
		path = append([]string{}, hopInfo.InboundPath...)
		path = append(path, hopInfo.BrokerChainId)
	} else {
		// Full route (possibly with multi-hop inbound)
		path = append([]string{}, hopInfo.InboundPath...)
		path = append(path, hopInfo.BrokerChainId, req.ChainTo)
	}

	// Build inbound legs (nil if source is broker)
	var inboundLegs []*models.IBCLeg
	var tokenInOnBroker *models.TokenMapping

	if hopInfo.SourceIsBroker {
		// Source is broker - token is already on broker chain
		tokenInOnBroker = &models.TokenMapping{
			ChainDenom:  hopInfo.TokenIn.ChainDenom,
			BaseDenom:   hopInfo.TokenIn.BaseDenom,
			OriginChain: hopInfo.TokenIn.OriginChain,
			IsNative:    s.denomResolver.IsTokenNativeToChain(hopInfo.TokenIn, hopInfo.BrokerChainId),
		}
	} else if len(hopInfo.InboundRoutes) > 0 {
		// Build inbound legs - can be single or multi-hop
		inboundLegs = make([]*models.IBCLeg, 0, len(hopInfo.InboundRoutes))

		// Track the current token as it travels through the hops
		currentTokenDenom := hopInfo.TokenIn.ChainDenom
		currentTokenInfo := hopInfo.TokenIn

		for i, route := range hopInfo.InboundRoutes {
			fromChain := hopInfo.InboundPath[i]

			// Determine the destination chain for this hop
			var toChain string
			if i < len(hopInfo.InboundRoutes)-1 {
				// Intermediate hop - destination is next chain in path
				toChain = hopInfo.InboundPath[i+1]
			} else {
				// Last hop - destination is broker
				toChain = hopInfo.BrokerChainId
			}

			// Get the token info for this hop
			var legTokenMapping *models.TokenMapping
			if i == 0 {
				// First hop uses the original token from source
				legTokenMapping = &models.TokenMapping{
					ChainDenom:  currentTokenDenom,
					BaseDenom:   hopInfo.TokenIn.BaseDenom,
					OriginChain: hopInfo.TokenIn.OriginChain,
					IsNative:    s.denomResolver.IsTokenNativeToChain(currentTokenInfo, fromChain),
				}
			} else {
				// Subsequent hops use intermediate token info
				intToken := hopInfo.InboundIntermediateTokens[i-1]
				legTokenMapping = &models.TokenMapping{
					ChainDenom:  intToken.ChainDenom,
					BaseDenom:   intToken.BaseDenom,
					OriginChain: intToken.OriginChain,
					IsNative:    s.denomResolver.IsTokenNativeToChain(intToken, fromChain),
				}
				currentTokenInfo = intToken
			}

			leg := &models.IBCLeg{
				FromChain: fromChain,
				ToChain:   toChain,
				Channel:   route.ChannelId,
				Port:      route.PortId,
				Token:     legTokenMapping,
				Amount:    req.AmountIn, // Amount stays the same through pure transfers
			}
			inboundLegs = append(inboundLegs, leg)

			// Update the token denom for the next hop (it becomes an IBC denom)
			if i == 0 {
				currentTokenDenom = hopInfo.TokenIn.IbcDenom
			} else if i < len(hopInfo.InboundIntermediateTokens) {
				currentTokenDenom = hopInfo.InboundIntermediateTokens[i-1].IbcDenom
			}
		}

		// Token on broker after all IBC transfers
		// Use the last intermediate token's IbcDenom, or the TokenIn.IbcDenom if single hop
		if len(hopInfo.InboundIntermediateTokens) > 0 {
			lastIntToken := hopInfo.InboundIntermediateTokens[len(hopInfo.InboundIntermediateTokens)-1]
			tokenInOnBroker = &models.TokenMapping{
				ChainDenom:  lastIntToken.IbcDenom,
				BaseDenom:   hopInfo.TokenIn.BaseDenom,
				OriginChain: hopInfo.TokenIn.OriginChain,
				IsNative:    s.denomResolver.IsTokenNativeToChain(lastIntToken, hopInfo.BrokerChainId),
			}
		} else {
			// Single hop inbound
			tokenInOnBroker = &models.TokenMapping{
				ChainDenom:  hopInfo.TokenIn.IbcDenom,
				BaseDenom:   hopInfo.TokenIn.BaseDenom,
				OriginChain: hopInfo.TokenIn.OriginChain,
				IsNative:    s.denomResolver.IsTokenNativeToChain(hopInfo.TokenIn, hopInfo.BrokerChainId),
			}
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

	// Build execution data based on route type and SmartRoute flag
	// SmartRoute controls whether to generate execution data:
	// - nil/false: Manual route - just return route info, no execution data
	// - true: Smart route - generate memo (IBC) or smart contract data (direct)
	var execution *models.BrokerExecutionData
	var err error

	// Only build execution data if SmartRoute is explicitly true
	if req.SmartRoute != nil && *req.SmartRoute {
		smartContractBuilder := brokerClient.GetSmartContractBuilder()

		if hopInfo.SourceIsBroker && hopInfo.SwapOnly {
			// Same-chain swap: Source == Broker == Destination
			// Use smart contract data (direct contract call, no IBC)
			pathfinderLog.Debug().Msg("Building same-chain swap route (smart contract)")
			execution, err = s.buildSmartContractSwapExecution(req, hopInfo, swapResult, smartContractBuilder, brokerChain, brokerExists)
			if err != nil {
				pathfinderLog.Warn().Err(err).Msg("Failed to build smart contract data, route still usable")
			}
		} else if hopInfo.SourceIsBroker {
			// Source is broker, dest is not: swap + outbound IBC
			// Use smart contract data with IBC forward built-in
			pathfinderLog.Debug().Msg("Building broker-as-source route (smart contract + IBC forward)")
			execution, err = s.buildSmartContractSwapAndForwardExecution(req, hopInfo, swapResult, outboundLegs, smartContractBuilder, brokerChain, brokerExists)
			if err != nil {
				pathfinderLog.Warn().Err(err).Msg("Failed to build smart contract data, route still usable")
			}
		} else if hopInfo.SwapOnly {
			// Source is not broker, dest is broker: inbound IBC + swap
			// Use IBC memo (ibc-hooks will trigger swap)
			pathfinderLog.Debug().Msg("Building swap-only route (IBC memo)")
			execution, err = s.buildSwapOnlyExecution(req, hopInfo, swapResult, memoBuilder, brokerChain, brokerExists)
			if err != nil {
				pathfinderLog.Warn().Err(err).Msg("Failed to build execution data, route still usable")
			}
		} else {
			// Full route: source -> broker -> dest (all different chains)
			// Use IBC memo (ibc-hooks will trigger swap + forward)
			pathfinderLog.Debug().Int("outboundHops", len(outboundLegs)).Msg("Building full broker route (IBC memo)")
			execution, err = s.buildSwapAndForwardExecution(req, hopInfo, swapResult, outboundLegs, memoBuilder, brokerExists)
			if err != nil {
				pathfinderLog.Warn().Err(err).Msg("Failed to build execution data, route still usable")
			}
		}
	} else {
		pathfinderLog.Debug().Msg("Manual route - skipping execution data generation")
	}

	return &models.BrokerRoute{
		Path:                path,
		InboundLegs:         inboundLegs,
		Swap:                swap,
		OutboundLegs:        outboundLegs,
		OutboundSupportsPFM: supportsPFM,
		Execution:           execution,
	}, nil
}

// buildSwapOnlyExecution builds execution data for swap-only routes (destination is broker)
// Supports both single-hop and multi-hop inbound paths
func (s *Pathfinder) buildSwapOnlyExecution(
	req models.RouteRequest,
	hopInfo *MultiHopInfo,
	swapResult *brokers.SwapResult,
	memoBuilder ibcmemo.MemoBuilder,
	brokerChain PathfinderChain,
	brokerExists bool,
) (*models.BrokerExecutionData, error) {
	if !brokerExists {
		return nil, fmt.Errorf("broker chain %s not found", hopInfo.BrokerChainId)
	}
	// Check if there is IBC hook, only needed for now but when more "Broker Chains" are added
	// some of these checks will need to be modified
	contractAddress := memoBuilder.GetContractAddress()
	if contractAddress == "" {
		return nil, fmt.Errorf("ibc-hooks contract not configured for broker %s", hopInfo.BrokerChainId)
	}

	// Derive addresses
	addresses, err := s.addressConverter.DeriveRouteAddresses(req.SenderAddress, hopInfo.BrokerChainId, req.ReceiverAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to derive addresses: %w", err)
	}

	// Calculate minimum output with slippage
	minOutput := swapResult.AmountOut
	if req.SlippageBps != nil {
		calculated, calcErr := brokers.CalculateMinOutput(swapResult.AmountOut, *req.SlippageBps)
		if calcErr != nil {
			// If for some reason it does fail at least try to return some value
			pathfinderLog.Warn().Err(calcErr).Msg("Failed to calculate min output, using original amount")
		} else {
			minOutput = calculated
		}
	}

	// Determine token in denom on broker (after all inbound IBC transfers)
	var tokenInDenomOnBroker string
	if len(hopInfo.InboundIntermediateTokens) > 0 {
		// Multi-hop: use the IBC denom from the last intermediate token
		lastIntToken := hopInfo.InboundIntermediateTokens[len(hopInfo.InboundIntermediateTokens)-1]
		tokenInDenomOnBroker = lastIntToken.IbcDenom
	} else {
		// Single hop: use the direct IBC denom
		tokenInDenomOnBroker = hopInfo.TokenIn.IbcDenom
	}

	var memo string

	// Check if we need multi-hop inbound (Forward + Swap)
	if len(hopInfo.InboundRoutes) > 1 {
		// Multi-hop inbound: build Forward + Swap memo.
		// The memo is attached to the first IBC transfer (source → first intermediate).
		// It must describe only the remaining hops (first intermediate → broker → ...).
		inboundHops := s.buildInboundHops(hopInfo, req)
		memoInboundHops := inboundHops[1:]

		memo, err = memoBuilder.BuildForwardSwapMemo(ibcmemo.ForwardSwapParams{
			InboundHops: memoInboundHops,
			SwapParams: ibcmemo.SwapAndForwardParams{
				SwapMemoParams: ibcmemo.SwapMemoParams{
					TokenInDenom:     tokenInDenomOnBroker,
					TokenOutDenom:    hopInfo.TokenOutOnBroker.ChainDenom,
					MinOutputAmount:  minOutput,
					RouteData:        swapResult.RouteData,
					TimeoutTimestamp: ibcmemo.DefaultTimeoutTimestamp(),
					RecoverAddress:   addresses.BrokerAddress,
					ReceiverAddress:  addresses.DestinationAddress,
				},
				// For swap-only with multi-hop inbound, there's no forward after swap
				SourceChannel:   "", // No forward after swap
				ForwardReceiver: addresses.DestinationAddress,
				ForwardMemo:     "",
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to build forward+swap memo: %w", err)
		}
	} else {
		// Single-hop inbound: build simple Swap memo
		memo, err = memoBuilder.BuildSwapMemo(ibcmemo.SwapMemoParams{
			TokenInDenom:     tokenInDenomOnBroker,
			TokenOutDenom:    hopInfo.TokenOutOnBroker.ChainDenom,
			MinOutputAmount:  minOutput,
			RouteData:        swapResult.RouteData,
			TimeoutTimestamp: ibcmemo.DefaultTimeoutTimestamp(),
			RecoverAddress:   addresses.BrokerAddress,
			ReceiverAddress:  addresses.DestinationAddress, // For swap-only, receiver is on broker chain
		})
		if err != nil {
			return nil, fmt.Errorf("failed to build wasm memo: %w", err)
		}
	}

	return &models.BrokerExecutionData{
		Memo:            &memo,
		IBCReceiver:     &contractAddress,
		RecoverAddress:  &addresses.BrokerAddress,
		MinOutputAmount: minOutput,
		UsesWasm:        true,
		Description:     fmt.Sprintf("IBC transfer with swap on %s", hopInfo.BrokerChainId),
	}, nil
}

// buildSwapAndForwardExecution builds execution data for swap+forward routes
// Supports both single-hop and multi-hop inbound/outbound via nested PFM memos
func (s *Pathfinder) buildSwapAndForwardExecution(
	req models.RouteRequest,
	hopInfo *MultiHopInfo,
	swapResult *brokers.SwapResult,
	outboundLegs []*models.IBCLeg,
	memoBuilder ibcmemo.MemoBuilder,
	brokerExists bool,
) (*models.BrokerExecutionData, error) {
	contractAddress := memoBuilder.GetContractAddress()
	if !brokerExists || contractAddress == "" {
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

	// Calculate minimum output with slippage
	minOutput := swapResult.AmountOut
	if req.SlippageBps != nil {
		calculated, calcErr := brokers.CalculateMinOutput(swapResult.AmountOut, *req.SlippageBps)
		if calcErr != nil {
			// If for some reason it does fail at least try to return some value
			pathfinderLog.Warn().Err(calcErr).Msg("Failed to calculate min output, using original amount")
		} else {
			minOutput = calculated
		}
	}

	// Determine token in denom on broker (after all inbound IBC transfers)
	var tokenInDenomOnBroker string
	if len(hopInfo.InboundIntermediateTokens) > 0 {
		// Multi-hop inbound: use the IBC denom from the last intermediate token
		lastIntToken := hopInfo.InboundIntermediateTokens[len(hopInfo.InboundIntermediateTokens)-1]
		tokenInDenomOnBroker = lastIntToken.IbcDenom
	} else {
		// Single hop inbound: use the direct IBC denom
		tokenInDenomOnBroker = hopInfo.TokenIn.IbcDenom
	}

	// Determine the receiver for the first IBC transfer (after swap)
	// If there are more hops, receiver should be on the intermediate chain
	firstHopReceiver := addresses.DestinationAddress
	if len(outboundLegs) > 1 {
		// For multi-hop, the receiver on the first intermediate chain
		// We need to derive the address for the intermediate chain
		intermediateChain := outboundLegs[0].ToChain
		intermediateAddr, addrErr := s.addressConverter.ConvertAddress(req.ReceiverAddress, intermediateChain)
		if addrErr != nil {
			// Fallback to destination address (PFM will use it anyway)
			pathfinderLog.Warn().Err(addrErr).Str("chain", intermediateChain).Msg("Failed to derive intermediate address")
			intermediateAddr = addresses.DestinationAddress
		}
		firstHopReceiver = intermediateAddr
	}

	// Build the memo based on inbound and outbound hop counts
	var memo string
	hasMultiHopInbound := len(hopInfo.InboundRoutes) > 1
	hasMultiHopOutbound := len(outboundLegs) > 1

	if hasMultiHopInbound {
		// Multi-hop inbound: use ForwardSwap or ForwardSwapForward.
		// The memo is attached to the first IBC transfer (source → first intermediate).
		// It must describe only the remaining hops (first intermediate → broker → ...).
		inboundHops := s.buildInboundHops(hopInfo, req)
		memoInboundHops := inboundHops[1:]

		if hasMultiHopOutbound {
			// Forward + Swap + MultiHop Forward (case 5.4)
			outboundHops := s.buildOutboundHops(outboundLegs, addresses.DestinationAddress, req)

			memo, err = memoBuilder.BuildForwardSwapForwardMemo(ibcmemo.ForwardSwapForwardParams{
				InboundHops: memoInboundHops,
				SwapParams: ibcmemo.SwapAndMultiHopParams{
					SwapMemoParams: ibcmemo.SwapMemoParams{
						TokenInDenom:     tokenInDenomOnBroker,
						TokenOutDenom:    hopInfo.TokenOutOnBroker.ChainDenom,
						MinOutputAmount:  minOutput,
						RouteData:        swapResult.RouteData,
						TimeoutTimestamp: ibcmemo.DefaultTimeoutTimestamp(),
						RecoverAddress:   addresses.BrokerAddress,
						ReceiverAddress:  firstHopReceiver,
					},
					OutboundHops:  outboundHops,
					FinalReceiver: addresses.DestinationAddress,
				},
			})
		} else {
			// Forward + Swap + Single Forward (case 5.2)
			memo, err = memoBuilder.BuildForwardSwapMemo(ibcmemo.ForwardSwapParams{
				InboundHops: memoInboundHops,
				SwapParams: ibcmemo.SwapAndForwardParams{
					SwapMemoParams: ibcmemo.SwapMemoParams{
						TokenInDenom:     tokenInDenomOnBroker,
						TokenOutDenom:    hopInfo.TokenOutOnBroker.ChainDenom,
						MinOutputAmount:  minOutput,
						RouteData:        swapResult.RouteData,
						TimeoutTimestamp: ibcmemo.DefaultTimeoutTimestamp(),
						RecoverAddress:   addresses.BrokerAddress,
						ReceiverAddress:  firstHopReceiver,
					},
					SourceChannel:   outboundLegs[0].Channel,
					ForwardReceiver: firstHopReceiver,
					ForwardMemo:     "",
				},
			})
		}
	} else {
		// Single-hop inbound
		if hasMultiHopOutbound {
			// Swap + MultiHop Forward (case 5.3)
			outboundHops := s.buildOutboundHops(outboundLegs, addresses.DestinationAddress, req)

			memo, err = memoBuilder.BuildSwapAndMultiHopMemo(ibcmemo.SwapAndMultiHopParams{
				SwapMemoParams: ibcmemo.SwapMemoParams{
					TokenInDenom:     tokenInDenomOnBroker,
					TokenOutDenom:    hopInfo.TokenOutOnBroker.ChainDenom,
					MinOutputAmount:  minOutput,
					RouteData:        swapResult.RouteData,
					TimeoutTimestamp: ibcmemo.DefaultTimeoutTimestamp(),
					RecoverAddress:   addresses.BrokerAddress,
					ReceiverAddress:  firstHopReceiver,
				},
				OutboundHops:  outboundHops,
				FinalReceiver: addresses.DestinationAddress,
			})
		} else {
			// Swap + Single Forward (case 5.1)
			memo, err = memoBuilder.BuildSwapAndForwardMemo(ibcmemo.SwapAndForwardParams{
				SwapMemoParams: ibcmemo.SwapMemoParams{
					TokenInDenom:     tokenInDenomOnBroker,
					TokenOutDenom:    hopInfo.TokenOutOnBroker.ChainDenom,
					MinOutputAmount:  minOutput,
					RouteData:        swapResult.RouteData,
					TimeoutTimestamp: ibcmemo.DefaultTimeoutTimestamp(),
					RecoverAddress:   addresses.BrokerAddress,
					ReceiverAddress:  firstHopReceiver,
				},
				SourceChannel:   outboundLegs[0].Channel,
				ForwardReceiver: firstHopReceiver,
				ForwardMemo:     "",
			})
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to build wasm memo: %w", err)
	}

	// Build description
	description := fmt.Sprintf("IBC transfer with swap on %s", hopInfo.BrokerChainId)
	if hasMultiHopInbound {
		description = fmt.Sprintf("Multi-hop IBC (%d hops) with swap on %s", len(hopInfo.InboundRoutes), hopInfo.BrokerChainId)
	}
	if len(outboundLegs) == 1 {
		description += fmt.Sprintf(" and forward to %s", req.ChainTo)
	} else {
		description += fmt.Sprintf(" and forward via %d hops to %s", len(outboundLegs), req.ChainTo)
	}

	return &models.BrokerExecutionData{
		Memo:            &memo,
		IBCReceiver:     &contractAddress,
		RecoverAddress:  &addresses.BrokerAddress,
		MinOutputAmount: minOutput,
		UsesWasm:        true,
		Description:     description,
	}, nil
}

// buildInboundHops converts inbound routes to IBCHop slice for memo building.
// For intermediate hops, Receiver is set to the address on that hop's destination chain
// (via the address converter). The last hop's receiver is left empty; the memo builder
// uses the broker contract address for it.
func (s *Pathfinder) buildInboundHops(hopInfo *MultiHopInfo, req models.RouteRequest) []ibcmemo.IBCHop {
	hops := make([]ibcmemo.IBCHop, len(hopInfo.InboundRoutes))
	for i, route := range hopInfo.InboundRoutes {
		receiver := ""
		if i < len(hopInfo.InboundRoutes)-1 {
			// Intermediate hop: derive receiver address for the destination chain
			addr, err := s.addressConverter.ConvertAddress(req.ReceiverAddress, route.ToChainId)
			if err == nil {
				receiver = addr
			}
		}
		hops[i] = ibcmemo.IBCHop{
			Channel:  route.ChannelId,
			Port:     route.PortId,
			Receiver: receiver,
			Timeout:  ibcmemo.DefaultTimeoutTimestamp(),
		}
	}
	return hops
}

// buildSmartContractSwapExecution builds execution data for same-chain swap (source == broker == dest)
// Returns smart contract data instead of IBC memo since no IBC transfer is needed.
func (s *Pathfinder) buildSmartContractSwapExecution(
	req models.RouteRequest,
	hopInfo *MultiHopInfo,
	swapResult *brokers.SwapResult,
	scBuilder brokers.SmartContractBuilder,
	brokerChain PathfinderChain,
	brokerExists bool,
) (*models.BrokerExecutionData, error) {
	if !brokerExists {
		return nil, fmt.Errorf("broker chain %s not found", hopInfo.BrokerChainId)
	}

	// Calculate minimum output with slippage
	minOutput := swapResult.AmountOut
	if req.SlippageBps != nil {
		calculated, calcErr := brokers.CalculateMinOutput(swapResult.AmountOut, *req.SlippageBps)
		if calcErr != nil {
			pathfinderLog.Warn().Err(calcErr).Msg("Failed to calculate min output, using original amount")
		} else {
			minOutput = calculated
		}
	}

	// Build smart contract data for same-chain swap
	scData, err := scBuilder.BuildSwapAndTransfer(ibcmemo.SwapMemoParams{
		TokenInDenom:     hopInfo.TokenIn.ChainDenom, // Native denom since source is broker
		TokenOutDenom:    hopInfo.TokenOutOnBroker.ChainDenom,
		MinOutputAmount:  minOutput,
		RouteData:        swapResult.RouteData,
		TimeoutTimestamp: ibcmemo.DefaultTimeoutTimestamp(),
		RecoverAddress:   req.SenderAddress,   // On same chain, use sender as recover
		ReceiverAddress:  req.ReceiverAddress, // Final destination on same chain
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build smart contract data: %w", err)
	}

	return &models.BrokerExecutionData{
		SmartContractData: scData,
		MinOutputAmount:   minOutput,
		UsesWasm:          true,
		Description:       fmt.Sprintf("Smart contract swap on %s", hopInfo.BrokerChainId),
	}, nil
}

// buildSmartContractSwapAndForwardExecution builds execution data for broker-as-source routes
// (source == broker, dest != broker). Returns smart contract data with IBC forward built-in.
func (s *Pathfinder) buildSmartContractSwapAndForwardExecution(
	req models.RouteRequest,
	hopInfo *MultiHopInfo,
	swapResult *brokers.SwapResult,
	outboundLegs []*models.IBCLeg,
	scBuilder brokers.SmartContractBuilder,
	brokerChain PathfinderChain,
	brokerExists bool,
) (*models.BrokerExecutionData, error) {
	if !brokerExists {
		return nil, fmt.Errorf("broker chain %s not found", hopInfo.BrokerChainId)
	}
	if len(outboundLegs) == 0 {
		return nil, fmt.Errorf("no outbound legs for swap-and-forward")
	}

	// Calculate minimum output with slippage
	minOutput := swapResult.AmountOut
	if req.SlippageBps != nil {
		calculated, calcErr := brokers.CalculateMinOutput(swapResult.AmountOut, *req.SlippageBps)
		if calcErr != nil {
			pathfinderLog.Warn().Err(calcErr).Msg("Failed to calculate min output, using original amount")
		} else {
			minOutput = calculated
		}
	}

	// For single outbound hop, use simple swap+forward
	// For multi-hop, we'd need PFM memo in the forward action
	firstLeg := outboundLegs[0]
	var forwardMemo string
	if len(outboundLegs) > 1 {
		// Build PFM memo for remaining hops
		forwardMemo = s.generatePFMMemo(outboundLegs[1:], req.ReceiverAddress)
	}

	// Build smart contract data for swap + IBC forward
	scData, err := scBuilder.BuildSwapAndForward(ibcmemo.SwapAndForwardParams{
		SwapMemoParams: ibcmemo.SwapMemoParams{
			TokenInDenom:     hopInfo.TokenIn.ChainDenom, // Native denom since source is broker
			TokenOutDenom:    hopInfo.TokenOutOnBroker.ChainDenom,
			MinOutputAmount:  minOutput,
			RouteData:        swapResult.RouteData,
			TimeoutTimestamp: ibcmemo.DefaultTimeoutTimestamp(),
			RecoverAddress:   req.SenderAddress,   // On broker, use sender as recover
			ReceiverAddress:  req.ReceiverAddress, // Used for forward action
		},
		SourceChannel:   firstLeg.Channel,
		ForwardReceiver: req.ReceiverAddress,
		ForwardMemo:     forwardMemo,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build smart contract data: %w", err)
	}

	description := fmt.Sprintf("Smart contract swap on %s then IBC to %s", hopInfo.BrokerChainId, req.ChainTo)
	if len(outboundLegs) > 1 {
		description = fmt.Sprintf("Smart contract swap on %s then IBC forward via %d hops", hopInfo.BrokerChainId, len(outboundLegs))
	}

	return &models.BrokerExecutionData{
		SmartContractData: scData,
		MinOutputAmount:   minOutput,
		UsesWasm:          true,
		Description:       description,
	}, nil
}

// buildOutboundHops converts outbound legs to IBCHop slice for memo building
func (s *Pathfinder) buildOutboundHops(outboundLegs []*models.IBCLeg, finalReceiver string, req models.RouteRequest) []ibcmemo.IBCHop {
	hops := make([]ibcmemo.IBCHop, len(outboundLegs))
	for i, leg := range outboundLegs {
		receiver := finalReceiver
		if i < len(outboundLegs)-1 {
			// Intermediate hop - derive address for next chain
			nextChain := outboundLegs[i+1].FromChain
			nextAddr, addrErr := s.addressConverter.ConvertAddress(req.ReceiverAddress, nextChain)
			if addrErr == nil {
				receiver = nextAddr
			}
		}
		hops[i] = ibcmemo.IBCHop{
			Channel:  leg.Channel,
			Port:     leg.Port,
			Receiver: receiver,
			Timeout:  ibcmemo.DefaultTimeoutTimestamp(),
		}
	}
	return hops
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
