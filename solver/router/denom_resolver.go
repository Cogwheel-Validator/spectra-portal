package router

import (
	"fmt"
	"strings"

	models "github.com/Cogwheel-Validator/spectra-ibc-hub/solver/models"
)

// DenomResolver helps resolve token denoms across chains and tracks token origins.
// It supports:
// - Human-readable denom resolution (e.g., "uatone" → IBC denom)
// - Cross-chain denom lookup
// - Token availability discovery
type DenomResolver struct {
	routeIndex *RouteIndex
	chains     map[string]SolverChain

	// Lookup maps for efficient resolution
	baseDenomToChains map[string]map[string]*TokenInfo // baseDenom:originChain -> chainID -> TokenInfo
}

// NewDenomResolver creates a new denom resolver
func NewDenomResolver(routeIndex *RouteIndex) *DenomResolver {
	dr := &DenomResolver{
		routeIndex:        routeIndex,
		chains:            make(map[string]SolverChain),
		baseDenomToChains: make(map[string]map[string]*TokenInfo),
	}
	dr.buildLookupMaps()
	return dr
}

// SetChains sets the chain information for the resolver
func (dr *DenomResolver) SetChains(chains []SolverChain) {
	for _, chain := range chains {
		dr.chains[chain.Id] = chain
	}
	dr.buildLookupMaps()
}

// buildLookupMaps builds efficient lookup maps from the route index
func (dr *DenomResolver) buildLookupMaps() {
	// Build baseDenom:originChain -> chainID -> TokenInfo map
	for chainID, tokens := range dr.routeIndex.denomToTokenInfo {
		for _, tokenInfo := range tokens {
			key := tokenKey(tokenInfo.BaseDenom, tokenInfo.OriginChain)
			if dr.baseDenomToChains[key] == nil {
				dr.baseDenomToChains[key] = make(map[string]*TokenInfo)
			}
			dr.baseDenomToChains[key][chainID] = tokenInfo
		}
	}
}

// tokenKey creates a unique key for a token (baseDenom:originChain)
func tokenKey(baseDenom, originChain string) string {
	return baseDenom + ":" + originChain
}

// ResolveDenom resolves a denom on a specific chain and returns detailed information.
// Accepts either:
// - Human-readable denom (e.g., "uatone", "uosmo")
// - IBC denom (e.g., "ibc/ABC123...")
func (dr *DenomResolver) ResolveDenom(chainID, denom string) (*models.DenomInfo, error) {
	// First try direct lookup
	if info, err := dr.lookupDenom(chainID, denom); err == nil {
		return info, nil
	}

	// If not found and it looks like a base denom (not IBC), try to find it
	if !strings.HasPrefix(denom, "ibc/") {
		// Search through all tokens to find one with matching base denom
		if info, err := dr.resolveHumanReadableDenom(chainID, denom); err == nil {
			return info, nil
		}
	}

	return nil, fmt.Errorf("denom %s not found on chain %s", denom, chainID)
}

// lookupDenom does a direct lookup of a denom on a chain
func (dr *DenomResolver) lookupDenom(chainID, denom string) (*models.DenomInfo, error) {
	chainTokens, exists := dr.routeIndex.denomToTokenInfo[chainID]
	if !exists {
		return nil, fmt.Errorf("chain %s not found", chainID)
	}

	tokenInfo, found := chainTokens[denom]
	if !found {
		return nil, fmt.Errorf("denom %s not found on chain %s", denom, chainID)
	}

	// Find IBC path if applicable
	ibcPath := ""
	if tokenInfo.OriginChain != chainID {
		// Find the route this token came from
		for _, route := range dr.routeIndex.chainRoutes[chainID] {
			if _, allowed := route.AllowedTokens[denom]; allowed {
				ibcPath = route.PortId + "/" + route.ChannelId
				break
			}
		}
	}

	return &models.DenomInfo{
		ChainDenom:  tokenInfo.ChainDenom,
		BaseDenom:   tokenInfo.BaseDenom,
		OriginChain: tokenInfo.OriginChain,
		IsNative:    tokenInfo.OriginChain == chainID,
		IbcPath:     ibcPath,
	}, nil
}

// resolveHumanReadableDenom tries to find a token by its base denom on a chain.
// Supports disambiguation syntax: "denom@origin_chain" (e.g., "uusdc@noble-1")
// If multiple tokens have the same base denom and no origin is specified, returns an error.
func (dr *DenomResolver) resolveHumanReadableDenom(chainID, denomInput string) (*models.DenomInfo, error) {
	chainTokens, exists := dr.routeIndex.denomToTokenInfo[chainID]
	if !exists {
		return nil, fmt.Errorf("chain %s not found", chainID)
	}

	// Check for disambiguation syntax: denom@origin_chain
	baseDenom := denomInput
	wantedOrigin := ""
	if idx := strings.LastIndex(denomInput, "@"); idx > 0 {
		baseDenom = denomInput[:idx]
		wantedOrigin = denomInput[idx+1:]
	}

	// Collect all matching tokens
	var matches []*tokenMatch
	for denom, tokenInfo := range chainTokens {
		if tokenInfo.BaseDenom == baseDenom {
			// If origin specified, filter by it
			if wantedOrigin != "" && tokenInfo.OriginChain != wantedOrigin {
				continue
			}
			matches = append(matches, &tokenMatch{
				denom:     denom,
				tokenInfo: tokenInfo,
			})
		}
	}

	// Handle results
	if len(matches) == 0 {
		if wantedOrigin != "" {
			return nil, fmt.Errorf("token %s from %s not found on chain %s", baseDenom, wantedOrigin, chainID)
		}
		return nil, fmt.Errorf("token with base denom %s not found on chain %s", baseDenom, chainID)
	}

	if len(matches) > 1 {
		// Ambiguous - list the options
		origins := make([]string, len(matches))
		for i, m := range matches {
			origins[i] = m.tokenInfo.OriginChain
		}
		return nil, fmt.Errorf("ambiguous token %s on chain %s - available from: %s. Use %s@<origin_chain> to specify",
			baseDenom, chainID, strings.Join(origins, ", "), baseDenom)
	}

	// Single match - return it
	match := matches[0]
	ibcPath := ""
	if match.tokenInfo.OriginChain != chainID {
		for _, route := range dr.routeIndex.chainRoutes[chainID] {
			if _, allowed := route.AllowedTokens[match.denom]; allowed {
				ibcPath = route.PortId + "/" + route.ChannelId
				break
			}
		}
	}

	return &models.DenomInfo{
		ChainDenom:  match.denom,
		BaseDenom:   match.tokenInfo.BaseDenom,
		OriginChain: match.tokenInfo.OriginChain,
		IsNative:    match.tokenInfo.OriginChain == chainID,
		IbcPath:     ibcPath,
	}, nil
}

