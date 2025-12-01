package enriched

import (
	"fmt"
	"log"
	"time"

	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/input"
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
type Builder struct {
	retryAttempts int
	retryDelay    time.Duration
	timeout       time.Duration
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

// Deprecated: queries are no longer used
// WithSkipDenomQueries is deprecated - queries are no longer used.
// Kept for some inital idea but will be removed in future.
func WithSkipDenomQueries(skip bool) BuilderOption {
	return func(b *Builder) {
		// No-op - we don't query anymore
	}
}

// NewBuilder creates a new enriched config builder.
func NewBuilder(opts ...BuilderOption) *Builder {
	b := &Builder{
		retryAttempts: defaultRetryAttempts,
		retryDelay:    defaultRetryDelay,
		timeout:       defaultTimeout,
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
// 3. Multi-hop tokens explicitly defined via received_token
func (b *Builder) BuildRegistry(
	inputConfigs map[string]*input.ChainInput,
	ibcData []registry.ChainIbcData,
) (*RegistryConfig, error) {
	if len(inputConfigs) == 0 {
		return nil, fmt.Errorf("no input configurations provided")
	}

	reg := &RegistryConfig{
		// TODO: Add proper versioning
		Version:     "1.0.0",
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
		Name:         chain.Name,
		ID:           chain.ID,
		Type:         chain.Type,
		Registry:     chain.Registry,
		ExplorerURL:  chain.ExplorerURL,
		Slip44:       chain.Slip44,
		Bech32Prefix: chain.Bech32Prefix,
		IsBroker:     chain.IsBroker,
		BrokerID:     chain.BrokerID,
	}

	// Set PFM support (from input or default to false)
	if chain.HasPFM != nil {
		config.HasPFM = *chain.HasPFM
	}

	// Convert REST/RPC endpoints
	config.HealthyRPCs = b.convertEndpoints(chain.RPCs)
	config.HealthyRests = b.convertEndpoints(chain.Rest)

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

func (b *Builder) convertEndpoints(endpoints []input.APIEndpoint) []Endpoint {
	result := make([]Endpoint, len(endpoints))
	for i, ep := range endpoints {
		result[i] = Endpoint{
			URL:      ep.URL,
			Provider: ep.Provider,
			Healthy:  true, // Assume healthy for now
		}
	}
	return result
}

func (b *Builder) buildNativeTokens(tokens []input.TokenMeta, chainID string) []TokenConfig {
	result := make([]TokenConfig, len(tokens))
	for i, token := range tokens {
		result[i] = TokenConfig{
			Denom:               token.Denom,
			Name:                token.Name,
			Symbol:              token.Symbol,
			Decimals:            token.Exponent,
			Icon:                token.Icon,
			CoinGeckoID:         token.CoinGeckoID,
			OriginChain:         chainID,
			AllowedDestinations: token.AllowedDestinations,
		}
	}
	return result
}
