package router

import (
	"fmt"

	models "github.com/Cogwheel-Validator/spectra-ibc-hub/solver/models"
)

// DenomResolver helps resolve token denoms across chains and tracks token origins
type DenomResolver struct {
	routeIndex *RouteIndex
}

// NewDenomResolver creates a new denom resolver
func NewDenomResolver(routeIndex *RouteIndex) *DenomResolver {
	return &DenomResolver{
		routeIndex: routeIndex,
	}
}

// ResolveDenom resolves a denom on a specific chain and returns detailed information
func (dr *DenomResolver) ResolveDenom(chainID, denom string) (*models.DenomInfo, error) {
	// Check if we have token info for this denom on this chain
	var ibcPath string
	if chainTokens, exists := dr.routeIndex.denomToTokenInfo[chainID]; exists {
		chainRoutes := dr.routeIndex.chainRoutes[chainID]
		for _, route := range chainRoutes {
			if _, allowed := route.AllowedTokens[denom]; allowed {
				ibcPath = route.PortId + "/" + route.ChannelId
				break
			}
		}
		if tokenInfo, found := chainTokens[denom]; found {
			return &models.DenomInfo{
				ChainDenom:  tokenInfo.ChainDenom,
				BaseDenom:   tokenInfo.BaseDenom,
				OriginChain: tokenInfo.OriginChain,
				IsNative:    tokenInfo.OriginChain == chainID,
				IbcPath:     ibcPath,
			}, nil
		}
	}

	return nil, fmt.Errorf("denom %s not found on chain %s", denom, chainID)
}

// CreateTokenMapping creates a TokenMapping for API responses
func (dr *DenomResolver) CreateTokenMapping(chainID, denom string) (*models.TokenMapping, error) {
	denomInfo, err := dr.ResolveDenom(chainID, denom)
	if err != nil {
		// If not found in index, assume it's a native token
		return &models.TokenMapping{
			ChainDenom:  denom,
			BaseDenom:   denom,
			OriginChain: chainID,
			IsNative:    true,
		}, nil
	}

	return &models.TokenMapping{
		ChainDenom:  denomInfo.ChainDenom,
		BaseDenom:   denomInfo.BaseDenom,
		OriginChain: denomInfo.OriginChain,
		IsNative:    denomInfo.IsNative,
	}, nil
}

// GetDenomOnChain returns the correct denom to use on a specific chain
// This handles the case where a token returns to its origin chain
func (dr *DenomResolver) GetDenomOnChain(tokenInfo *TokenInfo, targetChainID string) string {
	// If the token is returning to its origin chain, use the base denom
	if tokenInfo.OriginChain == targetChainID {
		return tokenInfo.BaseDenom
	}

	// Otherwise, use the IBC denom for that chain
	return tokenInfo.IbcDenom
}

// IsTokenNativeToChain checks if a token is native to a specific chain
func (dr *DenomResolver) IsTokenNativeToChain(tokenInfo *TokenInfo, chainID string) bool {
	return tokenInfo.OriginChain == chainID
}
