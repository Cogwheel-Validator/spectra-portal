package enriched

import (
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/input"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/query"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/registry"
)

// RouteBuilder builds IBC routes purely from configuration data.
// It computes expected IBC denoms from:
//
//  1. Native tokens - tokens where OriginChain is empty, can only be sent FROM this chain
//  2. Routable IBC tokens - tokens where OriginChain is set, these are IBC tokens
//     that have been received and can be forwarded to specific destinations
//
// Key principle: A route only allows tokens that:
//   - Are NATIVE to the source chain (sending out their first hop)
//   - Originated from the destination chain (unwinding/sending back)
//   - Are explicitly defined as ROUTABLE on this chain for that destination
type RouteBuilder struct {
	inputConfigs map[string]*input.ChainInput
	ibcData      []registry.ChainIbcData

	// Lookup maps built during initialization
	chainByRegistry map[string]string                      // registry name -> chain ID
	channelMap      map[string]map[string]*ChannelInfo     // fromChainID -> toChainID -> channel info
	tokenLookup     map[string]map[string]*input.TokenMeta // chainID -> denom -> token
	nativeTokens    map[string][]*input.TokenMeta          // chainID -> native tokens only
	routableTokens  map[string][]*input.TokenMeta          // chainID -> routable IBC tokens only
}

// ChannelInfo contains channel information for a direct connection
type ChannelInfo struct {
	FromChainID           string
	ToChainID             string
	ChannelID             string // Our channel (tokens arrive via this channel)
	CounterpartyChannelID string // Their channel
	PortID                string
	ConnectionID          string
	Status                string
}

// NewRouteBuilder creates a new route builder
func NewRouteBuilder(inputConfigs map[string]*input.ChainInput, ibcData []registry.ChainIbcData) *RouteBuilder {
	rb := &RouteBuilder{
		inputConfigs:    inputConfigs,
		ibcData:         ibcData,
		chainByRegistry: make(map[string]string),
		channelMap:      make(map[string]map[string]*ChannelInfo),
		tokenLookup:     make(map[string]map[string]*input.TokenMeta),
		nativeTokens:    make(map[string][]*input.TokenMeta),
		routableTokens:  make(map[string][]*input.TokenMeta),
	}
	rb.buildLookupMaps()
	return rb
}

func (rb *RouteBuilder) buildLookupMaps() {
	// Build registry name -> chain ID map and token lookups
	for chainID, cfg := range rb.inputConfigs {
		rb.chainByRegistry[cfg.Chain.Registry] = chainID

		// Build token lookup and categorize tokens
		rb.tokenLookup[chainID] = make(map[string]*input.TokenMeta)
		rb.nativeTokens[chainID] = make([]*input.TokenMeta, 0)
		rb.routableTokens[chainID] = make([]*input.TokenMeta, 0)

		for i := range cfg.Tokens {
			token := &cfg.Tokens[i]
			rb.tokenLookup[chainID][token.Denom] = token

			if token.IsNative() {
				rb.nativeTokens[chainID] = append(rb.nativeTokens[chainID], token)
			} else {
				rb.routableTokens[chainID] = append(rb.routableTokens[chainID], token)
			}
		}

		log.Printf("Chain %s: %d native tokens, %d routable IBC tokens",
			chainID, len(rb.nativeTokens[chainID]), len(rb.routableTokens[chainID]))
	}

	// Build channel map from IBC registry
	for _, data := range rb.ibcData {
		chain1ID := rb.chainByRegistry[data.Chain1.ChainName]
		chain2ID := rb.chainByRegistry[data.Chain2.ChainName]

		if chain1ID == "" || chain2ID == "" {
			continue // One of the chains is not in our config
		}

		// Find the preferred active channel
		for _, channel := range data.Channels {
			status := strings.ToUpper(channel.Tags.Status)
			isActive := status == "ACTIVE" || status == "LIVE"
			if !channel.Tags.Preferred || !isActive {
				continue
			}

			// Add channel for chain1 -> chain2
			if rb.channelMap[chain1ID] == nil {
				rb.channelMap[chain1ID] = make(map[string]*ChannelInfo)
			}
			rb.channelMap[chain1ID][chain2ID] = &ChannelInfo{
				FromChainID:           chain1ID,
				ToChainID:             chain2ID,
				ChannelID:             channel.Chain1.ChannelID,
				CounterpartyChannelID: channel.Chain2.ChannelID,
				PortID:                channel.Chain1.PortID,
				ConnectionID:          data.Chain1.ConnectionID,
				Status:                channel.Tags.Status,
			}

			// Add channel for chain2 -> chain1
			if rb.channelMap[chain2ID] == nil {
				rb.channelMap[chain2ID] = make(map[string]*ChannelInfo)
			}
			rb.channelMap[chain2ID][chain1ID] = &ChannelInfo{
				FromChainID:           chain2ID,
				ToChainID:             chain1ID,
				ChannelID:             channel.Chain2.ChannelID,
				CounterpartyChannelID: channel.Chain1.ChannelID,
				PortID:                channel.Chain2.PortID,
				ConnectionID:          data.Chain2.ConnectionID,
				Status:                channel.Tags.Status,
			}

			log.Printf("Channel: %s <-> %s via %s/%s",
				chain1ID, chain2ID, channel.Chain1.ChannelID, channel.Chain2.ChannelID)
			break // Only use first preferred active channel
		}
	}
}

