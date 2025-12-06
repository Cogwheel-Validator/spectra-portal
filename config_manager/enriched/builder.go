package enriched

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/input"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/query"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/registry"
)

const (
	defaultRetryAttempts = 3
	defaultRetryDelay    = 2 * time.Second
	defaultTimeout       = 10 * time.Second
)

// Builder builds enriched configurations from input configs and IBC registry data.
// It computes IBC denoms deterministically from the defined tokens and channels,
// without querying chain REST APIs. This ensures only "legitimate" tokens are included.
// It also validates the network reachability of the chain fully.
type Builder struct {
	retryAttempts int
	retryDelay    time.Duration
	timeout       time.Duration
	skipNetCheck  bool
}

// BuilderOption configures the builder.
type BuilderOption func(*Builder)

// WithRetryAttempts sets the number of retry attempts for API calls.
func WithRetryAttempts(attempts int) BuilderOption {
	return func(b *Builder) {
		b.retryAttempts = attempts
	}
}

// WithRetryDelay sets the delay between retries.
func WithRetryDelay(delay time.Duration) BuilderOption {
	return func(b *Builder) {
		b.retryDelay = delay
	}
}

// WithTimeout sets the timeout for API calls.
func WithTimeout(timeout time.Duration) BuilderOption {
	return func(b *Builder) {
		b.timeout = timeout
	}
}

// WithSkipNetworkCheck disables network reachability checks.
func WithSkipNetworkCheck(skip bool) BuilderOption {
	return func(b *Builder) {
		b.skipNetCheck = skip
	}
}

// NewBuilder creates a new enriched config builder.
func NewBuilder(opts ...BuilderOption) *Builder {
	b := &Builder{
		retryAttempts: defaultRetryAttempts,
		retryDelay:    defaultRetryDelay,
		timeout:       defaultTimeout,
		skipNetCheck:  false,
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// BuildRegistry builds the complete enriched registry from input configs.
// Routes and IBC tokens are computed deterministically from:
// 1. Native tokens defined in each chain's config
// 2. IBC channels from the registry
// 3. Routable IBC tokens (multi-hop support via origin_chain/origin_denom)
func (b *Builder) BuildRegistry(
	inputConfigs map[string]*input.ChainInput,
	ibcData []registry.ChainIbcData,
) (*RegistryConfig, error) {
	if len(inputConfigs) == 0 {
		return nil, fmt.Errorf("no input configurations provided")
	}

	reg := &RegistryConfig{
		// Use date as a version
		Version:     "v" + strings.ReplaceAll(time.Now().UTC().Format(time.DateOnly), "-", ""),
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Chains:      make(map[string]*ChainConfig),
	}

	// Create the route builder with all configs and IBC data
	routeBuilder := NewRouteBuilder(inputConfigs, ibcData)

	// Build each chain config
	for chainID, inputCfg := range inputConfigs {
		chainConfig, err := b.buildChainConfig(inputCfg, routeBuilder)
		if err != nil {
			log.Printf("Warning: failed to build config for chain %s: %v", chainID, err)
			continue
		}
		reg.Chains[chainID] = chainConfig
	}

	if len(reg.Chains) == 0 {
		return nil, fmt.Errorf("failed to build any chain configurations")
	}

	return reg, nil
}

// buildChainConfig builds an enriched chain configuration from an input config.
func (b *Builder) buildChainConfig(
	inputCfg *input.ChainInput,
	routeBuilder *RouteBuilder,
) (*ChainConfig, error) {
	chain := inputCfg.Chain

	config := &ChainConfig{
		Name:             chain.Name,
		ID:               chain.ID,
		Type:             chain.Type,
		Registry:         chain.Registry,
		ExplorerURL:      chain.ExplorerURL,
		Slip44:           chain.Slip44,
		Bech32Prefix:     chain.Bech32Prefix,
		IsBroker:         chain.IsBroker,
		BrokerID:         chain.BrokerID,
		IBCHooksContract: chain.IBCHooksContract,
	}

	// Set PFM support (from input or default to false)
	if chain.HasPFM != nil {
		config.HasPFM = *chain.HasPFM
	}

	// Convert REST/RPC endpoints
	config.HealthyRPCs = b.convertEndpoints(chain.RPCs, false)
	config.HealthyRests = b.convertEndpoints(chain.Rest, true)

	// Convert native tokens
	config.NativeTokens = b.buildNativeTokens(inputCfg.Tokens, chain.ID)

	// Build routes using the route builder (computed, not queried)
	config.Routes = routeBuilder.BuildRoutesForChain(chain.ID)

	// Build IBC tokens (computed, not queried)
	config.IBCTokens = routeBuilder.BuildIBCTokensForChain(chain.ID)

	log.Printf("Chain %s: %d routes, %d native tokens, %d IBC tokens",
		chain.ID, len(config.Routes), len(config.NativeTokens), len(config.IBCTokens))

	return config, nil
}

func (b *Builder) convertEndpoints(
	endpoints []input.APIEndpoint,
	checkingRest bool,
) []Endpoint {
	// Skip network validation - just mark all as healthy (assumed)
	if b.skipNetCheck {
		result := make([]Endpoint, 0, len(endpoints))
		for _, endpoint := range endpoints {
			result = append(result, Endpoint{
				URL:      endpoint.URL,
				Provider: endpoint.Provider,
				Healthy:  true,
			})
		}
		return result
	}

	// Perform full network validation
	var healthyEndpoints map[query.URLProvider]bool
	if checkingRest {
		healthyEndpoints = query.ValidateRestEndpoints(endpoints, b.retryAttempts, b.retryDelay, b.timeout)
	} else {
		healthyEndpoints = query.ValidateRpcEndpoints(endpoints, b.retryAttempts, b.retryDelay, b.timeout)
	}

	result := make([]Endpoint, 0, len(healthyEndpoints))
	for ep, healthy := range healthyEndpoints {
		result = append(result, Endpoint{
			URL:      ep.URL,
			Provider: ep.Provider,
			Healthy:  healthy,
		})
	}
	return result
}

func (b *Builder) buildNativeTokens(tokens []input.TokenMeta, chainID string) []TokenConfig {
	result := make([]TokenConfig, 0, len(tokens))
	for _, token := range tokens {
		// Only include native tokens (those without OriginChain)
		if !token.IsNative() {
			continue
		}
		result = append(result, TokenConfig{
			Denom:               token.Denom,
			Name:                token.Name,
			Symbol:              token.Symbol,
			Decimals:            token.Exponent,
			Icon:                token.Icon,
			CoinGeckoID:         token.CoinGeckoID,
			OriginChain:         chainID,
			AllowedDestinations: token.AllowedDestinations,
		})
	}
	return result
}
