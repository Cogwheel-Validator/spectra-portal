package output

import (
	"fmt"
	"slices"
	"time"

	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/enriched"
)

// ClientConverter converts enriched configs to frontend-compatible format.
type ClientConverter struct {
	// Optional: base URL for chain logos if not specified per-chain
	chainLogoBaseURL string
}

// ClientConverterOption configures the client converter.
type ClientConverterOption func(*ClientConverter)

// WithChainLogoBaseURL sets the base URL for chain logos.
func WithChainLogoBaseURL(url string) ClientConverterOption {
	return func(c *ClientConverter) {
		c.chainLogoBaseURL = url
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

	for _, chainConfig := range reg.Chains {
		clientChain := c.convertChain(chainConfig, reg, tokenTracker)
		config.Chains = append(config.Chains, clientChain)
	}

	// Convert token tracker to slice
	for _, token := range tokenTracker {
		config.AllTokens = append(config.AllTokens, *token)
	}

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
		ExplorerURL:  chain.ExplorerURL,
		IsDEX:        chain.IsBroker,
		ChainLogo:    c.getChainLogo(chain),
	}

	// Convert healthy endpoints
	clientChain.RPCEndpoints = c.convertEndpoints(chain.HealthyRPCs)
	clientChain.RESTEndpoints = c.convertEndpoints(chain.HealthyRests)

	// Convert native tokens
	clientChain.NativeTokens = make([]ClientToken, 0, len(chain.NativeTokens))
	for _, token := range chain.NativeTokens {
		clientToken := ClientToken{
			Denom:       token.Denom,
			Name:        token.Name,
			Symbol:      token.Symbol,
			Decimals:    token.Decimals,
			Icon:        token.Icon,
			OriginChain: chain.ID,
			CoinGeckoID: token.CoinGeckoID,
			IsNative:    true,
		}
		clientChain.NativeTokens = append(clientChain.NativeTokens, clientToken)

		// Track for AllTokens
		c.trackToken(tokenTracker, token.Denom, token.Name, token.Symbol,
			token.Icon, chain.ID, chain.Name, token.CoinGeckoID, chain.ID)
	}

	// Convert IBC tokens
	clientChain.IBCTokens = make([]ClientToken, 0, len(chain.IBCTokens))
	for _, token := range chain.IBCTokens {
		originChainName := ""
		if originChain, exists := reg.Chains[token.OriginChain]; exists {
			originChainName = originChain.Name
		}

		clientToken := ClientToken{
			Denom:           token.IBCDenom,
			Name:            token.Name,
			Symbol:          token.Symbol,
			Decimals:        token.Decimals,
			Icon:            token.Icon,
			OriginChain:     token.OriginChain,
			OriginChainName: originChainName,
			IsNative:        false,
			BaseDenom:       token.BaseDenom,
		}
		clientChain.IBCTokens = append(clientChain.IBCTokens, clientToken)

		// Track for AllTokens
		c.trackToken(tokenTracker, token.BaseDenom, token.Name, token.Symbol,
			token.Icon, token.OriginChain, originChainName, "", chain.ID)
	}

	// Build connected chains info
	clientChain.ConnectedChains = c.buildConnectedChains(chain, reg)

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
	if c.chainLogoBaseURL != "" {
		return fmt.Sprintf("%s/%s/logo.png", c.chainLogoBaseURL, chain.Registry)
	}
	return ""
}

func (c *ClientConverter) trackToken(
	tracker map[string]*ClientTokenSummary,
	baseDenom, name, symbol, icon, originChain, originChainName, coinGeckoID, availableOnChain string,
) {
	key := fmt.Sprintf("%s:%s", originChain, baseDenom)

	if existing, exists := tracker[key]; exists {
		// Add this chain to availableOn if not already there
		found := slices.Contains(existing.AvailableOn, availableOnChain)
		if !found {
			existing.AvailableOn = append(existing.AvailableOn, availableOnChain)
		}
	} else {
		tracker[key] = &ClientTokenSummary{
			BaseDenom:       baseDenom,
			Symbol:          symbol,
			Name:            name,
			Icon:            icon,
			OriginChain:     originChain,
			OriginChainName: originChainName,
			CoinGeckoID:     coinGeckoID,
			AvailableOn:     []string{availableOnChain},
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

		// Collect sendable token symbols
		sendableTokens := make([]string, 0)
		for _, token := range route.AllowedTokens {
			// Find the token symbol
			for _, nativeToken := range chain.NativeTokens {
				if nativeToken.Denom == token.SourceDenom {
					sendableTokens = append(sendableTokens, nativeToken.Symbol)
					break
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
