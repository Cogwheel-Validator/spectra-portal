package router

import (
	models "github.com/Cogwheel-Validator/spectra-portal/pathfinder/models"
)

// FindDirectRoute finds a direct route between two chains for a specific token
func (ri *RouteIndex) FindDirectRoute(req models.RouteRequest) *BasicRoute {
	// Check if same token can go directly
	key := routeKey(req.ChainFrom, req.ChainTo, req.TokenFromDenom)
	if route, exists := ri.directRoutes[key]; exists {
		// Verify output denom matches (same token on both chains).
		tokenInfo, ok := ri.TokenInfoForRoute(req.ChainFrom, req.ChainTo, req.TokenFromDenom)
		if ok && tokenInfo.IbcDenom == req.TokenToDenom {
			return route
		}
	}
	return nil
}
