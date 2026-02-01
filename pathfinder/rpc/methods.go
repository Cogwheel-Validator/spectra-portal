package rpc

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/models"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/router"
	v1 "github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/rpc/v1"
	v1connect "github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/rpc/v1/v1connect"
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

	Logger.Info().Msgf(
		"Request data for find path; %+v",
		req.Msg,
	)

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
		SmartRoute:      &req.Msg.SmartRoute,
		SlippageBps:     &req.Msg.SlippageBps,
	}

	// Step 4: Call pathfinder with resolved denoms
	internalResp := s.pathfinder.FindPath(internalReq)

	// Step 5: Convert to proto response
	// Note: "No route found" returns 200 with success=false (valid query, valid answer)
	protoResp := convertToProtoResponse(&internalResp)

	return connect.NewResponse(protoResp), nil
}

// validateFindPathRequest validates the request parameters
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

	Logger.Info().Msgf(
		"Request data for lookup denom; %+v",
		req.Msg,
	)
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

	Logger.Info().Msgf(
		"Request data for get token denoms; %+v",
		req.Msg,
	)

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

	Logger.Info().Msgf(
		"Request data for get chain tokens; %+v",
		req.Msg,
	)

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

	Logger.Info().Msgf(
		"Request data for get chain info; %+v",
		req.Msg,
	)

	chain, err := s.pathfinder.GetChainInfo(req.Msg.ChainId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	return connect.NewResponse(&v1.ChainInfoResponse{
		ChainInfo: convertToProtoChainInfo(&chain, &req.Msg.ShowSymbols),
	}), nil
}

/*
GetPathfinderSupportedChains returns the list of all supported chains

Returns:
- []string: the list of all chain ids
*/
func (s *PathfinderServer) ListSupportedChains(
	ctx context.Context,
	req *connect.Request[emptypb.Empty],
) (*connect.Response[v1.PathfinderSupportedChainsResponse], error) {
	chains := s.pathfinder.GetAllChains()
	return connect.NewResponse(&v1.PathfinderSupportedChainsResponse{
		ChainIds: chains,
	}), nil
}
