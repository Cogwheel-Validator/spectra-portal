package rpc

import (
	"context"

	"connectrpc.com/connect"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/solver/models"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/solver/router"
	v1 "github.com/Cogwheel-Validator/spectra-ibc-hub/solver/rpc/v1"
	v1connect "github.com/Cogwheel-Validator/spectra-ibc-hub/solver/rpc/v1/v1connect"
	"google.golang.org/protobuf/types/known/emptypb"
)

// SolverServer implements the ConnectRPC SolverServiceHandler interface
type SolverServer struct {
	solver        *router.Solver
	denomResolver *router.DenomResolver
}

// Verify that SolverServer implements the interface
var _ v1connect.SolverServiceHandler = (*SolverServer)(nil)

// NewSolverServer creates a new SolverServer
func NewSolverServer(solver *router.Solver, denomResolver *router.DenomResolver) *SolverServer {
	return &SolverServer{
		solver:        solver,
		denomResolver: denomResolver,
	}
}

// SolveRoute implements the ConnectRPC handler for solving routes
func (s *SolverServer) SolveRoute(
	ctx context.Context,
	req *connect.Request[v1.SolveRouteRequest],
) (*connect.Response[v1.SolveRouteResponse], error) {
	// Step 1: Convert proto request to internal models.RouteRequest
	internalReq := models.RouteRequest{
		ChainFrom:       req.Msg.ChainFrom,
		TokenFromDenom:  req.Msg.TokenFromDenom,
		AmountIn:        req.Msg.AmountIn,
		ChainTo:         req.Msg.ChainTo,
		TokenToDenom:    req.Msg.TokenToDenom,
		SenderAddress:   req.Msg.SenderAddress,
		ReceiverAddress: req.Msg.ReceiverAddress,
	}

	// Step 2: Call your existing solver logic with internal types
	internalResp := s.solver.Solve(internalReq)

	// Step 3: Convert internal models.RouteResponse to proto response
	protoResp := convertToProtoResponse(&internalResp)

	// Step 4: Return wrapped in connect.Response
	return connect.NewResponse(protoResp), nil
}

// LookupDenom implements the ConnectRPC handler for denom lookup
func (s *SolverServer) LookupDenom(
	ctx context.Context,
	req *connect.Request[v1.LookupDenomRequest],
) (*connect.Response[v1.LookupDenomResponse], error) {
	denomInfo, err := s.denomResolver.ResolveDenom(req.Msg.ChainId, req.Msg.Denom)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	return connect.NewResponse(&v1.LookupDenomResponse{
		ChainDenom:  denomInfo.ChainDenom,
		BaseDenom:   denomInfo.BaseDenom,
		OriginChain: denomInfo.OriginChain,
		IsNative:    denomInfo.IsNative,
		IbcPath:     denomInfo.IbcPath,
	}), nil
}

