package enriched

import (
	"fmt"
	"log"
	"strings"

	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/input"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/query"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/registry"
)

// RouteBuilder builds IBC routes purely from configuration data.
// It does NOT query chains - it computes expected IBC denoms from:
// 1. Native tokens defined in input configs
// 2. IBC channels from the registry
// 3. Multi-hop tokens explicitly defined in configs
type RouteBuilder struct {
	inputConfigs map[string]*input.ChainInput
	ibcData      []registry.ChainIbcData

	// Lookup maps built during initialization
	chainByRegistry map[string]string                            // registry name -> chain ID
	channelMap      map[string]map[string]*ChannelInfo           // fromChainID -> toChainID -> channel info
	tokenLookup     map[string]map[string]*input.TokenMeta       // chainID -> denom -> token
}

// ChannelInfo contains channel information for a direct connection
type ChannelInfo struct {
	FromChainID           string
	ToChainID             string
	ChannelID             string // Our channel
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
	}
	rb.buildLookupMaps()
	return rb
}

func (rb *RouteBuilder) buildLookupMaps() {
	// Build registry name -> chain ID map
	for chainID, cfg := range rb.inputConfigs {
		rb.chainByRegistry[cfg.Chain.Registry] = chainID

		// Build token lookup
		rb.tokenLookup[chainID] = make(map[string]*input.TokenMeta)
		for i := range cfg.Tokens {
			token := &cfg.Tokens[i]
			rb.tokenLookup[chainID][token.Denom] = token
		}
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

	chainConfig := rb.inputConfigs[chainID]

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
			AllowedTokens:         rb.buildAllowedTokensForRoute(chainID, toChainID, channelInfo, chainConfig),
		}

		routes = append(routes, route)
	}

	return routes
}

// buildAllowedTokensForRoute computes allowed tokens for a route
func (rb *RouteBuilder) buildAllowedTokensForRoute(
	fromChainID, toChainID string,
	channelInfo *ChannelInfo,
	chainConfig *input.ChainInput,
) []RouteTokenInfo {
	tokens := make([]RouteTokenInfo, 0)

	// 1. Add native tokens from this chain that can be sent out
	for _, token := range chainConfig.Tokens {
		if !rb.isTokenAllowedToChain(&token, toChainID) {
			continue
		}

		// Compute IBC denom on destination chain
		trace := fmt.Sprintf("%s/%s/%s", channelInfo.PortID, channelInfo.ChannelID, token.Denom)
		ibcDenom := query.ComputeDenomHash(trace)

		tokens = append(tokens, RouteTokenInfo{
			SourceDenom:      token.Denom,
			DestinationDenom: ibcDenom,
			BaseDenom:        token.Denom,
			OriginChain:      fromChainID,
			Decimals:         token.Exponent,
		})
	}

	// 2. Add tokens received from the destination chain that can be sent back (unwound)
	toChainConfig := rb.inputConfigs[toChainID]
	if toChainConfig != nil {
		for _, token := range toChainConfig.Tokens {
			// Compute what the IBC denom is ON OUR CHAIN
			// (received via the counterparty's channel)
			receiveTrace := fmt.Sprintf("%s/%s/%s",
				channelInfo.PortID, channelInfo.CounterpartyChannelID, token.Denom)
			ibcDenomOnOurChain := query.ComputeDenomHash(receiveTrace)

			// When sent back, it unwounds to the native denom
			tokens = append(tokens, RouteTokenInfo{
				SourceDenom:      ibcDenomOnOurChain,
				DestinationDenom: token.Denom, // Unwound to native
				BaseDenom:        token.Denom,
				OriginChain:      toChainID,
				Decimals:         token.Exponent,
			})
		}
	}

	// 3. Add explicitly defined received tokens that came through this route
	for _, received := range chainConfig.ReceivedTokens {
		// Check if this token should go through toChainID
		if !rb.receivedTokenUsesRoute(received, fromChainID, toChainID) {
			continue
		}

		// Compute the IBC denom of this token on our chain
		ibcDenomOnOurChain := rb.computeReceivedTokenDenom(received, fromChainID)
		if ibcDenomOnOurChain == "" {
			continue
		}

		// Get token info from origin chain
		originToken := rb.getTokenFromChain(received.OriginChain, received.OriginDenom)
		decimals := 6 // Default
		if originToken != nil {
			decimals = originToken.Exponent
		}

		// Compute destination denom
		var destDenom string
		if toChainID == received.OriginChain {
			// Unwinding to origin
			destDenom = received.OriginDenom
		} else {
			// Forwarding further - compute new IBC denom
			trace := fmt.Sprintf("%s/%s/%s", channelInfo.PortID, channelInfo.ChannelID, ibcDenomOnOurChain)
			destDenom = query.ComputeDenomHash(trace)
		}

		tokens = append(tokens, RouteTokenInfo{
			SourceDenom:      ibcDenomOnOurChain,
			DestinationDenom: destDenom,
			BaseDenom:        received.OriginDenom,
			OriginChain:      received.OriginChain,
			Decimals:         decimals,
		})
	}

	return tokens
}

