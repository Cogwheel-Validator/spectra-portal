package router

import (
	"fmt"

	models "github.com/Cogwheel-Validator/spectra-portal/pathfinder/models"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/brokers"
)

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

	tokenInDenomOnBroker := s.brokerSwapInputDenom(hopInfo)
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

// brokerSwapInputDenom determines the denom to quote against on the broker chain.
// Osmosis SQS expects broker-chain denoms, so the denom depends on how the token
// reaches the broker.
func (s *Pathfinder) brokerSwapInputDenom(hopInfo *MultiHopInfo) string {
	switch {
	case hopInfo.SourceIsBroker:
		// Source is the broker - token is already on broker, use ChainDenom directly
		return hopInfo.TokenIn.ChainDenom
	case len(hopInfo.InboundIntermediateTokens) > 0:
		// Multi-hop inbound (e.g. Cosmos Hub → Noble → Osmosis): token arrives at broker via last hop.
		// TokenIn.IbcDenom is the denom on the first hop's destination (e.g. uusdc on Noble), not on the broker.
		// Use the last intermediate token's IbcDenom, which is the denom when the token lands on the broker.
		lastIntToken := hopInfo.InboundIntermediateTokens[len(hopInfo.InboundIntermediateTokens)-1]
		return lastIntToken.IbcDenom
	default:
		// Single-hop inbound - token goes source → broker, use TokenIn.IbcDenom (denom on broker)
		return hopInfo.TokenIn.IbcDenom
	}
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
	if err := validateBrokerRouteInputs(hopInfo, swapResult); err != nil {
		return nil, err
	}
	if err := normalizeSlippage(&req); err != nil {
		return nil, err
	}

	_, brokerExists := s.chainsMap[hopInfo.BrokerChainId]

	path := s.buildBrokerPath(req, hopInfo)
	inboundLegs, tokenInOnBroker := s.buildBrokerInboundLegs(req, hopInfo)

	// Token out on broker (after swap)
	tokenOutOnBroker := &models.TokenMapping{
		ChainDenom:  hopInfo.TokenOutOnBroker.ChainDenom,
		BaseDenom:   hopInfo.TokenOutOnBroker.BaseDenom,
		OriginChain: hopInfo.TokenOutOnBroker.OriginChain,
		IsNative:    s.denomResolver.IsTokenNativeToChain(hopInfo.TokenOutOnBroker, hopInfo.BrokerChainId),
	}

	swap := &models.SwapQuote{
		Broker:       brokerClient.GetBrokerType(),
		TokenIn:      tokenInOnBroker,
		TokenOut:     tokenOutOnBroker,
		AmountIn:     swapResult.AmountIn,
		AmountOut:    swapResult.AmountOut,
		PriceImpact:  swapResult.PriceImpact,
		EffectiveFee: swapResult.EffectiveFee,
		RouteData:    swapResult.RouteData,
	}

	outboundLegs, supportsPFM := s.buildBrokerOutboundLegs(hopInfo, swapResult, tokenOutOnBroker)

	execution := s.buildBrokerExecutionData(req, hopInfo, swapResult, outboundLegs, brokerClient, brokerExists)

	return &models.BrokerRoute{
		Path:                path,
		InboundLegs:         inboundLegs,
		Swap:                swap,
		OutboundLegs:        outboundLegs,
		OutboundSupportsPFM: supportsPFM,
		Execution:           execution,
	}, nil
}

// validateBrokerRouteInputs checks that the required hop and swap data are present.
func validateBrokerRouteInputs(hopInfo *MultiHopInfo, swapResult *brokers.SwapResult) error {
	if hopInfo == nil {
		return fmt.Errorf("hopInfo is nil")
	}
	if hopInfo.TokenIn == nil {
		return fmt.Errorf("hopInfo.TokenIn is nil")
	}
	if hopInfo.TokenOutOnBroker == nil {
		return fmt.Errorf("hopInfo.TokenOutOnBroker is nil")
	}
	if swapResult == nil {
		return fmt.Errorf("swapResult is nil")
	}
	return nil
}

// normalizeSlippage applies a default slippage when none is provided and validates the bound.
func normalizeSlippage(req *models.RouteRequest) error {
	// Leave it like this only for the tests... The proto will ALWAYS provide value
	// TODO: Refactor this in the future, it is not needed for program to function but tests rely on it
	if req.SlippageBps == nil {
		defaultSlippage := uint32(100) // 1% default slippage
		req.SlippageBps = &defaultSlippage
	}

	if *req.SlippageBps > 10000 {
		return fmt.Errorf("slippage bps must be less than 10000")
	}
	return nil
}

// buildBrokerPath assembles the ordered chain path for the route based on whether
// the source/destination is the broker and whether there are multi-hop inbound routes.
func (s *Pathfinder) buildBrokerPath(req models.RouteRequest, hopInfo *MultiHopInfo) []string {
	switch {
	case hopInfo.SourceIsBroker && hopInfo.SwapOnly:
		// Same-chain swap
		return []string{hopInfo.BrokerChainId}
	case hopInfo.SourceIsBroker:
		// Source is broker, has outbound
		return []string{hopInfo.BrokerChainId, req.ChainTo}
	case hopInfo.SwapOnly:
		// Dest is broker, has inbound (possibly multi-hop)
		path := append([]string{}, hopInfo.InboundPath...)
		return append(path, hopInfo.BrokerChainId)
	default:
		// Full route (possibly with multi-hop inbound)
		path := append([]string{}, hopInfo.InboundPath...)
		return append(path, hopInfo.BrokerChainId, req.ChainTo)
	}
}