/*
GetChainInfo returns the information about a specific chain

Parameters:
- chainId: the id of the chain to get information for

Returns:
- *v1.ChainInfo: the information about the chain
- *connect.Error: if the chain is not found
*/
func (s *SolverServer) GetChainInfo(
	ctx context.Context,
	req *connect.Request[v1.ChainInfoRequest],
) (*connect.Response[v1.ChainInfoResponse], error) {
	chain, err := s.solver.GetChainInfo(req.Msg.ChainId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	return connect.NewResponse(&v1.ChainInfoResponse{
		ChainInfo: convertToProtoChainInfo(&chain),
	}), nil
}

/*
GetSolverSupportedChains returns the list of all supported chains

Returns:
- []string: the list of all chain ids
*/
func (s *SolverServer) GetSolverSupportedChains(
	ctx context.Context,
	req *connect.Request[emptypb.Empty],
) (*connect.Response[v1.SolverSupportedChainsResponse], error) {
	chains := s.solver.GetAllChains()
	return connect.NewResponse(&v1.SolverSupportedChainsResponse{
		ChainIds: chains,
	}), nil
}

// CONVERT FUNCTIONS
// These convert between internal models and protobuf types

/*
Converts internal models.RouteResponse to v1.SolveRouteResponse
It uses the protobuf oneof to represent the different route types.

Parameters:
- resp: *models.RouteResponse

Returns:
- *v1.SolveRouteResponse

Errors:
- None
*/
func convertToProtoResponse(resp *models.RouteResponse) *v1.SolveRouteResponse {
	protoResp := &v1.SolveRouteResponse{
		Success:      resp.Success,
		ErrorMessage: resp.ErrorMessage,
	}

	// Convert Direct route if present (using protobuf oneof)
	if resp.Direct != nil {
		protoResp.Route = &v1.SolveRouteResponse_Direct{
			Direct: convertToProtoDirectRoute(resp.Direct),
		}
	}

	// Convert Indirect route if present (using protobuf oneof)
	if resp.Indirect != nil {
		protoResp.Route = &v1.SolveRouteResponse_Indirect{
			Indirect: convertToProtoIndirectRoute(resp.Indirect),
		}
	}

	// Convert BrokerSwap route if present (using protobuf oneof)
	if resp.BrokerSwap != nil {
		protoResp.Route = &v1.SolveRouteResponse_BrokerSwap{
			BrokerSwap: convertToProtoBrokerSwapRoute(resp.BrokerSwap),
		}
	}

	return protoResp
}

/*
Converts internal models.DirectRoute to v1.DirectRoute

Parameters:
- direct: *models.DirectRoute

Returns:
- *v1.DirectRoute

Errors:
- None
*/
func convertToProtoDirectRoute(direct *models.DirectRoute) *v1.DirectRoute {
	return &v1.DirectRoute{
		Transfer: convertToProtoIBCLeg(direct.Transfer),
	}
}

/*
Converts internal models.IndirectRoute to v1.IndirectRoute

Parameters:
- indirect: *models.IndirectRoute

Returns:
- *v1.IndirectRoute

Errors:
- None
*/
func convertToProtoIndirectRoute(indirect *models.IndirectRoute) *v1.IndirectRoute {
	legs := make([]*v1.IBCLeg, len(indirect.Legs))
	for i, leg := range indirect.Legs {
		legs[i] = convertToProtoIBCLeg(leg)
	}
	return &v1.IndirectRoute{
		Path:          indirect.Path,
		Legs:          legs,
		SupportsPfm:   indirect.SupportsPFM,
		PfmStartChain: indirect.PFMStartChain,
		PfmMemo:       indirect.PFMMemo,
	}
}

/*
Converts internal models.BrokerRoute to v1.BrokerSwapRoute

Parameters:
- brokerSwap: *models.BrokerRoute

Returns:
- *v1.BrokerSwapRoute

Errors:
- None
*/
func convertToProtoBrokerSwapRoute(brokerSwap *models.BrokerRoute) *v1.BrokerSwapRoute {
	return &v1.BrokerSwapRoute{
		Path:                brokerSwap.Path,
		InboundLeg:          convertToProtoIBCLeg(brokerSwap.InboundLeg),
		Swap:                convertToProtoSwapQuote(brokerSwap.Swap),
		OutboundLeg:         convertToProtoIBCLeg(brokerSwap.OutboundLeg),
		OutboundSupportsPfm: brokerSwap.OutboundSupportsPFM,
		OutboundPfmMemo:     brokerSwap.OutboundPFMMemo,
	}
}

/*
Converts internal models.IBCLeg to v1.IBCLeg

Parameters:
- leg: *models.IBCLeg

Returns:
- *v1.IBCLeg

Errors:
- None
*/
func convertToProtoIBCLeg(leg *models.IBCLeg) *v1.IBCLeg {
	if leg == nil {
		return nil
	}
	return &v1.IBCLeg{
		FromChain: leg.FromChain,
		ToChain:   leg.ToChain,
		Channel:   leg.Channel,
		Port:      leg.Port,
		Token:     convertToProtoTokenMapping(leg.Token),
		Amount:    leg.Amount,
	}
}

/*
Converts internal models.TokenMapping to v1.TokenMapping

Parameters:
- token: *models.TokenMapping

Returns:
- *v1.TokenMapping

Errors:
- None
*/
func convertToProtoTokenMapping(token *models.TokenMapping) *v1.TokenMapping {
	if token == nil {
		return nil
	}
	return &v1.TokenMapping{
		ChainDenom:  token.ChainDenom,
		BaseDenom:   token.BaseDenom,
		OriginChain: token.OriginChain,
		IsNative:    token.IsNative,
	}
}

/*
Converts internal models.SwapQuote to v1.SwapQuote

Parameters:
- swap: *models.SwapQuote

Returns:
- *v1.SwapQuote

Errors:
- None
*/
func convertToProtoSwapQuote(swap *models.SwapQuote) *v1.SwapQuote {
	if swap == nil {
		return nil
	}

	protoSwap := &v1.SwapQuote{
		Broker:       swap.Broker,
		TokenIn:      convertToProtoTokenMapping(swap.TokenIn),
		TokenOut:     convertToProtoTokenMapping(swap.TokenOut),
		AmountIn:     swap.AmountIn,
		AmountOut:    swap.AmountOut,
		PriceImpact:  swap.PriceImpact,
		EffectiveFee: swap.EffectiveFee,
	}

	// Convert broker-specific RouteData based on broker type
	// This is the key part - converting interface{} to typed oneof
	switch swap.Broker {
	case "osmosis":
		if osmosisData, ok := swap.RouteData.(*router.OsmosisRouteData); ok {
			protoSwap.RouteData = &v1.SwapQuote_OsmosisRouteData{
				OsmosisRouteData: convertOsmosisRouteData(osmosisData),
			}
		}
		// Add more brokers here as you implement them:
		// case "astroport" for example:
	}

	return protoSwap
}

/*
Converts internal models.OsmosisRouteData to v1.OsmosisRouteData

Parameters:
- data: *models.OsmosisRouteData

Returns:
- *v1.OsmosisRouteData

Errors:
- None
*/
func convertOsmosisRouteData(data *router.OsmosisRouteData) *v1.OsmosisRouteData {
	if data == nil {
		return nil
	}

	routes := make([]*v1.OsmosisRoute, len(data.Routes))
	for i, route := range data.Routes {
		pools := make([]*v1.OsmosisPool, len(route.Pools))
		for j, pool := range route.Pools {
			pools[j] = &v1.OsmosisPool{
				Id:            int32(pool.ID),
				Type:          int32(pool.Type),
				SpreadFactor:  pool.SpreadFactor,
				TokenOutDenom: pool.TokenOutDenom,
				TakerFee:      pool.TakerFee,
				LiquidityCap:  pool.LiquidityCap,
			}
		}

		routes[i] = &v1.OsmosisRoute{
			Pools:     pools,
			HasCwPool: route.HasCwPool,
			OutAmount: route.OutAmount,
			InAmount:  route.InAmount,
		}
	}

	return &v1.OsmosisRouteData{
		Routes:               routes,
		LiquidityCap:         data.LiquidityCap,
		LiquidityCapOverflow: data.LiquidityCapOverflow,
	}
}

func convertToProtoChainInfo(chain *router.SolverChain) *v1.ChainInfo {
	return &v1.ChainInfo{
		ChainId:   chain.Id,
		ChainName: chain.Name,
		HasPfm:    chain.HasPFM,
		IsBroker:  chain.Broker,
		Routes:    convertToProtoBasicRoute(chain.Routes),
	}
}

func convertToProtoBasicRoute(routes []router.BasicRoute) []*v1.BasicRoute {
	protoRoutes := make([]*v1.BasicRoute, len(routes))
	for i := range routes {
		protoRoutes[i] = &v1.BasicRoute{
			ToChain:       routes[i].ToChain,
			ToChainId:     routes[i].ToChainId,
			ConnectionId:  routes[i].ConnectionId,
			ChannelId:     routes[i].ChannelId,
			PortId:        routes[i].PortId,
			AllowedTokens: convertToProtoTokenInfo(routes[i].AllowedTokens),
		}
	}
	return protoRoutes
}

func convertToProtoTokenInfo(tokenInfo map[string]router.TokenInfo) map[string]*v1.TokenInfo {
	protoTokenInfos := make(map[string]*v1.TokenInfo, len(tokenInfo))
	for denom, tokenInfo := range tokenInfo {
		protoTokenInfos[denom] = &v1.TokenInfo{
			ChainDenom:  tokenInfo.ChainDenom,
			IbcDenom:    tokenInfo.IbcDenom,
			BaseDenom:   tokenInfo.BaseDenom,
			OriginChain: tokenInfo.OriginChain,
			Decimals:    int32(tokenInfo.Decimals),
		}
	}
	return protoTokenInfos
}