// GetChannel returns channel info between two chains, or nil if no channel exists
func (rb *RouteBuilder) GetChannel(fromChainID, toChainID string) *ChannelInfo {
	if channels, ok := rb.channelMap[fromChainID]; ok {
		return channels[toChainID]
	}
	return nil
}

// BuildRoutesForChain builds all routes for a specific chain
func (rb *RouteBuilder) BuildRoutesForChain(chainID string) []RouteConfig {
	routes := make([]RouteConfig, 0)

	channels, ok := rb.channelMap[chainID]
	if !ok {
		return routes
	}

	for toChainID, channelInfo := range channels {
		toChainConfig := rb.inputConfigs[toChainID]
		if toChainConfig == nil {
			continue
		}

		route := RouteConfig{
			ToChainID:             toChainID,
			ToChainName:           toChainConfig.Chain.Name,
			ConnectionID:          channelInfo.ConnectionID,
			ChannelID:             channelInfo.ChannelID,
			PortID:                channelInfo.PortID,
			CounterpartyChannelID: channelInfo.CounterpartyChannelID,
			State:                 channelInfo.Status,
			AllowedTokens:         rb.buildAllowedTokensForRoute(chainID, toChainID, channelInfo),
		}

		routes = append(routes, route)
	}

	return routes
}

// buildAllowedTokensForRoute computes allowed tokens for a route.
//
// The logic is:
//
//  1. NATIVE tokens from source chain: These are tokens that originate on this chain
//     and can be sent out (first hop). Only included if destination is in AllowedDestinations.
//
//  2. Tokens originally from DESTINATION chain: IBC tokens that came from the destination
//     can be sent back (unwinding). These are computed, not explicitly defined.
//
//  3. ROUTABLE IBC tokens: Explicitly defined IBC tokens on the source chain that can
//     be forwarded to specific destinations. Only included if destination matches.
func (rb *RouteBuilder) buildAllowedTokensForRoute(
	fromChainID, toChainID string,
	channelInfo *ChannelInfo,
) []RouteTokenInfo {
	tokens := make([]RouteTokenInfo, 0)
	seen := make(map[string]bool) // Track added tokens by source denom

	log.Printf("Building tokens for route %s -> %s (channel %s)", fromChainID, toChainID, channelInfo.ChannelID)

	// === 1. NATIVE tokens from this chain ===
	// Only tokens that are truly native (OriginChain is empty)
	for _, token := range rb.nativeTokens[fromChainID] {
		if !rb.isTokenAllowedToChain(token, toChainID) {
			log.Printf("\tSkipping native token %s - not allowed to %s", token.Denom, toChainID)
			continue
		}

		// Compute IBC denom on destination chain
		// When we SEND, the destination receives: ibc/hash(transfer/THEIR_CHANNEL/denom)
		ibcDenom := rb.computeIBCDenom(channelInfo.PortID, channelInfo.CounterpartyChannelID, token.Denom)

		log.Printf("\tAdding native token %s -> %s", token.Denom, ibcDenom)

		tokens = append(tokens, RouteTokenInfo{
			SourceDenom:      token.Denom,
			DestinationDenom: ibcDenom,
			BaseDenom:        token.Denom,
			OriginChain:      fromChainID,
			Decimals:         token.Exponent,
		})
		seen[token.Denom] = true
	}

	// === 2. Tokens that ORIGINATED from the destination chain (unwinding) ===
	// If destination chain has native tokens, we might have received them via IBC.
	// When we send them back, they unwind to native.
	for _, token := range rb.nativeTokens[toChainID] {
		// Compute IBC denom of this token ON OUR CHAIN
		// When we RECEIVE from toChain: ibc/hash(transfer/OUR_CHANNEL/denom)
		ibcDenomOnOurChain := rb.computeIBCDenom(channelInfo.PortID, channelInfo.ChannelID, token.Denom)

		if seen[ibcDenomOnOurChain] {
			continue
		}

		log.Printf("\tAdding unwind token %s (from %s) -> %s (native)",
			ibcDenomOnOurChain, toChainID, token.Denom)

		tokens = append(tokens, RouteTokenInfo{
			SourceDenom:      ibcDenomOnOurChain,
			DestinationDenom: token.Denom, // Unwound to native
			BaseDenom:        token.Denom,
			OriginChain:      toChainID,
			Decimals:         token.Exponent,
		})
		seen[ibcDenomOnOurChain] = true
	}

	// === 3. ROUTABLE IBC tokens defined on this chain ===
	// These are IBC tokens that we've received and explicitly want to forward
	for _, token := range rb.routableTokens[fromChainID] {
		if !rb.isTokenAllowedToChain(token, toChainID) {
			log.Printf("\tSkipping routable token %s - not allowed to %s", token.Denom, toChainID)
			continue
		}

		if seen[token.Denom] {
			continue
		}

		// The token is already an IBC denom on our chain
		// When we forward it, compute what it becomes on destination
		destDenom := rb.computeForwardedTokenDenom(token, channelInfo)

		log.Printf("\tAdding routable token %s (origin: %s) -> %s",
			token.Denom, token.OriginChain, destDenom)

		tokens = append(tokens, RouteTokenInfo{
			SourceDenom:      token.Denom,
			DestinationDenom: destDenom,
			BaseDenom:        token.OriginDenom,
			OriginChain:      token.OriginChain,
			Decimals:         token.Exponent,
		})
		seen[token.Denom] = true
	}

	return tokens
}

