package v1handlers

import (
	"context"

	"connectrpc.com/connect"
	v1 "github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/services/pathfinder/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

// LookupDenom implements the ConnectRPC handler for denom lookup.
// Accepts human-readable denoms (e.g., "uatone") or IBC denoms.
// Returns the token info plus all chains where this token is available.
func (s *Server) LookupDenom(
	ctx context.Context,
	req *connect.Request[v1.LookupDenomRequest],
) (*connect.Response[v1.LookupDenomResponse], error) {
	denomInfo, err := s.DenomResolver.ResolveDenom(req.Msg.ChainId, req.Msg.Denom)

	s.Logger.Info().Msgf(
		"Request data for lookup denom; %+v",
		req.Msg,
	)
	if err != nil {
		//nolint:nilerr // unresolved denom is a valid "not found" response, not an error
		return connect.NewResponse(&v1.LookupDenomResponse{
			Found: false,
		}), nil
	}

	// Get where else this token is available
	availableOn := s.DenomResolver.GetAvailableOn(denomInfo.BaseDenom, denomInfo.OriginChain)
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
func (s *Server) GetTokenDenoms(
	ctx context.Context,
	req *connect.Request[v1.GetTokenDenomsRequest],
) (*connect.Response[v1.GetTokenDenomsResponse], error) {

	s.Logger.Info().Msgf(
		"Request data for get token denoms; %+v",
		req.Msg,
	)

	denoms, found := s.DenomResolver.GetTokenDenomsAcrossChains(
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
func (s *Server) GetChainTokens(
	ctx context.Context,
	req *connect.Request[v1.GetChainTokensRequest],
) (*connect.Response[v1.GetChainTokensResponse], error) {

	s.Logger.Info().Msgf(
		"Request data for get chain tokens; %+v",
		req.Msg,
	)

	tokens, err := s.DenomResolver.GetChainTokens(req.Msg.ChainId)
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
			Decimals:    int32(t.Decimals), //nolint:gosec // G115: Decimals is chain metadata, always within int32 range
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
			Decimals:    int32(t.Decimals), //nolint:gosec // G115: Decimals is chain metadata, always within int32 range
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
func (s *Server) GetChainInfo(
	ctx context.Context,
	req *connect.Request[v1.ChainInfoRequest],
) (*connect.Response[v1.ChainInfoResponse], error) {

	s.Logger.Info().Msgf(
		"Request data for get chain info; %+v",
		req.Msg,
	)

	chain, err := s.Pathfinder.GetChainInfo(req.Msg.ChainId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	return connect.NewResponse(&v1.ChainInfoResponse{
		ChainInfo: convertToProtoChainInfo(&chain, &req.Msg.ShowSymbols),
	}), nil
}

// ListSupportedChains returns the list of all supported chains
//
// Returns:
// - []string: the list of all chain ids
func (s *Server) ListSupportedChains(
	ctx context.Context,
	req *connect.Request[emptypb.Empty],
) (*connect.Response[v1.PathfinderSupportedChainsResponse], error) {
	chains := s.Pathfinder.GetAllChains()
	return connect.NewResponse(&v1.PathfinderSupportedChainsResponse{
		ChainIds: chains,
	}), nil
}
