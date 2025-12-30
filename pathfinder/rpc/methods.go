package rpc

import (
	"context"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/models"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/router"
	v1 "github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/rpc/v1"
	v1connect "github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/rpc/v1/v1connect"
	"github.com/btcsuite/btcutil/bech32"
	"google.golang.org/protobuf/types/known/emptypb"
)

// PathfinderServer implements the ConnectRPC PathfinderServiceHandler interface
type PathfinderServer struct {
	pathfinder    *router.Pathfinder
	denomResolver *router.DenomResolver
}

// Verify that PathfinderServer implements the interface
var _ v1connect.PathfinderServiceHandler = (*PathfinderServer)(nil)

// NewPathfinderServer creates a new PathfinderServer
func NewPathfinderServer(pathfinder *router.Pathfinder, denomResolver *router.DenomResolver) *PathfinderServer {
	return &PathfinderServer{
		pathfinder:    pathfinder,
		denomResolver: denomResolver,
	}
}

// FindPath implements the ConnectRPC handler for finding paths.
// Supports:
// - Human-readable denoms (e.g., "uatone") which are resolved automatically
// - Empty token_to_denom which defaults to the same token (bridging without swap)
//
// Returns:
// - 400 Bad Request: Invalid input (bad address format, unknown chain, etc.)
// - 200 OK with success=false: Valid query but no route exists
// - 200 OK with success=true: Route found
func (s *PathfinderServer) FindPath(
	ctx context.Context,
	req *connect.Request[v1.FindPathRequest],
) (*connect.Response[v1.FindPathResponse], error) {
	// Step 0: Validate input parameters (returns 400 for validation errors)
	if err := s.validateFindPathRequest(req.Msg); err != nil {
		return nil, err
	}

	// Step 1: Resolve token_from_denom (could be human-readable)
	resolvedFromDenom, err := s.denomResolver.ResolveToChainDenom(req.Msg.ChainFrom, req.Msg.TokenFromDenom)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("could not resolve source token '%s' on chain '%s': %w",
				req.Msg.TokenFromDenom, req.Msg.ChainFrom, err))
	}

	// Step 2: Resolve token_to_denom (could be empty or human-readable)
	var resolvedToDenom string
	if req.Msg.TokenToDenom == "" {
		// Empty → infer same token on destination chain
		resolvedToDenom, err = s.denomResolver.InferTokenToDenom(
			req.Msg.ChainFrom,
			resolvedFromDenom,
			req.Msg.ChainTo,
		)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("could not infer destination token: %w", err))
		}
	} else {
		// Resolve human-readable denom if needed
		resolvedToDenom, err = s.denomResolver.ResolveToChainDenom(req.Msg.ChainTo, req.Msg.TokenToDenom)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("could not resolve destination token '%s' on chain '%s': %w",
					req.Msg.TokenToDenom, req.Msg.ChainTo, err))
		}
	}

	// Step 3: Build internal request with resolved denoms
	internalReq := models.RouteRequest{
		ChainFrom:       req.Msg.ChainFrom,
		TokenFromDenom:  resolvedFromDenom,
		AmountIn:        req.Msg.AmountIn,
		ChainTo:         req.Msg.ChainTo,
		TokenToDenom:    resolvedToDenom,
		SenderAddress:   req.Msg.SenderAddress,
		ReceiverAddress: req.Msg.ReceiverAddress,
	}

	// Step 4: Call solver with resolved denoms
	internalResp := s.pathfinder.FindPath(internalReq)

	// Step 5: Convert to proto response
	// Note: "No route found" returns 200 with success=false (valid query, valid answer)
	protoResp := convertToProtoResponse(&internalResp)

	return connect.NewResponse(protoResp), nil
}

