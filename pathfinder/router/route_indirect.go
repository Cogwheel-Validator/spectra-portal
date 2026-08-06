package router

import (
	"container/list"
	"errors"

	models "github.com/Cogwheel-Validator/spectra-portal/pathfinder/models"
)

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
		current, ok := element.Value.(*pathNode)
		if !ok {
			return nil
		}
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

// buildDirectResponse creates a RouteResponse for a direct IBC transfer
func (s *Pathfinder) buildDirectResponse(req models.RouteRequest, route *BasicRoute) models.RouteResponse {
	needs := directAddressNeeds(req)

	// The transfer itself carries no address-derived data, but strict mode
	// still must reject a request whose address map doesn't cover the
	// chains this route needs - a client shouldn't get a silent "OK" for an
	// incomplete request just because a direct route has nothing to embed.
	if _, err := s.resolveRouteAddresses(req, needs); err != nil {
		var missingErr *MissingAddressesError
		if errors.As(err, &missingErr) {
			return models.RouteResponse{
				Success:              false,
				RouteType:            "impossible",
				ErrorMessage:         missingErr.Error(),
				MissingAddressChains: missingErr.ChainIDs,
			}
		}
		pathfinderLog.Warn().Err(err).Msg("Failed to resolve direct route addresses")
	}

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
		Success:        true,
		RouteType:      "direct",
		Direct:         direct,
		RequiredChains: chainsFromNeeds(needs),
	}
}

// directAddressNeeds lists the addresses a direct route needs: the sender on
// the source chain and the receiver on the destination chain.
func directAddressNeeds(req models.RouteRequest) []AddressNeed {
	return []AddressNeed{
		{ChainID: req.ChainFrom, Role: RoleSender, Required: true},
		{ChainID: req.ChainTo, Role: RoleReceiver, Required: true},
	}
}

// indirectAddressNeeds lists the addresses an indirect (non-broker, PFM)
// route needs: the source, the destination, and every intermediate chain
// the path crosses. Strict (v2beta) mode requires all of them explicitly
// regardless of the Required flag below (see resolveRouteAddresses) - we
// don't rely on PFM intermediate hops silently escrowing in a module
// account, since that path is untested; a real request must supply an
// address for every chain the route touches, same as Skip Go.
// Required stays false here purely so legacy v1/DeriveMissing requests keep
// their old lenient fallback when a slip-44 mismatch makes derivation
// impossible for an intermediate chain.
func indirectAddressNeeds(req models.RouteRequest, path []string) []AddressNeed {
	needs := directAddressNeeds(req)
	if len(path) <= 2 {
		return needs
	}
	for _, chainID := range path[1 : len(path)-1] {
		needs = append(needs, AddressNeed{ChainID: chainID, Role: RoleReceiver, Required: false})
	}
	return needs
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
	needs := indirectAddressNeeds(req, routeInfo.Path)

	if supportsPFM && len(routeInfo.Path) > 2 {
		resolved, err := s.resolveRouteAddresses(req, needs)
		if err != nil {
			var missingErr *MissingAddressesError
			if errors.As(err, &missingErr) {
				return models.RouteResponse{
					Success:              false,
					RouteType:            "impossible",
					ErrorMessage:         missingErr.Error(),
					MissingAddressChains: missingErr.ChainIDs,
				}
			}
			pathfinderLog.Warn().Err(err).Msg("Failed to resolve route addresses, skipping PFM memo")
		} else if !resolved.Placeholders {
			// Mock discovery requests carry no memo; the placeholder receiver
			// must never end up in a signable payload.
			pfmMemo = s.generatePFMMemo(legs, resolved.On(req.ChainTo, RoleReceiver))
		}
	}

	indirect := &models.IndirectRoute{
		Path:          routeInfo.Path,
		Legs:          legs,
		SupportsPFM:   supportsPFM,
		PFMStartChain: req.ChainFrom,
		PFMMemo:       pfmMemo,
	}

	return models.RouteResponse{
		Success:        true,
		RouteType:      "indirect",
		Indirect:       indirect,
		RequiredChains: chainsFromNeeds(needs),
	}
}
