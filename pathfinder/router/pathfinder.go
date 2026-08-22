package router

import (
	"errors"
	"fmt"
	"os"
	"time"

	models "github.com/Cogwheel-Validator/spectra-portal/pathfinder/models"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/addressing"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/brokers"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/denomresolver"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/routeindex"
	"github.com/rs/zerolog"
)

var pathfinderLog zerolog.Logger

func init() {
	out := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}
	pathfinderLog = zerolog.New(out).With().Timestamp().Str("component", "pathfinder").Logger()
}

// Pathfinder orchestrates route finding and integrates with broker DEX APIs
type Pathfinder struct {
	chainsMap        map[string]routeindex.PathfinderChain // mapped chainId -> PathfinderChain
	routeIndex       *routeindex.RouteIndex                // routeIndex from which all routes are found
	brokerClients    map[string]brokers.BrokerClient       // mapped brokerId -> broker client interface
	denomResolver    *denomresolver.DenomResolver          // denomResolver for resolving denoms across chains
	addressConverter *addressing.AddressConverter          // addressConverter for converting addresses across chains
	maxRetries       int                                   // maximum number of retries for broker queries
	retryDelay       time.Duration                         // delay between retries for broker queries
}

// NewPathfinder creates a new Pathfinder with the given route index and broker clients
func NewPathfinder(chains []routeindex.PathfinderChain, routeIndex *routeindex.RouteIndex, brokerClients map[string]brokers.BrokerClient) *Pathfinder {
	chainMap := make(map[string]routeindex.PathfinderChain, len(chains))
	for _, chain := range chains {
		chainMap[chain.Id] = chain
	}
	return &Pathfinder{
		chainsMap:        chainMap,
		routeIndex:       routeIndex,
		brokerClients:    brokerClients,
		denomResolver:    denomresolver.NewDenomResolver(routeIndex),
		addressConverter: addressing.NewAddressConverter(chains),
		maxRetries:       3,
		retryDelay:       500 * time.Millisecond,
	}
}

// FindPath attempts to find a route for the given request and returns execution details
// Priority order: 1) Direct route, 2) Indirect route (no swap), 3) Broker swap route
func (s *Pathfinder) FindPath(req models.RouteRequest) models.RouteResponse {
	pathfinderLog.Info().
		Str("chainFrom", req.ChainFrom).
		Str("chainTo", req.ChainTo).
		Str("tokenFrom", req.TokenFromDenom).
		Str("tokenTo", req.TokenToDenom).
		Str("amount", req.AmountIn).
		Bool("mock", len(req.Addresses) == 0).
		Msg("Solving route")

	return s.markIfMock(req, s.findPath(req))
}

// markIfMock flags successful responses to a mock (empty-address) discovery
// request so the RPC layer can mark them as not executable.
func (s *Pathfinder) markIfMock(req models.RouteRequest, resp models.RouteResponse) models.RouteResponse {
	if resp.Success && len(req.Addresses) == 0 {
		resp.Mock = true
	}
	return resp
}