// validateSolveRouteRequest validates the request parameters
// Returns a ConnectRPC error (which translates to HTTP 400) for invalid input
func (s *PathfinderServer) validateFindPathRequest(req *v1.FindPathRequest) error {
	// Validate chain IDs exist and get chain info for prefix validation
	sourceChain, err := s.pathfinder.GetChainInfo(req.ChainFrom)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("unknown source chain: %s", req.ChainFrom))
	}
	destChain, err := s.pathfinder.GetChainInfo(req.ChainTo)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("unknown destination chain: %s", req.ChainTo))
	}

	// Validate sender address format (must be valid bech32)
	senderPrefix, err := validateBech32Address(req.SenderAddress)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid sender address '%s': %w", req.SenderAddress, err))
	}

	// Validate sender address prefix matches source chain
	if sourceChain.Bech32Prefix != "" && senderPrefix != sourceChain.Bech32Prefix {
		return connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("sender address prefix '%s' does not match source chain '%s' (expected prefix: %s)",
				senderPrefix, req.ChainFrom, sourceChain.Bech32Prefix))
	}

	// Validate receiver address format (must be valid bech32)
	receiverPrefix, err := validateBech32Address(req.ReceiverAddress)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid receiver address '%s': %w", req.ReceiverAddress, err))
	}

	// Validate receiver address prefix matches destination chain
	if destChain.Bech32Prefix != "" && receiverPrefix != destChain.Bech32Prefix {
		return connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("receiver address prefix '%s' does not match destination chain '%s' (expected prefix: %s)",
				receiverPrefix, req.ChainTo, destChain.Bech32Prefix))
	}

	// Validate amount is positive
	if req.AmountIn == "" || req.AmountIn == "0" {
		return connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("amount_in must be a positive number"))
	}

	return nil
}

// LookupDenom implements the ConnectRPC handler for denom lookup.
// Accepts human-readable denoms (e.g., "uatone") or IBC denoms.
// Returns the token info plus all chains where this token is available.
func (s *PathfinderServer) LookupDenom(
	ctx context.Context,
	req *connect.Request[v1.LookupDenomRequest],
) (*connect.Response[v1.LookupDenomResponse], error) {
	denomInfo, err := s.denomResolver.ResolveDenom(req.Msg.ChainId, req.Msg.Denom)
	if err != nil {
		return connect.NewResponse(&v1.LookupDenomResponse{
			Found: false,
		}), nil
	}

	// Get where else this token is available
	availableOn := s.denomResolver.GetAvailableOn(denomInfo.BaseDenom, denomInfo.OriginChain)
	protoAvailableOn := make([]*v1.ChainDenom, len(availableOn))
	for i, cd := range availableOn {
		protoAvailableOn[i] = &v1.ChainDenom{
			ChainId:   cd.ChainID,
			ChainName: cd.ChainName,
			Denom:     cd.Denom,
			IsNative:  cd.IsNative,
		}
	}

	return connect.NewResponse(&v1.LookupDenomResponse{
		Found:       true,
		ChainDenom:  denomInfo.ChainDenom,
		BaseDenom:   denomInfo.BaseDenom,
		OriginChain: denomInfo.OriginChain,
		IsNative:    denomInfo.IsNative,
		IbcPath:     denomInfo.IbcPath,
		AvailableOn: protoAvailableOn,
	}), nil
}

// GetTokenDenoms returns all denoms for a token across supported chains.
// Use this to discover what denom a token has on different chains.
func (s *PathfinderServer) GetTokenDenoms(
	ctx context.Context,
	req *connect.Request[v1.GetTokenDenomsRequest],
) (*connect.Response[v1.GetTokenDenomsResponse], error) {
	denoms, found := s.denomResolver.GetTokenDenomsAcrossChains(
		req.Msg.BaseDenom,
		req.Msg.OriginChain,
		req.Msg.OnChainId, // Optional filter
	)

	if !found {
		return connect.NewResponse(&v1.GetTokenDenomsResponse{
			Found:       false,
			BaseDenom:   req.Msg.BaseDenom,
			OriginChain: req.Msg.OriginChain,
		}), nil
	}

	protoDenoms := make([]*v1.ChainDenom, len(denoms))
	for i, cd := range denoms {
		protoDenoms[i] = &v1.ChainDenom{
			ChainId:   cd.ChainID,
			ChainName: cd.ChainName,
			Denom:     cd.Denom,
			IsNative:  cd.IsNative,
		}
	}

	return connect.NewResponse(&v1.GetTokenDenomsResponse{
		Found:       true,
		BaseDenom:   req.Msg.BaseDenom,
		OriginChain: req.Msg.OriginChain,
		Denoms:      protoDenoms,
	}), nil
}

