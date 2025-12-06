package output

import (
	"fmt"
	"time"

	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/enriched"
)

// SolverConverter converts enriched configs to solver-compatible format.
type SolverConverter struct{}

// NewSolverConverter creates a new solver converter.
func NewSolverConverter() *SolverConverter {
	return &SolverConverter{}
}

// Convert transforms an enriched registry config into a solver config.
func (c *SolverConverter) Convert(reg *enriched.RegistryConfig) (*SolverConfig, error) {
	if reg == nil || len(reg.Chains) == 0 {
		return nil, fmt.Errorf("no chains to convert")
	}

	config := &SolverConfig{
		Version:     reg.Version,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Chains:      make([]SolverChain, 0, len(reg.Chains)),
	}

	for _, chainConfig := range reg.Chains {
		solverChain := c.convertChain(chainConfig)
		config.Chains = append(config.Chains, solverChain)
	}

	return config, nil
}

func (c *SolverConverter) convertChain(chain *enriched.ChainConfig) SolverChain {
	solverChain := SolverChain{
		Name:             chain.Name,
		ID:               chain.ID,
		HasPFM:           chain.HasPFM,
		Broker:           chain.IsBroker,
		BrokerID:         chain.BrokerID,
		IBCHooksContract: chain.IBCHooksContract,
		Bech32Prefix:     chain.Bech32Prefix,
		Routes:           make([]SolverRoute, 0, len(chain.Routes)),
	}

	for _, route := range chain.Routes {
		solverRoute := c.convertRoute(route)
		solverChain.Routes = append(solverChain.Routes, solverRoute)
	}

	return solverChain
}

func (c *SolverConverter) convertRoute(route enriched.RouteConfig) SolverRoute {
	solverRoute := SolverRoute{
		ToChain:       route.ToChainName,
		ToChainID:     route.ToChainID,
		ConnectionID:  route.ConnectionID,
		ChannelID:     route.ChannelID,
		PortID:        route.PortID,
		AllowedTokens: make(map[string]SolverTokenInfo),
	}

	// Use the pre-computed AllowedTokens from the route builder
	// These are already correctly filtered to only include:
	// 1. Native tokens from the source chain
	// 2. Tokens originating from the destination (unwinding)
	// 3. Routable IBC tokens explicitly configured for this destination
	for _, token := range route.AllowedTokens {
		solverRoute.AllowedTokens[token.SourceDenom] = SolverTokenInfo{
			ChainDenom:  token.SourceDenom,
			IBCDenom:    token.DestinationDenom,
			BaseDenom:   token.BaseDenom,
			OriginChain: token.OriginChain,
			Symbol:      token.Symbol,
			Decimals:    token.Decimals,
		}
	}

	return solverRoute
}