// computeForwardedTokenDenom computes what an IBC token becomes when forwarded
func (rb *RouteBuilder) computeForwardedTokenDenom(token *input.TokenMeta, channelInfo *ChannelInfo) string {
	// If we're sending to the token's origin chain, it might unwind
	if channelInfo.ToChainID == token.OriginChain {
		// Check if the path is direct (single hop IBC denom)
		// For simplicity, assume direct path unwinding
		return token.OriginDenom
	}

	// Otherwise, the token gets wrapped again on the destination
	// New denom = ibc/hash(transfer/THEIR_CHANNEL/our_ibc_denom)
	return rb.computeIBCDenom(channelInfo.PortID, channelInfo.CounterpartyChannelID, token.Denom)
}

// computeIBCDenom computes the IBC denom hash for a token
func (rb *RouteBuilder) computeIBCDenom(port, channel, denom string) string {
	trace := fmt.Sprintf("%s/%s/%s", port, channel, denom)
	return query.ComputeDenomHash(trace)
}

func (rb *RouteBuilder) isTokenAllowedToChain(token *input.TokenMeta, toChainID string) bool {
	if len(token.AllowedDestinations) == 0 {
		return true // No restrictions
	}
	return slices.Contains(token.AllowedDestinations, toChainID)
}

func (rb *RouteBuilder) GetTokenFromChain(chainID, denom string) *input.TokenMeta {
	if tokens, ok := rb.tokenLookup[chainID]; ok {
		return tokens[denom]
	}
	return nil
}

// BuildIBCTokensForChain computes IBC tokens that exist on a chain
func (rb *RouteBuilder) BuildIBCTokensForChain(chainID string) []IBCTokenConfig {
	tokens := make([]IBCTokenConfig, 0)
	seen := make(map[string]bool)

	chainConfig := rb.inputConfigs[chainID]
	if chainConfig == nil {
		return tokens
	}

	// 1. Tokens received directly from connected chains (their native tokens)
	for toChainID := range rb.channelMap[chainID] {
		channel := rb.GetChannel(chainID, toChainID)
		if channel == nil {
			continue
		}

		// Get native tokens from the connected chain
		for _, token := range rb.nativeTokens[toChainID] {
			// Compute IBC denom on our chain (using our channel where tokens arrive)
			ibcDenom := rb.computeIBCDenom("transfer", channel.ChannelID, token.Denom)

			if seen[ibcDenom] {
				continue
			}
			seen[ibcDenom] = true

			tokens = append(tokens, IBCTokenConfig{
				IBCDenom:      ibcDenom,
				BaseDenom:     token.Denom,
				Name:          token.Name,
				Symbol:        token.Symbol,
				Decimals:      token.Exponent,
				Icon:          token.Icon,
				OriginChain:   toChainID,
				IBCPath:       fmt.Sprintf("transfer/%s", channel.ChannelID),
				SourceChannel: channel.ChannelID,
			})
		}
	}

	// 2. Routable IBC tokens defined on this chain
	// These are explicitly declared IBC tokens that can be forwarded
	for _, token := range rb.routableTokens[chainID] {
		if seen[token.Denom] {
			continue
		}
		seen[token.Denom] = true

		tokens = append(tokens, IBCTokenConfig{
			IBCDenom:      token.Denom,
			BaseDenom:     token.OriginDenom,
			Name:          token.Name,
			Symbol:        token.Symbol,
			Decimals:      token.Exponent,
			Icon:          token.Icon,
			OriginChain:   token.OriginChain,
			IBCPath:       fmt.Sprintf("multi-hop from %s", token.OriginChain),
			SourceChannel: "", // Multi-hop
		})
	}

	return tokens
}