// GetChainTokens returns all tokens available on a specific chain.
// Includes both native tokens and IBC tokens with their denoms.
func (s *PathfinderServer) GetChainTokens(
	ctx context.Context,
	req *connect.Request[v1.GetChainTokensRequest],
) (*connect.Response[v1.GetChainTokensResponse], error) {
	tokens, err := s.denomResolver.GetChainTokens(req.Msg.ChainId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	nativeTokens := make([]*v1.TokenDetails, len(tokens.NativeTokens))
	for i, t := range tokens.NativeTokens {
		nativeTokens[i] = &v1.TokenDetails{
			Denom:       t.Denom,
			Symbol:      t.Symbol,
			BaseDenom:   t.BaseDenom,
			OriginChain: t.OriginChain,
			Decimals:    int32(t.Decimals),
			IsNative:    t.IsNative,
		}
	}

	ibcTokens := make([]*v1.TokenDetails, len(tokens.IBCTokens))
	for i, t := range tokens.IBCTokens {
		ibcTokens[i] = &v1.TokenDetails{
			Denom:       t.Denom,
			Symbol:      t.Symbol,
			BaseDenom:   t.BaseDenom,
			OriginChain: t.OriginChain,
			Decimals:    int32(t.Decimals),
			IsNative:    t.IsNative,
		}
	}

	return connect.NewResponse(&v1.GetChainTokensResponse{
		ChainId:      tokens.ChainID,
		ChainName:    tokens.ChainName,
		NativeTokens: nativeTokens,
		IbcTokens:    ibcTokens,
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
func (s *PathfinderServer) GetChainInfo(
	ctx context.Context,
	req *connect.Request[v1.ChainInfoRequest],
) (*connect.Response[v1.ChainInfoResponse], error) {
	chain, err := s.pathfinder.GetChainInfo(req.Msg.ChainId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	return connect.NewResponse(&v1.ChainInfoResponse{
		ChainInfo: convertToProtoChainInfo(&chain, &req.Msg.SortBySymbol),
	}), nil
}

/*
GetPathfinderSupportedChains returns the list of all supported chains

Returns:
- []string: the list of all chain ids
*/
func (s *PathfinderServer) GetPathfinderSupportedChains(
	ctx context.Context,
	req *connect.Request[emptypb.Empty],
) (*connect.Response[v1.PathfinderSupportedChainsResponse], error) {
	chains := s.pathfinder.GetAllChains()
	return connect.NewResponse(&v1.PathfinderSupportedChainsResponse{
		ChainIds: chains,
	}), nil
}

// CONVERT FUNCTIONS
// These convert between internal models and protobuf types

/*
Converts internal models.RouteResponse to v1.FindPathResponse
It uses the protobuf oneof to represent the different route types.

Parameters:
- resp: *models.RouteResponse

Returns:
- *v1.FindPathResponse

Errors:
- None
*/
func convertToProtoResponse(resp *models.RouteResponse) *v1.FindPathResponse {
	protoResp := &v1.FindPathResponse{
		Success:      resp.Success,
		ErrorMessage: resp.ErrorMessage,
	}

	// Convert Direct route if present (using protobuf oneof)
	if resp.Direct != nil {
		protoResp.Route = &v1.FindPathResponse_Direct{
			Direct: convertToProtoDirectRoute(resp.Direct),
		}
	}

	// Convert Indirect route if present (using protobuf oneof)
	if resp.Indirect != nil {
		protoResp.Route = &v1.FindPathResponse_Indirect{
			Indirect: convertToProtoIndirectRoute(resp.Indirect),
		}
	}

	// Convert BrokerSwap route if present (using protobuf oneof)
	if resp.BrokerSwap != nil {
		protoResp.Route = &v1.FindPathResponse_BrokerSwap{
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
	// Convert outbound legs
	outboundLegs := make([]*v1.IBCLeg, len(brokerSwap.OutboundLegs))
	for i, leg := range brokerSwap.OutboundLegs {
		outboundLegs[i] = convertToProtoIBCLeg(leg)
	}

	result := &v1.BrokerSwapRoute{
		Path:                brokerSwap.Path,
		InboundLeg:          convertToProtoIBCLeg(brokerSwap.InboundLeg),
		Swap:                convertToProtoSwapQuote(brokerSwap.Swap),
		OutboundLegs:        outboundLegs,
		OutboundSupportsPfm: brokerSwap.OutboundSupportsPFM,
	}

	// Add execution data if available
	if brokerSwap.Execution != nil {
		result.Execution = &v1.BrokerExecutionData{
			Memo:            brokerSwap.Execution.Memo,
			IbcReceiver:     brokerSwap.Execution.IBCReceiver,
			RecoverAddress:  brokerSwap.Execution.RecoverAddress,
			MinOutputAmount: brokerSwap.Execution.MinOutputAmount,
			UsesWasm:        brokerSwap.Execution.UsesWasm,
			Description:     brokerSwap.Execution.Description,
		}
	}

	return result
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

func convertToProtoChainInfo(chain *router.PathfinderChain, sortBySymbol *bool) *v1.ChainInfo {
	return &v1.ChainInfo{
		ChainId:   chain.Id,
		ChainName: chain.Name,
		HasPfm:    chain.HasPFM,
		IsBroker:  chain.Broker,
		Routes:    convertToProtoBasicRoute(chain.Routes, sortBySymbol),
	}
}

func convertToProtoBasicRoute(routes []router.BasicRoute, sortBySymbol *bool) []*v1.BasicRoute {
	protoRoutes := make([]*v1.BasicRoute, len(routes))
	for i := range routes {
		protoRoutes[i] = &v1.BasicRoute{
			ToChain:       routes[i].ToChain,
			ToChainId:     routes[i].ToChainId,
			ConnectionId:  routes[i].ConnectionId,
			ChannelId:     routes[i].ChannelId,
			PortId:        routes[i].PortId,
			AllowedTokens: convertToProtoTokenInfo(routes[i].AllowedTokens, sortBySymbol),
		}
	}
	return protoRoutes
}

func convertToProtoTokenInfo(tokenInfo map[string]router.TokenInfo, sortBySymbol *bool) map[string]*v1.TokenInfo {
	protoTokenInfos := make(map[string]*v1.TokenInfo, len(tokenInfo))
	if *sortBySymbol {
		for _, tokenInfo := range tokenInfo {
			protoTokenInfos[tokenInfo.Symbol+"@"+tokenInfo.OriginChain] = &v1.TokenInfo{
				ChainDenom:  tokenInfo.ChainDenom,
				IbcDenom:    tokenInfo.IbcDenom,
				BaseDenom:   tokenInfo.BaseDenom,
				OriginChain: tokenInfo.OriginChain,
				Decimals:    int32(tokenInfo.Decimals),
				Symbol:      tokenInfo.Symbol,
			}
		}
	} else {
		for denom, tokenInfo := range tokenInfo {
			protoTokenInfos[denom] = &v1.TokenInfo{
				ChainDenom:  tokenInfo.ChainDenom,
				IbcDenom:    tokenInfo.IbcDenom,
				BaseDenom:   tokenInfo.BaseDenom,
				OriginChain: tokenInfo.OriginChain,
				Decimals:    int32(tokenInfo.Decimals),
				Symbol:      tokenInfo.Symbol,
			}
		}
	}
	return protoTokenInfos
}

// validateBech32Address validates that an address is a valid bech32 address
// Returns the prefix if valid, or an error if invalid
func validateBech32Address(address string) (string, error) {
	if address == "" {
		return "", fmt.Errorf("address is empty")
	}

	// Check minimum length (prefix + "1" + data + checksum)
	if len(address) < 10 {
		return "", fmt.Errorf("address too short (minimum 10 characters)")
	}

	// Check for separator
	sepIdx := strings.LastIndex(address, "1")
	if sepIdx < 1 {
		return "", fmt.Errorf("missing bech32 separator '1'")
	}

	// Validate the prefix (human-readable part)
	prefix := address[:sepIdx]
	if prefix == "" {
		return "", fmt.Errorf("empty bech32 prefix")
	}

	// Try to decode as bech32 - this validates the checksum
	decodedPrefix, data, err := bech32.Decode(address)
	if err != nil {
		return "", fmt.Errorf("invalid bech32 address (checksum failed): %w", err)
	}

	// Verify decoded prefix matches
	if decodedPrefix != prefix {
		return "", fmt.Errorf("bech32 prefix mismatch")
	}

	// Verify data is not empty (should be 20 or 32 bytes for cosmos addresses)
	if len(data) == 0 {
		return "", fmt.Errorf("empty address data")
	}

	return prefix, nil
}