func (s *Pathfinder) findPath(req models.RouteRequest) models.RouteResponse {
	// First, try to find a direct IBC route (no swap needed)
	directRoute := s.routeIndex.FindDirectRoute(req)
	if directRoute != nil {
		pathfinderLog.Info().Msg("Found direct route")
		return s.buildDirectResponse(req, directRoute)
	}
	pathfinderLog.Debug().Msg("No direct route found")

	// Second, try to find an indirect route (multi-hop without swap)
	indirectRoute := s.routeIndex.FindIndirectRoute(req)
	if indirectRoute != nil {
		pathfinderLog.Info().Int("hops", len(indirectRoute.Path)-1).Msg("Found indirect route")
		return s.buildIndirectResponse(req, indirectRoute)
	}
	pathfinderLog.Debug().Msg("No indirect route found")

	// Third, try multi-hop routes through brokers with swap
	brokerRoutes := s.routeIndex.FindMultiHopRoute(req)
	if len(brokerRoutes) == 0 {
		pathfinderLog.Warn().Msg("No route found")
		return models.RouteResponse{
			Success:      false,
			RouteType:    "impossible",
			ErrorMessage: "No route found between chains for the requested tokens",
		}
	}

	pathfinderLog.Info().Int("candidates", len(brokerRoutes)).Msg("Found broker route candidates")

	// Try each broker route and query the broker for swap details
	var lastErr error
	for i, hopInfo := range brokerRoutes {
		pathfinderLog.Debug().
			Int("attempt", i+1).
			Str("broker", hopInfo.BrokerChain).
			Bool("swapOnly", hopInfo.SwapOnly).
			Msg("Trying broker route")

		response, err := s.buildBrokerSwapResponse(req, hopInfo)
		if err == nil {
			pathfinderLog.Info().Str("broker", hopInfo.BrokerChain).Msg("Broker route succeeded")
			return response
		}
		// A route was found but the request's address map does not cover it;
		// trying other brokers would not fix the request.
		var missingErr *MissingAddressesError
		if errors.As(err, &missingErr) {
			pathfinderLog.Warn().Strs("missingChains", missingErr.ChainIDs).Msg("Route found but addresses are missing")
			return models.RouteResponse{
				Success:              false,
				RouteType:            "impossible",
				ErrorMessage:         missingErr.Error(),
				MissingAddressChains: missingErr.ChainIDs,
			}
		}
		lastErr = err
		pathfinderLog.Debug().Err(err).Str("broker", hopInfo.BrokerChain).Msg("Broker route failed, trying next")
	}

	// All brokers failed or returned no valid route
	errMsg := "Broker swap route found but broker query failed"
	if lastErr != nil {
		errMsg = fmt.Sprintf("Broker swap route found but query failed: %v", lastErr)
	}
	pathfinderLog.Warn().Err(lastErr).Msg("All broker routes failed")
	return models.RouteResponse{
		Success:      false,
		RouteType:    "impossible",
		ErrorMessage: errMsg,
	}
}

// getMapKeys returns the keys of a map as a slice
func getMapKeys(m map[string]brokers.BrokerClient) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// queryBrokerWithRetry queries any broker DEX with exponential backoff retry logic
func (s *Pathfinder) queryBrokerWithRetry(
	client brokers.BrokerClient,
	amountIn string,
	tokenInDenom string,
	tokenOutDenom string,
	singleRoute *bool,
) (*brokers.SwapResult, error) {
	var lastErr error
	delay := s.retryDelay

	for attempt := 0; attempt <= s.maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(delay)
			delay *= 2 // Exponential backoff
		}

		// Query broker for the swap route
		result, err := client.QuerySwap(tokenInDenom, amountIn, tokenOutDenom, singleRoute)
		if err == nil {
			return result, nil
		}

		lastErr = err
	}

	return nil, fmt.Errorf("%s query failed after %d attempts: %w", client.GetBrokerType(), s.maxRetries+1, lastErr)
}

/*
GetChainInfo returns the information about a specific chain

Parameters:
- chainId: the id of the chain to get information for

Returns:
- PathfinderChain: the information about the chain
- error: if the chain is not found
*/
func (s *Pathfinder) GetChainInfo(chainId string) (routeindex.PathfinderChain, error) {
	chain, exists := s.chainsMap[chainId]
	if !exists {
		return routeindex.PathfinderChain{}, fmt.Errorf("chain %s not found", chainId)
	}
	return chain, nil
}

/*
GetAllChains returns the list of all chains

Returns:
- []string: the list of all chain ids
*/
func (s *Pathfinder) GetAllChains() []string {
	chains := make([]string, 0, len(s.chainsMap))
	for chainId := range s.chainsMap {
		chains = append(chains, chainId)
	}
	return chains
}
