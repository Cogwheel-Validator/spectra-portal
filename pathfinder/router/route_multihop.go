package router

import (
	models "github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/models"
)

// FindMultiHopRoute finds routes that go through a broker with token swaps.
//
// In the future there might be more than one broker that can reach the destination chain.
// So we need to return a list of possible multi-hop routes.
//
// Handles these cases:
// - Case 1: Same-chain swap (source == broker == destination) - just swap, no IBC
// - Case 2: Swap-only (source != broker, destination == broker) - inbound IBC + swap
// - Case 3: Source-is-broker (source == broker, destination != broker) - swap + outbound IBC
// - Case 4: Full broker route (source != broker != destination) - inbound IBC + swap + outbound IBC
// - Case 5: Swap and forward (source == broker != destination) - swap + ibc forward
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
		InboundRoutes:    nil, // No inbound route needed (source is broker)
		InboundPath:      nil,
		OutboundRoutes:   nil, // No outbound route needed (dest is broker)
		TokenIn:          tokenIn,
		TokenOut:         tokenOut,
		TokenOutOnBroker: tokenOut, // Same as TokenOut since staying on broker
		SwapOnly:         true,     // Technically just a swap
		SourceIsBroker:   true,     // New flag to distinguish from regular swap-only
	}
}

// findSwapOnlyRoute finds a route where the destination is the broker (IBC transfer + swap, no outbound)
// Supports both single-hop and multi-hop inbound paths
func (ri *RouteIndex) findSwapOnlyRoute(req models.RouteRequest, brokerId, brokerChainId string) *MultiHopInfo {
	// Is output token available on the broker chain?
	tokenOut := ri.denomToTokenInfo[brokerChainId][req.TokenToDenom]
	if tokenOut == nil {
		pathfinderLog.Debug().
			Str("tokenToDenom", req.TokenToDenom).
			Str("brokerChainId", brokerChainId).
			Msg("Output token not found on broker chain")
		return nil
	}

	// Try single-hop inbound first (source -> broker)
	inboundRoute := ri.chainToBrokerRoutes[req.ChainFrom][brokerId]
	if inboundRoute != nil {
		// Is input token allowed on inbound route?
		tokenIn, tokenAllowed := inboundRoute.AllowedTokens[req.TokenFromDenom]
		if tokenAllowed {
			pathfinderLog.Debug().
				Str("tokenIn", tokenIn.ChainDenom).
				Str("tokenOut", tokenOut.ChainDenom).
				Msg("Swap-only route validated (single hop inbound)")

			return &MultiHopInfo{
				BrokerChain:      brokerId,
				BrokerChainId:    brokerChainId,
				InboundRoutes:    []*BasicRoute{inboundRoute},
				InboundPath:      []string{req.ChainFrom},
				OutboundRoutes:   nil,
				TokenIn:          &tokenIn,
				TokenOut:         tokenOut,
				TokenOutOnBroker: tokenOut,
				SwapOnly:         true,
			}
		}
	}

	// Try multi-hop inbound (source -> intermediate -> broker)
	multiHopInbound := ri.findMultiHopInboundRoute(req.ChainFrom, brokerChainId, req.TokenFromDenom)
	if multiHopInbound != nil {
		pathfinderLog.Debug().
			Str("tokenIn", multiHopInbound.TokenIn.ChainDenom).
			Str("tokenOut", tokenOut.ChainDenom).
			Int("inboundHops", len(multiHopInbound.Routes)).
			Msg("Swap-only route validated (multi-hop inbound)")

		return &MultiHopInfo{
			BrokerChain:               brokerId,
			BrokerChainId:             brokerChainId,
			InboundRoutes:             multiHopInbound.Routes,
			InboundPath:               multiHopInbound.Path,
			InboundIntermediateTokens: multiHopInbound.IntermediateTokens,
			OutboundRoutes:            nil,
			TokenIn:                   multiHopInbound.TokenIn,
			TokenOut:                  tokenOut,
			TokenOutOnBroker:          tokenOut,
			SwapOnly:                  true,
		}
	}

	pathfinderLog.Debug().Str("chainFrom", req.ChainFrom).Str("brokerId", brokerId).Msg("No inbound route to broker")
	return nil
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
		InboundRoutes:    nil, // No inbound route needed (source is broker)
		InboundPath:      nil,
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
// Supports multi-hop inbound (source → intermediate → broker)
func (ri *RouteIndex) findFullBrokerRoute(req models.RouteRequest, brokerId, brokerChainId string) *MultiHopInfo {
	// Is output token available on destination?
	tokenOut := ri.denomToTokenInfo[req.ChainTo][req.TokenToDenom]
	if tokenOut == nil {
		pathfinderLog.Debug().Str("tokenToDenom", req.TokenToDenom).Str("chainTo", req.ChainTo).Msg("Output token not found on destination")
		return nil
	}

	// Try to find inbound route (single or multi-hop)
	var inboundRoutes []*BasicRoute
	var inboundPath []string
	var tokenIn *TokenInfo
	var inboundIntermediateTokens []*TokenInfo

	// Try single-hop inbound first
	inboundRoute := ri.chainToBrokerRoutes[req.ChainFrom][brokerId]
	if inboundRoute != nil {
		tokenInInfo, tokenAllowed := inboundRoute.AllowedTokens[req.TokenFromDenom]
		if tokenAllowed {
			inboundRoutes = []*BasicRoute{inboundRoute}
			inboundPath = []string{req.ChainFrom}
			tokenIn = &tokenInInfo
		}
	}

	// If single hop didn't work, try multi-hop
	if inboundRoutes == nil {
		multiHopInbound := ri.findMultiHopInboundRoute(req.ChainFrom, brokerChainId, req.TokenFromDenom)
		if multiHopInbound != nil {
			inboundRoutes = multiHopInbound.Routes
			inboundPath = multiHopInbound.Path
			tokenIn = multiHopInbound.TokenIn
			inboundIntermediateTokens = multiHopInbound.IntermediateTokens
		}
	}

	if inboundRoutes == nil {
		pathfinderLog.Debug().Str("chainFrom", req.ChainFrom).Str("brokerId", brokerId).Msg("No inbound route to broker (single or multi-hop)")
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
					Int("inboundHops", len(inboundRoutes)).
					Msg("Full broker route validated (direct outbound)")

				return &MultiHopInfo{
					BrokerChain:               brokerId,
					BrokerChainId:             brokerChainId,
					InboundRoutes:             inboundRoutes,
					InboundPath:               inboundPath,
					InboundIntermediateTokens: inboundIntermediateTokens,
					OutboundRoutes:            []*BasicRoute{directOutbound},
					TokenIn:                   tokenIn,
					TokenOut:                  tokenOut,
					TokenOutOnBroker:          &tokenInfo,
					SwapOnly:                  false,
				}
			}
		}
	}

	// No direct route works - check if we need 4-chain outbound route
	// This happens when the token's origin is different from both broker and destination
	// e.g., USDC from Noble: broker=osmosis, origin=noble, dest=juno
	// Route: source -> broker -> noble (unwind) -> juno (forward)
	originChain := tokenOut.OriginChain
	if originChain != brokerChainId && originChain != req.ChainTo {
		pathfinderLog.Debug().
			Str("originChain", originChain).
			Str("brokerChain", brokerChainId).
			Str("destChain", req.ChainTo).
			Msg("Checking 4-chain outbound route through origin")

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
			Int("inboundHops", len(inboundRoutes)).
			Msg("Multi-chain broker route validated")

		return &MultiHopInfo{
			BrokerChain:               brokerId,
			BrokerChainId:             brokerChainId,
			InboundRoutes:             inboundRoutes,
			InboundPath:               inboundPath,
			InboundIntermediateTokens: inboundIntermediateTokens,
			OutboundRoutes:            []*BasicRoute{brokerToOrigin, originToDest},
			TokenIn:                   tokenIn,
			TokenOut:                  tokenOut,
			TokenOutOnBroker:          tokenOnBroker,
			IntermediateTokens:        []*TokenInfo{tokenOnOrigin},
			SwapOnly:                  false,
		}
	}

	pathfinderLog.Debug().Str("tokenToDenom", req.TokenToDenom).Msg("No valid broker route found")
	return nil
}