func (rb *RouteBuilder) isTokenAllowedToChain(token *input.TokenMeta, toChainID string) bool {
	if len(token.AllowedDestinations) == 0 {
		return true // No restrictions
	}
	for _, allowed := range token.AllowedDestinations {
		if allowed == toChainID {
			return true
		}
	}
	return false
}

// receivedTokenUsesRoute checks if a received token's path goes through toChainID
func (rb *RouteBuilder) receivedTokenUsesRoute(received input.ReceivedToken, ourChainID, toChainID string) bool {
	if len(received.ViaChains) == 0 {
		// Direct transfer - check if toChainID is the origin
		return toChainID == received.OriginChain
	}

	// Multi-hop - check if toChainID is in the path
	// The path is: OriginChain -> ViaChains[0] -> ViaChains[1] -> ... -> OurChain
	// When sending back, we reverse: OurChain -> ViaChains[last] -> ... -> OriginChain

	// Check if toChainID is the last via chain (immediate hop back)
	if received.ViaChains[len(received.ViaChains)-1] == toChainID {
		return true
	}

	return false
}

// computeReceivedTokenDenom computes what IBC denom a received token has on our chain
func (rb *RouteBuilder) computeReceivedTokenDenom(received input.ReceivedToken, ourChainID string) string {
	if len(received.ViaChains) == 0 {
		// Direct transfer from origin
		channel := rb.GetChannel(ourChainID, received.OriginChain)
		if channel == nil {
			return ""
		}
		// Note: we receive via the counterparty's channel
		trace := fmt.Sprintf("transfer/%s/%s", channel.CounterpartyChannelID, received.OriginDenom)
		return query.ComputeDenomHash(trace)
	}

	// Multi-hop: build the path backwards
	// Token travels: OriginChain -> via[0] -> via[1] -> ... -> via[n-1] -> OurChain
	// The IBC path on our chain is: transfer/channel-to-via[n-1]/transfer/channel-via[n-1]-to-via[n-2]/.../denom

	path := ""
	currentChain := ourChainID

	// Walk backwards through the via chains
	for i := len(received.ViaChains) - 1; i >= 0; i-- {
		prevChain := received.ViaChains[i]
		channel := rb.GetChannel(currentChain, prevChain)
		if channel == nil {
			log.Printf("Warning: no channel between %s and %s for received token", currentChain, prevChain)
			return ""
		}
		// We receive via the counterparty's channel
		path = fmt.Sprintf("transfer/%s/%s", channel.CounterpartyChannelID, path)
		currentChain = prevChain
	}

	// Finally, add the hop from via[0] to origin chain
	channel := rb.GetChannel(received.ViaChains[0], received.OriginChain)
	if channel == nil {
		log.Printf("Warning: no channel between %s and %s for received token", received.ViaChains[0], received.OriginChain)
		return ""
	}
	path = fmt.Sprintf("%stransfer/%s/%s", path, channel.CounterpartyChannelID, received.OriginDenom)

	return query.ComputeDenomHash(path)
}

func (rb *RouteBuilder) getTokenFromChain(chainID, denom string) *input.TokenMeta {
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

	// 1. Tokens received directly from connected chains
	for toChainID := range rb.channelMap[chainID] {
		toChainConfig := rb.inputConfigs[toChainID]
		if toChainConfig == nil {
			continue
		}

		channel := rb.GetChannel(chainID, toChainID)
		if channel == nil {
			continue
		}

		for _, token := range toChainConfig.Tokens {
			// Compute IBC denom on our chain
			trace := fmt.Sprintf("transfer/%s/%s", channel.CounterpartyChannelID, token.Denom)
			ibcDenom := query.ComputeDenomHash(trace)

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
				IBCPath:       fmt.Sprintf("transfer/%s", channel.CounterpartyChannelID),
				SourceChannel: channel.ChannelID,
			})
		}
	}

	// 2. Explicitly defined received tokens (multi-hop)
	for _, received := range chainConfig.ReceivedTokens {
		ibcDenom := rb.computeReceivedTokenDenom(received, chainID)
		if ibcDenom == "" || seen[ibcDenom] {
			continue
		}
		seen[ibcDenom] = true

		// Get token info from origin
		originToken := rb.getTokenFromChain(received.OriginChain, received.OriginDenom)

		name := received.DisplayName
		symbol := received.DisplaySymbol
		decimals := 6
		icon := ""

		if originToken != nil {
			if name == "" {
				name = originToken.Name
			}
			if symbol == "" {
				symbol = originToken.Symbol
			}
			decimals = originToken.Exponent
			icon = originToken.Icon
		}

		// Build the IBC path description
		pathParts := []string{}
		pathParts = append(pathParts, received.ViaChains...)
		pathParts = append(pathParts, received.OriginChain)

		tokens = append(tokens, IBCTokenConfig{
			IBCDenom:      ibcDenom,
			BaseDenom:     received.OriginDenom,
			Name:          name,
			Symbol:        symbol,
			Decimals:      decimals,
			Icon:          icon,
			OriginChain:   received.OriginChain,
			IBCPath:       fmt.Sprintf("via %s", strings.Join(pathParts, " -> ")),
			SourceChannel: "", // Multi-hop doesn't have single source
		})
	}

	return tokens
}

