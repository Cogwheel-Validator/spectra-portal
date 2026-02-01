package router

import (
	"fmt"
)

// NewRouteIndex creates a new RouteIndex with initialized maps
func NewRouteIndex() *RouteIndex {
	return &RouteIndex{
		directRoutes:        make(map[string]*BasicRoute),
		brokerRoutes:        make(map[string]map[string]*BasicRoute),
		chainToBrokerRoutes: make(map[string]map[string]*BasicRoute),
		denomToTokenInfo:    make(map[string]map[string]*TokenInfo),
		brokers:             make(map[string]bool),
		brokerChains:        make(map[string]string),
		pfmChains:           make(map[string]bool),
		chainRoutes:         make(map[string]map[string]*BasicRoute),
	}
}

// BuildIndex builds the route index from the given chains
func (ri *RouteIndex) BuildIndex(chains []PathfinderChain) error {
	if len(chains) == 0 {
		return fmt.Errorf("no chains to build index for")
	}

	// First pass: Register all brokers and PFM chains
	for _, chain := range chains {
		if chain.HasPFM {
			ri.pfmChains[chain.Id] = true
		}

		if chain.Broker {
			ri.brokers[chain.BrokerId] = true
			ri.brokerChains[chain.Id] = chain.BrokerId
		}
	}

	// Second pass: Build all route indices
	for _, chain := range chains {
		if ri.denomToTokenInfo[chain.Id] == nil {
			ri.denomToTokenInfo[chain.Id] = make(map[string]*TokenInfo)
		}

		// Track PFM support
		if chain.HasPFM {
			ri.pfmChains[chain.Id] = true
		}

		// Initialize chain routes map
		if ri.chainRoutes[chain.Id] == nil {
			ri.chainRoutes[chain.Id] = make(map[string]*BasicRoute)
		}

		// Add native tokens to denomToTokenInfo (even if they're not in any routes)
		// This ensures tokens with allowed_destinations=["none"] are still available on their native chain
		for _, nativeToken := range chain.NativeTokens {
			// Only add if not already present (routes take precedence as they have more complete info)
			if _, exists := ri.denomToTokenInfo[chain.Id][nativeToken.ChainDenom]; !exists {
				tokenInfoCopy := nativeToken
				ri.denomToTokenInfo[chain.Id][nativeToken.ChainDenom] = &tokenInfoCopy
			}
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

			// Index all chain-to-chain routes
			if _, exists := ri.chainRoutes[chain.Id][route.ToChainId]; !exists {
				ri.chainRoutes[chain.Id][route.ToChainId] = &route
			}

			// Index broker routes - check if destination chain is a broker
			if brokerId, isBroker := ri.brokerChains[route.ToChainId]; isBroker {
				if ri.chainToBrokerRoutes[chain.Id] == nil {
					ri.chainToBrokerRoutes[chain.Id] = make(map[string]*BasicRoute)
				}
				ri.chainToBrokerRoutes[chain.Id][brokerId] = &route
			}
		}

		// Build broker-specific route maps
		if chain.Broker {
			if ri.brokerRoutes[chain.BrokerId] == nil {
				ri.brokerRoutes[chain.BrokerId] = make(map[string]*BasicRoute)
			}
			for _, route := range chain.Routes {
				if _, exists := ri.brokerRoutes[chain.BrokerId][route.ToChainId]; !exists {
					ri.brokerRoutes[chain.BrokerId][route.ToChainId] = &route
				}
			}
		}
	}

	return nil
}
