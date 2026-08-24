package router

import (
	"errors"

	models "github.com/Cogwheel-Validator/spectra-portal/pathfinder/models"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/routeindex"
)

// buildDirectResponse creates a RouteResponse for a direct IBC transfer
func (s *Pathfinder) buildDirectResponse(req models.RouteRequest, route *routeindex.BasicRoute) models.RouteResponse {
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
func (s *Pathfinder) buildIndirectResponse(req models.RouteRequest, routeInfo *routeindex.IndirectRouteInfo) models.RouteResponse {
	// Build IBC legs for each hop
	legs := []*models.IBCLeg{}
	currentDenom := req.TokenFromDenom
	amount := req.AmountIn

	for i, route := range routeInfo.Routes {
		fromChain := routeInfo.Path[i]
		toChain := routeInfo.Path[i+1]

		// Get token info on the current chain
		var tokenInfo *routeindex.TokenInfo
		if i == 0 {
			tokenInfo = routeInfo.Token
		} else {
			tokenInfo = s.routeIndex.FindTokenByOrigin(fromChain, routeInfo.Token.OriginChain, routeInfo.Token.BaseDenom)
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
			tokenInfo = &routeindex.TokenInfo{
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
