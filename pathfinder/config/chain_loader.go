package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/output"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/router"
	"github.com/pelletier/go-toml/v2"
)

// ChainConfigLoader loads generated pathfinder chain configurations and converts
// them to the router types used by the pathfinder.
type ChainConfigLoader struct{}

// NewChainConfigLoader creates a new chain config loader.
func NewChainConfigLoader() *ChainConfigLoader {
	return &ChainConfigLoader{}
}

// LoadFromFile loads a pathfinder config from a file and returns router-compatible types.
func (l *ChainConfigLoader) LoadFromFile(filePath string) ([]router.PathfinderChain, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read chain config file: %w", err)
	}

	var pathfinderConfig output.PathfinderConfig

	if strings.HasSuffix(filePath, ".json") {
		if err := json.Unmarshal(data, &pathfinderConfig); err != nil {
			return nil, fmt.Errorf("failed to parse JSON config: %w", err)
		}
	} else {
		if err := toml.Unmarshal(data, &pathfinderConfig); err != nil {
			return nil, fmt.Errorf("failed to parse TOML config: %w", err)
		}
	}

	return l.ConvertToRouterTypes(&pathfinderConfig)
}

// ConvertToRouterTypes converts a PathfinderConfig to the router.PathfinderChain type.
func (l *ChainConfigLoader) ConvertToRouterTypes(config *output.PathfinderConfig) ([]router.PathfinderChain, error) {
	if config == nil || len(config.Chains) == 0 {
		return nil, fmt.Errorf("no chains in config")
	}

	chains := make([]router.PathfinderChain, len(config.Chains))

	for i, pathfinderChain := range config.Chains {
		chains[i] = router.PathfinderChain{
			Name:             pathfinderChain.Name,
			Id:               pathfinderChain.ID,
			HasPFM:           pathfinderChain.HasPFM,
			Broker:           pathfinderChain.Broker,
			BrokerId:         pathfinderChain.BrokerID,
			IBCHooksContract: pathfinderChain.IBCHooksContract,
			Bech32Prefix:     pathfinderChain.Bech32Prefix,
			Routes:           make([]router.BasicRoute, len(pathfinderChain.Routes)),
		}

		for j, route := range pathfinderChain.Routes {
			chains[i].Routes[j] = router.BasicRoute{
				ToChain:       route.ToChain,
				ToChainId:     route.ToChainID,
				ConnectionId:  route.ConnectionID,
				ChannelId:     route.ChannelID,
				PortId:        route.PortID,
				AllowedTokens: make(map[string]router.TokenInfo),
			}

			for denom, tokenInfo := range route.AllowedTokens {
				chains[i].Routes[j].AllowedTokens[denom] = router.TokenInfo{
					ChainDenom:  tokenInfo.ChainDenom,
					IbcDenom:    tokenInfo.IBCDenom,
					BaseDenom:   tokenInfo.BaseDenom,
					OriginChain: tokenInfo.OriginChain,
					Symbol:      tokenInfo.Symbol,
					Decimals:    tokenInfo.Decimals,
				}
			}
		}
	}

	return chains, nil
}

// InitializePathfinder creates a fully initialized Pathfinder from a config file.
// brokerClients should contain configured broker clients (e.g., Osmosis SQS client).
func (l *ChainConfigLoader) InitializePathfinder(
	configPath string,
	brokerClients map[string]router.BrokerClient,
) (*router.Pathfinder, error) {
	chains, err := l.LoadFromFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load chain config: %w", err)
	}

	// Build the route index
	routeIndex := router.NewRouteIndex()
	if err := routeIndex.BuildIndex(chains); err != nil {
		return nil, fmt.Errorf("failed to build route index: %w", err)
	}

	// Create and return the pathfinder
	pathfinder := router.NewPathfinder(chains, routeIndex, brokerClients)
	return pathfinder, nil
}
