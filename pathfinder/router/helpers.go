package router

import (
	"fmt"
)

// routeKey is a helper function to create a unique key for a route
func routeKey(fromChain, toChain, denom string) string {
	return fmt.Sprintf("%s->%s:%s", fromChain, toChain, denom)
}

// findRouteFromChain finds a route from a chain to another chain
func (ri *RouteIndex) findRouteFromChain(fromChain, toChain string) *BasicRoute {
	routes := ri.chainRoutes[fromChain]
	for _, route := range routes {
		if route.ToChainId == toChain {
			return route
		}
	}
	return nil
}

// TokenInfoForRoute returns the token info for denom as it travels the
// specific fromChain->toChain route.
func (ri *RouteIndex) TokenInfoForRoute(fromChain, toChain, denom string) (*TokenInfo, bool) {
	route, ok := ri.chainRoutes[fromChain][toChain]
	if !ok {
		return nil, false
	}
	tokenInfo, ok := route.AllowedTokens[denom]
	if !ok {
		return nil, false
	}
	return &tokenInfo, true
}
