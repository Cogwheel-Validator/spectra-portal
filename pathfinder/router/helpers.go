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

