package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/output"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/solver/router"
	"github.com/pelletier/go-toml/v2"
)

// ChainConfigLoader loads generated solver chain configurations and converts
// them to the router types used by the solver.
type ChainConfigLoader struct{}

// NewChainConfigLoader creates a new chain config loader.
func NewChainConfigLoader() *ChainConfigLoader {
	return &ChainConfigLoader{}
}

// LoadFromFile loads a solver config from a file and returns router-compatible types.
func (l *ChainConfigLoader) LoadFromFile(filePath string) ([]router.SolverChain, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read chain config file: %w", err)
	}

	var solverConfig output.SolverConfig

	if strings.HasSuffix(filePath, ".json") {
		if err := json.Unmarshal(data, &solverConfig); err != nil {
			return nil, fmt.Errorf("failed to parse JSON config: %w", err)
		}
	} else {
		if err := toml.Unmarshal(data, &solverConfig); err != nil {
			return nil, fmt.Errorf("failed to parse TOML config: %w", err)
		}
	}

	return l.ConvertToRouterTypes(&solverConfig)
}

// ConvertToRouterTypes converts a SolverConfig to the router.SolverChain type.
func (l *ChainConfigLoader) ConvertToRouterTypes(config *output.SolverConfig) ([]router.SolverChain, error) {
	if config == nil || len(config.Chains) == 0 {
		return nil, fmt.Errorf("no chains in config")
	}

	chains := make([]router.SolverChain, len(config.Chains))

	for i, solverChain := range config.Chains {
		chains[i] = router.SolverChain{
			Name:     solverChain.Name,
			Id:       solverChain.ID,
			HasPFM:   solverChain.HasPFM,
			Broker:   solverChain.Broker,
			BrokerId: solverChain.BrokerID,
			Routes:   make([]router.BasicRoute, len(solverChain.Routes)),
		}

		for j, route := range solverChain.Routes {
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
					Decimals:    tokenInfo.Decimals,
				}
			}
		}
	}

	return chains, nil
}

// InitializeSolver creates a fully initialized Solver from a config file.
// brokerClients should contain configured broker clients (e.g., Osmosis SQS client).
func (l *ChainConfigLoader) InitializeSolver(
	configPath string,
	brokerClients map[string]router.BrokerClient,
) (*router.Solver, error) {
	chains, err := l.LoadFromFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load chain config: %w", err)
	}

	// Build the route index
	routeIndex := router.NewRouteIndex()
	if err := routeIndex.BuildIndex(chains); err != nil {
		return nil, fmt.Errorf("failed to build route index: %w", err)
	}

	// Create and return the solver
	solver := router.NewSolver(chains, routeIndex, brokerClients)
	return solver, nil
}
