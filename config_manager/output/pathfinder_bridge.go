package output

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// LoadPathfinderConfig loads a pathfinder configuration from a file.
// Supports both TOML and JSON formats based on file extension.
func LoadPathfinderConfig(filePath string) (*PathfinderConfig, error) {
	data, err := os.ReadFile(filePath) //nolint:gosec // G304: filePath is an operator-supplied startup config path, not external input
	if err != nil {
		return nil, fmt.Errorf("failed to read pathfinder config: %w", err)
	}

	var config PathfinderConfig

	if strings.HasSuffix(filePath, ".json") {
		if err := json.Unmarshal(data, &config); err != nil {
			return nil, fmt.Errorf("failed to parse JSON pathfinder config: %w", err)
		}
	} else {
		if err := toml.Unmarshal(data, &config); err != nil {
			return nil, fmt.Errorf("failed to parse TOML pathfinder config: %w", err)
		}
	}

	return &config, nil
}

// RouterChain is the router-compatible representation of a pathfinder chain.
// It bridges the config_manager output to the runtime router structures.
//
// Usage:
//
//	config, _ := output.LoadPathfinderConfig("path/to/pathfinder_config.toml")
//	chains, err := config.ToRouterTypes()
//	routeIndex := router.NewRouteIndex()
//	routeIndex.BuildIndex(chains)
//	pathfinder := router.NewPathfinder(chains, routeIndex, brokerClients)
type RouterChain struct {
	Name     string
	ID       string
	HasPFM   bool
	Broker   bool
	BrokerID string
	Routes   []RouterRoute
}

// RouterRoute is the router-compatible representation of a pathfinder route.
type RouterRoute struct {
	ToChain       string
	ToChainID     string
	ConnectionID  string
	ChannelID     string
	PortID        string
	AllowedTokens map[string]RouterTokenInfo
}

// RouterTokenInfo is the router-compatible representation of an allowed token.
type RouterTokenInfo struct {
	ChainDenom  string
	IBCDenom    string
	BaseDenom   string
	OriginChain string
	Symbol      string
	Decimals    int
}

// ToRouterTypes converts the pathfinder config to router-compatible types.
func (c *PathfinderConfig) ToRouterTypes() ([]RouterChain, error) {
	if c == nil || len(c.Chains) == 0 {
		return nil, fmt.Errorf("no chains in pathfinder config")
	}

	chains := make([]RouterChain, len(c.Chains))
	for i, pathfinderChain := range c.Chains {
		chains[i] = RouterChain{
			Name:     pathfinderChain.Name,
			ID:       pathfinderChain.ID,
			HasPFM:   pathfinderChain.HasPFM,
			Broker:   pathfinderChain.Broker,
			BrokerID: pathfinderChain.BrokerID,
			Routes:   make([]RouterRoute, len(pathfinderChain.Routes)),
		}

		for j, route := range pathfinderChain.Routes {
			chains[i].Routes[j] = RouterRoute{
				ToChain:       route.ToChain,
				ToChainID:     route.ToChainID,
				ConnectionID:  route.ConnectionID,
				ChannelID:     route.ChannelID,
				PortID:        route.PortID,
				AllowedTokens: make(map[string]RouterTokenInfo),
			}

			for denom, tokenInfo := range route.AllowedTokens {
				chains[i].Routes[j].AllowedTokens[denom] = RouterTokenInfo(tokenInfo)
			}
		}
	}

	return chains, nil
}
