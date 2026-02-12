package output

import (
	"fmt"
	"log"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Cogwheel-Validator/spectra-portal/config_manager/enriched"
)

// ClientConverter converts enriched configs to frontend-compatible format.
type ClientConverter struct {
	// Whether to copy the icons to the public/icons directory
	copyIcons bool
}

// ClientConverterOption configures the client converter.
type ClientConverterOption func(*ClientConverter)

// WithIconCopy sets whether to copy the icons to the public/icons directory.
// If true, the icons will be copied to the public/icons directory.
func WithIconCopy(copy bool) ClientConverterOption {
	return func(c *ClientConverter) {
		c.copyIcons = copy
	}
}

// NewClientConverter creates a new client converter.
func NewClientConverter(opts ...ClientConverterOption) *ClientConverter {
	c := &ClientConverter{}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Convert transforms an enriched registry config into a client config.
func (c *ClientConverter) Convert(reg *enriched.RegistryConfig) (*ClientConfig, error) {
	if reg == nil || len(reg.Chains) == 0 {
		return nil, fmt.Errorf("no chains to convert")
	}

	config := &ClientConfig{
		Version:     reg.Version,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Chains:      make([]ClientChain, 0, len(reg.Chains)),
		AllTokens:   make([]ClientTokenSummary, 0),
	}

	// Track unique tokens for the AllTokens summary
	tokenTracker := make(map[string]*ClientTokenSummary) // key: originChain:baseDenom

	chainIDs := make([]string, 0, len(reg.Chains))
	for chainID := range reg.Chains {
		chainIDs = append(chainIDs, chainID)
	}
	sort.Strings(chainIDs)

	for _, chainID := range chainIDs {
		chainConfig := reg.Chains[chainID]
		clientChain := c.convertChain(chainConfig, reg, tokenTracker)
		config.Chains = append(config.Chains, clientChain)
	}

	// Convert token tracker to slice
	for _, token := range tokenTracker {
		config.AllTokens = append(config.AllTokens, *token)
	}

	slices.SortStableFunc(config.AllTokens, func(a, b ClientTokenSummary) int {
		return strings.Compare(a.BaseDenom, b.BaseDenom)
	})

	return config, nil
}

func (c *ClientConverter) convertChain(
	chain *enriched.ChainConfig,
	reg *enriched.RegistryConfig,
	tokenTracker map[string]*ClientTokenSummary,
) ClientChain {
	clientChain := ClientChain{
		Name:         chain.Name,
		ID:           chain.ID,
		Bech32Prefix: chain.Bech32Prefix,
		Slip44:       chain.Slip44,
		ExplorerDetails: ExplorerDetails{
			BaseUrl:         chain.ExplorerDetails.Url,
			AccountPath:     chain.ExplorerDetails.AccountPath,
			TransactionPath: chain.ExplorerDetails.TransactionPath,
		},
		IsDEX:            chain.IsBroker,
		ChainLogo:        c.getChainLogo(chain),
		KeplrChainConfig: chain.KeplrChainConfig,
	}

	// Convert healthy endpoints
	clientChain.RPCEndpoints = c.convertEndpoints(chain.HealthyRPCs)
	clientChain.RESTEndpoints = c.convertEndpoints(chain.HealthyRests)

	slices.SortStableFunc(clientChain.RPCEndpoints, func(a, b ClientEndpoint) int {
		return strings.Compare(a.URL, b.URL)
	})
	slices.SortStableFunc(clientChain.RESTEndpoints, func(a, b ClientEndpoint) int {
		return strings.Compare(a.URL, b.URL)
	})

	clientChain.CosmosSdkVersion = chain.CosmosSdkVersion

	// Track all allowed tokens that can traverse to other chains via simple map tied to their on chain denom
	// with their allowd chain destinations
	tokenTrajectories := make(map[string][]string)

	// Convert native tokens
	clientChain.NativeTokens = make([]ClientToken, 0, len(chain.NativeTokens))
	for _, token := range chain.NativeTokens {
		clientToken := ClientToken{
			Denom:       token.Denom,
			Name:        token.Name,
			Symbol:      token.Symbol,
			Decimals:    token.Decimals,
			Icon:        c.convertUrlToIcon(token.Icon),
			OriginChain: chain.ID,
			CoinGeckoID: token.CoinGeckoID,
			IsNative:    true,
		}
		clientChain.NativeTokens = append(clientChain.NativeTokens, clientToken)

		// Track for AllTokens
		c.trackToken(tokenTracker, &clientToken)

		// Track token trajectories
		// but check if there is "none", it should be already checked but just to be sure
		if len(token.AllowedDestinations) == 1 && slices.Contains(token.AllowedDestinations, "none") {
			tokenTrajectories[token.Denom] = []string{}
		} else {
			tokenTrajectories[token.Denom] = token.AllowedDestinations
		}
	}

	slices.SortStableFunc(chain.IBCTokens, func(a, b enriched.IBCTokenConfig) int {
		return strings.Compare(a.IBCDenom, b.IBCDenom)
	})

	// Convert IBC tokens
	clientChain.IBCTokens = []ClientToken{} // no allocations needed, will be filled in below
	// Only include tokens that are explicitly allowed to reach this chain
	for _, token := range chain.IBCTokens {
		originChainName := ""
		if originChain, exists := reg.Chains[token.OriginChain]; exists {
			originChainName = originChain.Name
		}

		// Check if the token from its origin chain is allowed to be sent to THIS chain
		// Look up the original token definition on its origin chain
		originChainConfig := reg.Chains[token.OriginChain]
		if originChainConfig != nil {
			tokenAllowed := false
			// Check if this token (identified by baseDenom) is allowed to reach our chain
			for _, nativeToken := range originChainConfig.NativeTokens {
				if nativeToken.Denom == token.BaseDenom {
					// Check if current chain is in the allowed destinations
					// Empty list means allowed everywhere
					if len(nativeToken.AllowedDestinations) == 0 {
						tokenAllowed = true
					} else if slices.Contains(nativeToken.AllowedDestinations, chain.ID) {
						tokenAllowed = true
					}
					break
				}
			}

			if !tokenAllowed {
				log.Printf("Skipping IBC token %s on %s - not in allowed destinations from %s",
					token.Symbol, chain.ID, token.OriginChain)
				continue
			}
		}

		clientToken := ClientToken{
			Denom:           token.IBCDenom,
			Name:            token.Name,
			Symbol:          token.Symbol,
			Decimals:        token.Decimals,
			Icon:            c.convertUrlToIcon(token.Icon),
			OriginChain:     token.OriginChain,
			OriginChainName: originChainName,
			IsNative:        false,
			BaseDenom:       token.BaseDenom,
		}
		clientChain.IBCTokens = append(clientChain.IBCTokens, clientToken)

		// Track for AllTokens
		c.trackToken(tokenTracker, &clientToken)
	}

	slices.SortStableFunc(clientChain.IBCTokens, func(a, b ClientToken) int {
		return strings.Compare(a.Denom, b.Denom)
	})

	// Build connected chains info
	clientChain.ConnectedChains = c.buildConnectedChains(chain, reg)

	slices.SortStableFunc(clientChain.ConnectedChains, func(a, b ConnectedChainInfo) int {
		return strings.Compare(a.ID, b.ID)
	})

	return clientChain
}

func (c *ClientConverter) convertEndpoints(endpoints []enriched.Endpoint) []ClientEndpoint {
	result := make([]ClientEndpoint, 0)
	for _, ep := range endpoints {
		if ep.Healthy {
			result = append(result, ClientEndpoint{
				URL:      ep.URL,
				Provider: ep.Provider,
			})
		}
	}
	return result
}

func (c *ClientConverter) getChainLogo(chain *enriched.ChainConfig) string {
	// If copy icons option is enabled the program should assume that there is also a logo
	// in the public/icons directory within the frontend project.
	if c.copyIcons {
		return fmt.Sprintf("/icons/%s/logo.png", chain.Registry)
	}
	return ""
}

func (c *ClientConverter) trackToken(
	tracker map[string]*ClientTokenSummary,
	clientToken *ClientToken,
) {
	key := fmt.Sprintf("%s:%s", clientToken.OriginChain, clientToken.Denom)

	if existing, exists := tracker[key]; exists {
		// Add this chain to availableOn if not already there
		found := slices.Contains(existing.AvailableOn, clientToken.OriginChain)
		if !found {
			existing.AvailableOn = append(existing.AvailableOn, clientToken.OriginChain)
		}
	} else {
		tracker[key] = &ClientTokenSummary{
			BaseDenom:       clientToken.Denom,
			Symbol:          clientToken.Symbol,
			Name:            clientToken.Name,
			Icon:            clientToken.Icon,
			OriginChain:     clientToken.OriginChain,
			OriginChainName: clientToken.OriginChainName,
			CoinGeckoID:     clientToken.CoinGeckoID,
			AvailableOn:     []string{clientToken.OriginChain},
		}
	}
}

func (c *ClientConverter) buildConnectedChains(
	chain *enriched.ChainConfig,
	reg *enriched.RegistryConfig,
) []ConnectedChainInfo {
	connected := make([]ConnectedChainInfo, 0, len(chain.Routes))

	for _, route := range chain.Routes {
		destChain, exists := reg.Chains[route.ToChainID]
		if !exists {
			continue
		}

		// Collect sendable token symbols - check both native and IBC tokens
		sendableTokens := make([]string, 0)
		for _, token := range route.AllowedTokens {
			// First check native tokens
			found := false
			for _, nativeToken := range chain.NativeTokens {
				if nativeToken.Denom == token.SourceDenom {
					sendableTokens = append(sendableTokens, nativeToken.Symbol)
					found = true
					break
				}
			}

			// If not found in native tokens, check IBC tokens
			if !found {
				for _, ibcToken := range chain.IBCTokens {
					if ibcToken.IBCDenom == token.SourceDenom {
						sendableTokens = append(sendableTokens, ibcToken.Symbol)
						break
					}
				}
			}
		}

		info := ConnectedChainInfo{
			ID:             route.ToChainID,
			Name:           destChain.Name,
			Logo:           c.getChainLogo(destChain),
			SendableTokens: sendableTokens,
		}
		connected = append(connected, info)
	}

	return connected
}

// Convert the URL of icon to make a path to the icon within the
// public/icons directory within the frontend project
// Example: https://raw.githubusercontent.com/Cogwheel-Validator/spectra-ibc-hub/main/images/osmosis/osmo.png
// becomes /icons/osmosis/osmo.png
//
// It will only work if the option to copy this data is enabled.
//
// Parameters:
// - url: The URL of the icon
//
// Returns:
//
// - The path: to the icon within the public/icons directory
// - The original URL: if the option to copy this data is disabled
func (c *ClientConverter) convertUrlToIcon(url string) string {
	if c.copyIcons {
		// Get the last part of the URL
		splitUrl := strings.Split(url, "/")
		iconPath := strings.Join(splitUrl[len(splitUrl)-2:], "/")
		// Return the path to the icon within the public/icons directory
		return fmt.Sprintf("/icons/%s", iconPath)
	}
	return url
}
