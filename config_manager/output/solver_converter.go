package output

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
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
		solverChain := c.convertChain(chainConfig, reg)
		config.Chains = append(config.Chains, solverChain)
	}

	return config, nil
}

func (c *SolverConverter) convertChain(chain *enriched.ChainConfig, reg *enriched.RegistryConfig) SolverChain {
	solverChain := SolverChain{
		Name:     chain.Name,
		ID:       chain.ID,
		HasPFM:   chain.HasPFM,
		Broker:   chain.IsBroker,
		BrokerID: chain.BrokerID,
		Routes:   make([]SolverRoute, 0, len(chain.Routes)),
	}

	for _, route := range chain.Routes {
		solverRoute := c.convertRoute(route, chain)
		solverChain.Routes = append(solverChain.Routes, solverRoute)
	}

	return solverChain
}

func (c *SolverConverter) convertRoute(route enriched.RouteConfig, sourceChain *enriched.ChainConfig) SolverRoute {
	solverRoute := SolverRoute{
		ToChain:       route.ToChainName,
		ToChainID:     route.ToChainID,
		ConnectionID:  route.ConnectionID,
		ChannelID:     route.ChannelID,
		PortID:        route.PortID,
		AllowedTokens: make(map[string]SolverTokenInfo),
	}

	// Add native tokens that can be sent on this route
	for _, token := range sourceChain.NativeTokens {
		// Check if token is allowed for this destination
		if len(token.AllowedDestinations) > 0 {
			allowed := slices.Contains(token.AllowedDestinations, route.ToChainID)
			if !allowed {
				continue
			}
		}

		// Calculate the IBC denom on the destination chain
		ibcDenom := c.calculateIBCDenom(route.PortID, route.ChannelID, token.Denom)

		solverRoute.AllowedTokens[token.Denom] = SolverTokenInfo{
			ChainDenom:  token.Denom,
			IBCDenom:    ibcDenom,
			BaseDenom:   token.Denom,
			OriginChain: sourceChain.ID,
			Decimals:    token.Decimals,
		}
	}

	// Add IBC tokens that can be forwarded (unwound) on this route
	// An IBC token can be sent to its origin chain or further forwarded
	for _, ibcToken := range sourceChain.IBCTokens {
		// If routing to the token's origin chain, it will unwind to the base denom
		if route.ToChainID == ibcToken.OriginChain {
			solverRoute.AllowedTokens[ibcToken.IBCDenom] = SolverTokenInfo{
				ChainDenom:  ibcToken.IBCDenom,
				IBCDenom:    ibcToken.BaseDenom, // Unwound to native denom
				BaseDenom:   ibcToken.BaseDenom,
				OriginChain: ibcToken.OriginChain,
				Decimals:    ibcToken.Decimals,
			}
		} else {
			// Forwarding to a different chain - token gets wrapped again
			newIBCDenom := c.calculateIBCDenom(route.PortID, route.ChannelID, ibcToken.IBCDenom)
			solverRoute.AllowedTokens[ibcToken.IBCDenom] = SolverTokenInfo{
				ChainDenom:  ibcToken.IBCDenom,
				IBCDenom:    newIBCDenom,
				BaseDenom:   ibcToken.BaseDenom,
				OriginChain: ibcToken.OriginChain,
				Decimals:    ibcToken.Decimals,
			}
		}
	}

	return solverRoute
}

// calculateIBCDenom computes the IBC denom hash for a token arriving via a channel.
// Format: ibc/SHA256(port/channel/denom) in uppercase hex
func (c *SolverConverter) calculateIBCDenom(port, channel, denom string) string {
	// Handle already-IBC denoms (multi-hop)
	path := fmt.Sprintf("%s/%s/%s", port, channel, denom)

	hash := sha256.Sum256([]byte(path))
	hashHex := strings.ToUpper(hex.EncodeToString(hash[:]))

	return fmt.Sprintf("ibc/%s", hashHex)
}