// tokenMatch is a helper for collecting matching tokens
type tokenMatch struct {
	denom     string
	tokenInfo *TokenInfo
}

// ResolveToChainDenom resolves a human-readable or IBC denom to the actual chain denom.
// Use this when you need the exact denom to use in transactions.
func (dr *DenomResolver) ResolveToChainDenom(chainID, denom string) (string, error) {
	info, err := dr.ResolveDenom(chainID, denom)
	if err != nil {
		return "", err
	}
	return info.ChainDenom, nil
}

// GetTokenDenomsAcrossChains returns all denoms for a token (identified by base denom and origin)
// across all supported chains. If onChainID is provided, returns only for that chain.
func (dr *DenomResolver) GetTokenDenomsAcrossChains(baseDenom, originChain, onChainID string) ([]models.ChainDenom, bool) {
	key := tokenKey(baseDenom, originChain)
	chainDenoms, exists := dr.baseDenomToChains[key]
	if !exists {
		return nil, false
	}

	result := make([]models.ChainDenom, 0, len(chainDenoms))

	for chainID, tokenInfo := range chainDenoms {
		// Filter by onChainID if specified
		if onChainID != "" && chainID != onChainID {
			continue
		}

		chainName := chainID
		if chain, ok := dr.chains[chainID]; ok {
			chainName = chain.Name
		}

		result = append(result, models.ChainDenom{
			ChainID:   chainID,
			ChainName: chainName,
			Denom:     tokenInfo.ChainDenom,
			IsNative:  tokenInfo.OriginChain == chainID,
		})
	}

	return result, len(result) > 0
}

// GetChainTokens returns all tokens available on a specific chain
func (dr *DenomResolver) GetChainTokens(chainID string) (*models.ChainTokens, error) {
	chainTokens, exists := dr.routeIndex.denomToTokenInfo[chainID]
	if !exists {
		return nil, fmt.Errorf("chain %s not found", chainID)
	}

	chainName := chainID
	if chain, ok := dr.chains[chainID]; ok {
		chainName = chain.Name
	}

	result := &models.ChainTokens{
		ChainID:      chainID,
		ChainName:    chainName,
		NativeTokens: make([]models.TokenDetails, 0),
		IBCTokens:    make([]models.TokenDetails, 0),
	}

	for denom, tokenInfo := range chainTokens {
		detail := models.TokenDetails{
			Denom:       denom,
			Symbol:      tokenInfo.Symbol,
			BaseDenom:   tokenInfo.BaseDenom,
			OriginChain: tokenInfo.OriginChain,
			Decimals:    tokenInfo.Decimals,
			IsNative:    tokenInfo.OriginChain == chainID,
		}

		if detail.IsNative {
			result.NativeTokens = append(result.NativeTokens, detail)
		} else {
			result.IBCTokens = append(result.IBCTokens, detail)
		}
	}

	return result, nil
}

// GetAvailableOn returns all chains where a specific token is available
func (dr *DenomResolver) GetAvailableOn(baseDenom, originChain string) []models.ChainDenom {
	denoms, _ := dr.GetTokenDenomsAcrossChains(baseDenom, originChain, "")
	return denoms
}

// InferTokenToDenom handles the case when token_to_denom is empty.
// It tries to find the same token (same origin) on the destination chain.
func (dr *DenomResolver) InferTokenToDenom(chainFrom, tokenFromDenom, chainTo string) (string, error) {
	// First resolve the source token
	sourceInfo, err := dr.ResolveDenom(chainFrom, tokenFromDenom)
	if err != nil {
		return "", fmt.Errorf("cannot resolve source token: %w", err)
	}

	// Find the same token on the destination chain
	destInfo, err := dr.resolveHumanReadableDenom(chainTo, sourceInfo.BaseDenom)
	if err != nil {
		// Try direct lookup with the base denom key
		denoms, found := dr.GetTokenDenomsAcrossChains(sourceInfo.BaseDenom, sourceInfo.OriginChain, chainTo)
		if !found || len(denoms) == 0 {
			return "", fmt.Errorf("token %s (origin: %s) not available on chain %s",
				sourceInfo.BaseDenom, sourceInfo.OriginChain, chainTo)
		}
		return denoms[0].Denom, nil
	}

	return destInfo.ChainDenom, nil
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