// findMultiHopInboundRoute tries to find a 2-hop path from source to broker: source -> intermediate -> broker
// This is useful when source doesn't have a direct channel to broker but can reach it through another chain
func (ri *RouteIndex) findMultiHopInboundRoute(sourceChain, brokerChainId, tokenDenom string) *MultiHopInboundResult {
	// Find all chains that can reach the broker
	brokerId := ri.brokerChains[brokerChainId]
	if brokerId == "" {
		return nil
	}

	// For each chain that has a route to the broker, check if source can reach that chain
	for intermediateChain, intermediateToBroker := range ri.chainToBrokerRoutes {
		brokerRoute := intermediateToBroker[brokerId]
		if brokerRoute == nil {
			continue
		}

		// Skip if intermediate is source or broker
		if intermediateChain == sourceChain || intermediateChain == brokerChainId {
			continue
		}

		// Check if source can reach the intermediate chain
		sourceToIntermediate := ri.findRouteFromChain(sourceChain, intermediateChain)
		if sourceToIntermediate == nil {
			continue
		}

		// Check if the token can travel this path
		// First hop: source -> intermediate
		tokenOnSource, tokenAllowedFirst := sourceToIntermediate.AllowedTokens[tokenDenom]
		if !tokenAllowedFirst {
			continue
		}

		// The token on intermediate chain is the IBC denom
		tokenDenomOnIntermediate := tokenOnSource.IbcDenom

		// Second hop: intermediate -> broker
		tokenOnIntermediate, tokenAllowedSecond := brokerRoute.AllowedTokens[tokenDenomOnIntermediate]
		if !tokenAllowedSecond {
			continue
		}

		pathfinderLog.Debug().
			Str("source", sourceChain).
			Str("intermediate", intermediateChain).
			Str("broker", brokerChainId).
			Str("tokenOnSource", tokenOnSource.ChainDenom).
			Str("tokenOnIntermediate", tokenOnIntermediate.ChainDenom).
			Msg("Found multi-hop inbound route")

		return &MultiHopInboundResult{
			Routes:             []*BasicRoute{sourceToIntermediate, brokerRoute},
			Path:               []string{sourceChain, intermediateChain},
			TokenIn:            &tokenOnSource,
			IntermediateTokens: []*TokenInfo{&tokenOnIntermediate},
		}
	}

	return nil
}

