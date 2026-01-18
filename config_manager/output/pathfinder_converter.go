package output

import (
	"fmt"
	"time"

	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/enriched"
)

// PathfinderConverter converts enriched configs to pathfinder-compatible format.
type PathfinderConverter struct{}

// NewPathfinderConverter creates a new pathfinder converter.
func NewPathfinderConverter() *PathfinderConverter {
	return &PathfinderConverter{}
}

// Convert transforms an enriched registry config into a pathfinder config.
func (c *PathfinderConverter) Convert(reg *enriched.RegistryConfig) (*PathfinderConfig, error) {
	if reg == nil || len(reg.Chains) == 0 {
		return nil, fmt.Errorf("no chains to convert")
	}

	config := &PathfinderConfig{
		Version:     reg.Version,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Chains:      make([]PathfinderChain, 0, len(reg.Chains)),
	}

	for _, chainConfig := range reg.Chains {
		pathfinderChain := c.convertChain(chainConfig)
		config.Chains = append(config.Chains, pathfinderChain)
	}

	return config, nil
}

func (c *PathfinderConverter) convertChain(chain *enriched.ChainConfig) PathfinderChain {
	pathfinderChain := PathfinderChain{
		Name:             chain.Name,
		ID:               chain.ID,
		HasPFM:           chain.HasPFM,
		Broker:           chain.IsBroker,
		BrokerID:         chain.BrokerID,
		IBCHooksContract: chain.IBCHooksContract,
		Bech32Prefix:     chain.Bech32Prefix,
		NativeTokens:     make([]PathfinderTokenInfo, 0, len(chain.NativeTokens)),
		Routes:           make([]PathfinderRoute, 0, len(chain.Routes)),
	}

	// Add all native tokens to the chain (including those with no allowed destinations)
	for _, token := range chain.NativeTokens {
		pathfinderChain.NativeTokens = append(pathfinderChain.NativeTokens, PathfinderTokenInfo{
			ChainDenom:  token.Denom,
			IBCDenom:    "", // For this we do not need to compute the IBC denom because it is not used in this form
			BaseDenom:   token.Denom,
			OriginChain: chain.ID,
			Symbol:      token.Symbol,
			Decimals:    token.Decimals,
		})
	}

	for _, route := range chain.Routes {
		pathfinderRoute := c.convertRoute(route)
		pathfinderChain.Routes = append(pathfinderChain.Routes, pathfinderRoute)
	}

	return pathfinderChain
}

func (c *PathfinderConverter) convertRoute(route enriched.RouteConfig) PathfinderRoute {
	pathfinderRoute := PathfinderRoute{
		ToChain:       route.ToChainName,
		ToChainID:     route.ToChainID,
		ConnectionID:  route.ConnectionID,
		ChannelID:     route.ChannelID,
		PortID:        route.PortID,
		AllowedTokens: make(map[string]PathfinderTokenInfo),
	}

	// Use the pre-computed AllowedTokens from the route builder
	// These are already correctly filtered to only include:
	// 1. Native tokens from the source chain
	// 2. Tokens originating from the destination (unwinding)
	// 3. Routable IBC tokens explicitly configured for this destination
	for _, token := range route.AllowedTokens {
		pathfinderRoute.AllowedTokens[token.SourceDenom] = PathfinderTokenInfo{
			ChainDenom:  token.SourceDenom,
			IBCDenom:    token.DestinationDenom,
			BaseDenom:   token.BaseDenom,
			OriginChain: token.OriginChain,
			Symbol:      token.Symbol,
			Decimals:    token.Decimals,
		}
	}

	return pathfinderRoute
}