// buildBrokerInboundLegs builds the inbound IBC legs and the mapping of the inbound
// token as it lands on the broker chain. Returns nil legs when the source is the broker.
func (s *Pathfinder) buildBrokerInboundLegs(
	req models.RouteRequest,
	hopInfo *MultiHopInfo,
) ([]*models.IBCLeg, *models.TokenMapping) {
	if hopInfo.SourceIsBroker {
		// Source is broker - token is already on broker chain
		tokenInOnBroker := &models.TokenMapping{
			ChainDenom:  hopInfo.TokenIn.ChainDenom,
			BaseDenom:   hopInfo.TokenIn.BaseDenom,
			OriginChain: hopInfo.TokenIn.OriginChain,
			IsNative:    s.denomResolver.IsTokenNativeToChain(hopInfo.TokenIn, hopInfo.BrokerChainId),
		}
		return nil, tokenInOnBroker
	}

	if len(hopInfo.InboundRoutes) == 0 {
		return nil, nil
	}

	// Build inbound legs - can be single or multi-hop
	inboundLegs := make([]*models.IBCLeg, 0, len(hopInfo.InboundRoutes))

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
	var tokenInOnBroker *models.TokenMapping
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

	return inboundLegs, tokenInOnBroker
}

// buildBrokerOutboundLegs builds the outbound IBC legs from the broker chain to the
// destination and reports whether all intermediate outbound chains support PFM.
// Returns nil legs when the destination is the broker (swap-only).
func (s *Pathfinder) buildBrokerOutboundLegs(
	hopInfo *MultiHopInfo,
	swapResult *brokers.SwapResult,
	tokenOutOnBroker *models.TokenMapping,
) ([]*models.IBCLeg, bool) {
	if hopInfo.SwapOnly || len(hopInfo.OutboundRoutes) == 0 {
		return nil, false
	}

	var outboundLegs []*models.IBCLeg

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
	supportsPFM := true
	for i := 0; i < len(hopInfo.OutboundRoutes)-1; i++ {
		if !s.routeIndex.pfmChains[hopInfo.OutboundRoutes[i].ToChainId] {
			supportsPFM = false
			break
		}
	}

	return outboundLegs, supportsPFM
}

// buildBrokerExecutionData generates execution data for the route when SmartRoute is
// enabled, dispatching to the correct builder for the route shape. Execution data is
// best-effort: builder failures are logged and result in nil execution, leaving the
// route still usable as a manual route.
func (s *Pathfinder) buildBrokerExecutionData(
	req models.RouteRequest,
	hopInfo *MultiHopInfo,
	swapResult *brokers.SwapResult,
	outboundLegs []*models.IBCLeg,
	brokerClient brokers.BrokerClient,
	brokerExists bool,
) *models.BrokerExecutionData {
	// Only build execution data if SmartRoute is explicitly true
	if req.SmartRoute == nil || !*req.SmartRoute {
		pathfinderLog.Debug().Msg("Manual route - skipping execution data generation")
		return nil
	}

	memoBuilder := brokerClient.GetMemoBuilder()
	smartContractBuilder := brokerClient.GetSmartContractBuilder()

	var execution *models.BrokerExecutionData
	var err error

	switch {
	case hopInfo.SourceIsBroker && hopInfo.SwapOnly:
		// Same-chain swap: Source == Broker == Destination
		// Use smart contract data (direct contract call, no IBC)
		pathfinderLog.Debug().Msg("Building same-chain swap route (smart contract)")
		execution, err = s.buildSmartContractSwapExecution(req, hopInfo, swapResult, smartContractBuilder, brokerExists)
	case hopInfo.SourceIsBroker:
		// Source is broker, dest is not: swap + outbound IBC
		// Use smart contract data with IBC forward built-in
		pathfinderLog.Debug().Msg("Building broker-as-source route (smart contract + IBC forward)")
		execution, err = s.buildSmartContractSwapAndForwardExecution(req, hopInfo, swapResult, outboundLegs, smartContractBuilder, brokerExists)
	case hopInfo.SwapOnly:
		// Source is not broker, dest is broker: inbound IBC + swap
		// Use IBC memo (ibc-hooks will trigger swap)
		pathfinderLog.Debug().Msg("Building swap-only route (IBC memo)")
		execution, err = s.buildSwapOnlyExecution(req, hopInfo, swapResult, memoBuilder, brokerExists)
	default:
		// Full route: source -> broker -> dest (all different chains)
		// Use IBC memo (ibc-hooks will trigger swap + forward)
		pathfinderLog.Debug().Int("outboundHops", len(outboundLegs)).Msg("Building full broker route (IBC memo)")
		execution, err = s.buildSwapAndForwardExecution(req, hopInfo, swapResult, outboundLegs, memoBuilder, brokerExists)
	}

	if err != nil {
		pathfinderLog.Warn().Err(err).Msg("Failed to build execution data, route still usable")
	}

	return execution
}
