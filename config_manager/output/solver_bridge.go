package output

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// LoadSolverConfig loads a solver configuration from a file.
// Supports both TOML and JSON formats based on file extension.
func LoadSolverConfig(filePath string) (*SolverConfig, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read solver config: %w", err)
	}

	var config SolverConfig

	if strings.HasSuffix(filePath, ".json") {
		if err := json.Unmarshal(data, &config); err != nil {
			return nil, fmt.Errorf("failed to parse JSON solver config: %w", err)
		}
	} else {
		if err := toml.Unmarshal(data, &config); err != nil {
			return nil, fmt.Errorf("failed to parse TOML solver config: %w", err)
		}
	}

	return &config, nil
}

// ToRouterTypes converts the output solver types to the types expected by
// the solver/router package. This bridges the config_manager output to
// the runtime router structures.
//
// Usage:
//
//	config, _ := output.LoadSolverConfig("path/to/solver_config.toml")
//	chains, err := config.ToRouterTypes()
//	routeIndex := router.NewRouteIndex()
//	routeIndex.BuildIndex(chains)
//	solver := router.NewSolver(chains, routeIndex, brokerClients)
type RouterChain struct {
	Name     string
	ID       string
	HasPFM   bool
	Broker   bool
	BrokerID string
	Routes   []RouterRoute
}

type RouterRoute struct {
	ToChain       string
	ToChainID     string
	ConnectionID  string
	ChannelID     string
	PortID        string
	AllowedTokens map[string]RouterTokenInfo
}

type RouterTokenInfo struct {
	ChainDenom  string
	IBCDenom    string
	BaseDenom   string
	OriginChain string
	Symbol      string
	Decimals    int
}

// ToRouterTypes converts the solver config to router-compatible types.
func (c *SolverConfig) ToRouterTypes() ([]RouterChain, error) {
	if c == nil || len(c.Chains) == 0 {
		return nil, fmt.Errorf("no chains in solver config")
	}

	chains := make([]RouterChain, len(c.Chains))
	for i, solverChain := range c.Chains {
		chains[i] = RouterChain{
			Name:     solverChain.Name,
			ID:       solverChain.ID,
			HasPFM:   solverChain.HasPFM,
			Broker:   solverChain.Broker,
			BrokerID: solverChain.BrokerID,
			Routes:   make([]RouterRoute, len(solverChain.Routes)),
		}

		for j, route := range solverChain.Routes {
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
