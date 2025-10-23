package router

import "fmt"

type SmartChain struct {
	Name string
	Id string
	Routes []BasicRoute
}

type BasicRoute struct {
	ToChain string
	ToChainId string
	ConnectionId string
	ChannelId string
	PortId string
	AllowedTokens map[string]TokenInfo
}


// TokenInfo contains both chain denom and IBC denom
type TokenInfo struct {
	ChainDenom string // Native denom on source chain (e.g., "ujuno")
	IbcDenom   string // IBC denom on destination (e.g., "ibc/ABC123...")
	Decimals   int
}

// RouteIndex with denom mapping should be internal logic for the router
type RouteIndex struct {
	directRoutes      map[string]*BasicRoute              // "chainA->chainB->denom"
	chainToBroker     map[string]map[string]*BasicRoute   // chainId -> brokerId -> route
	// brokerId -> chainId -> route this is mostly osmosis but it could be oasis now also
	// while this should relly on the Osmosis SQS, it should be lenient enought 
	// to accept other brokers, for now we will just use the Osmosis SQS and the oasis SQS
	brokerToChain     map[string]map[string]*BasicRoute   
	denomToTokenInfo  map[string]map[string]*TokenInfo    // chainId -> denom -> TokenInfo
	brokers           map[string]bool
}


func (ri *RouteIndex) BuildIndex(chains []SmartChain) {
	for _, chain := range chains {
		if ri.denomToTokenInfo[chain.Id] == nil {
			ri.denomToTokenInfo[chain.Id] = make(map[string]*TokenInfo)
		}
		
		for _, route := range chain.Routes {
			// Index token info
			for denom, tokenInfo := range route.AllowedTokens {
				ri.denomToTokenInfo[chain.Id][denom] = &tokenInfo
			}
			
			// Index routes to brokers
			if ri.brokers[route.ToChainId] {
				if ri.chainToBroker[chain.Id] == nil {
					ri.chainToBroker[chain.Id] = make(map[string]*BasicRoute)
				}
				ri.chainToBroker[chain.Id][route.ToChainId] = &route
			}
			
			// Index routes from brokers
			if ri.brokers[chain.Id] {
				if ri.brokerToChain[chain.Id] == nil {
					ri.brokerToChain[chain.Id] = make(map[string]*BasicRoute)
				}
				ri.brokerToChain[chain.Id][route.ToChainId] = &route
			}
			
			// Index direct routes
			for denom := range route.AllowedTokens {
				key := routeKey(chain.Id, route.ToChainId, denom)
				ri.directRoutes[key] = &route
			}
		}
	}
}

func (ri *RouteIndex) findDirectRoute(req RouteRequest) *BasicRoute {
	// Check if same token can go directly
	key := routeKey(req.ChainA, req.ChainB, req.TokenInDenom)
	if route, exists := ri.directRoutes[key]; exists {
		// Verify output denom matches (same token on both chains)
		tokenInfo := ri.denomToTokenInfo[req.ChainA][req.TokenInDenom]
		if tokenInfo != nil && tokenInfo.IbcDenom == req.TokenOutDenom {
			return route
		}
	}
	return nil
}

type multiHopInfo struct {
	brokerChain   string
	inboundRoute  *BasicRoute
	outboundRoute *BasicRoute
	tokenIn       *TokenInfo
	tokenOut      *TokenInfo
}

func (ri *RouteIndex) findMultiHopRoute(req RouteRequest) *multiHopInfo {
	// Check if we can route through a broker with token swap
	for brokerId := range ri.brokers {
		// Can we reach broker from source?
		inboundRoute := ri.chainToBroker[req.ChainA][brokerId]
		if inboundRoute == nil {
			continue
		}
		
		// Is input token allowed?
		tokenIn, tokenAllowed := inboundRoute.AllowedTokens[req.TokenInDenom]
		if !tokenAllowed {
			continue
		}
		
		// Can broker reach destination?
		outboundRoute := ri.brokerToChain[brokerId][req.ChainB]
		if outboundRoute == nil {
			continue
		}
		
		// Is output token available on destination?
		tokenOut := ri.denomToTokenInfo[req.ChainB][req.TokenOutDenom]
		if tokenOut == nil {
			continue
		}
		
		// Check if output token is allowed on outbound route
		if _, allowed := outboundRoute.AllowedTokens[req.TokenOutDenom]; !allowed {
			continue
		}
		
		return &multiHopInfo{
			brokerChain:   brokerId,
			inboundRoute:  inboundRoute,
			outboundRoute: outboundRoute,
			tokenIn:       &tokenIn,
			tokenOut:      tokenOut,
		}
	}
	
	return nil
}

func (ri *RouteIndex) buildDirectIBCExecution(req RouteRequest, route *BasicRoute) *DirectIBCExecution {
	return &DirectIBCExecution{
		SourceChannel:      route.ChannelId,
		SourcePort:         route.PortId,
		DestinationChannel: "", // Would need reverse lookup or store this
		TokenDenom:         req.TokenInDenom,
		Amount:             req.AmountIn,
		Receiver:           req.ReceiverAddress,
		TimeoutTimestamp:   uint64(600), // 10 minutes
		Memo:               "",
	}
}

func routeKey(fromChain, toChain, denom string) string {
	return fmt.Sprintf("%s->%s:%s", fromChain, toChain, denom)
}
