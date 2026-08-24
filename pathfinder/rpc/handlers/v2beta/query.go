package v2betahandlers

import (
	"context"

	"connectrpc.com/connect"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/handlers/common"
	v2beta "github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/services/pathfinder/v2beta"
	v2betaconnect "github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/services/pathfinder/v2beta/v2betaconnect"
	"google.golang.org/protobuf/types/known/emptypb"
)

// QueryServer implements the ConnectRPC PathfinderQueryServiceHandler
// interface. The query methods mirror the v1 ones; they were split into
// their own service so route-finding can evolve without touching them.
type QueryServer struct {
	common.Deps
}

// Verify that QueryServer implements the interface
var _ v2betaconnect.PathfinderQueryServiceHandler = (*QueryServer)(nil)

// NewQueryServer creates a new v2beta query handler server
func NewQueryServer(deps common.Deps) *QueryServer {
	return &QueryServer{Deps: deps}
}

// LookupDenom implements the ConnectRPC handler for denom lookup.
// Accepts human-readable denoms (e.g., "uatone") or IBC denoms.
// Returns the token info plus all chains where this token is available.
func (s *QueryServer) LookupDenom(
	ctx context.Context,
	req *connect.Request[v2beta.LookupDenomRequest],
) (*connect.Response[v2beta.LookupDenomResponse], error) {
	denomInfo, err := s.DenomResolver.ResolveDenom(req.Msg.ChainId, req.Msg.Denom)

	s.Logger.Info().Msgf(
		"Request data for lookup denom; %+v",
		req.Msg,
	)
	if err != nil {
		//nolint:nilerr // unresolved denom is a valid "not found" response, not an error
		return connect.NewResponse(&v2beta.LookupDenomResponse{
			Found: false,
		}), nil
	}

	// Get where else this token is available
	availableOn := s.DenomResolver.GetAvailableOn(denomInfo.BaseDenom, denomInfo.OriginChain)
	protoAvailableOn := make([]*v2beta.ChainDenom, len(availableOn))
	for i, cd := range availableOn {
		protoAvailableOn[i] = &v2beta.ChainDenom{
			ChainId:   cd.ChainID,
			ChainName: cd.ChainName,
			Denom:     cd.Denom,
			IsNative:  cd.IsNative,
		}
	}

	return connect.NewResponse(&v2beta.LookupDenomResponse{
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
func (s *QueryServer) GetTokenDenoms(
	ctx context.Context,
	req *connect.Request[v2beta.GetTokenDenomsRequest],
) (*connect.Response[v2beta.GetTokenDenomsResponse], error) {

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
		return connect.NewResponse(&v2beta.GetTokenDenomsResponse{
			Found:       false,
			BaseDenom:   req.Msg.BaseDenom,
			OriginChain: req.Msg.OriginChain,
		}), nil
	}

	protoDenoms := make([]*v2beta.ChainDenom, len(denoms))
	for i, cd := range denoms {
		protoDenoms[i] = &v2beta.ChainDenom{
			ChainId:   cd.ChainID,
			ChainName: cd.ChainName,
			Denom:     cd.Denom,
			IsNative:  cd.IsNative,
		}
	}

	return connect.NewResponse(&v2beta.GetTokenDenomsResponse{
		Found:       true,
		BaseDenom:   req.Msg.BaseDenom,
		OriginChain: req.Msg.OriginChain,
		Denoms:      protoDenoms,
	}), nil
}

// GetChainTokens returns all tokens available on a specific chain.
// Includes both native tokens and IBC tokens with their denoms.
func (s *QueryServer) GetChainTokens(
	ctx context.Context,
	req *connect.Request[v2beta.GetChainTokensRequest],
) (*connect.Response[v2beta.GetChainTokensResponse], error) {

	s.Logger.Info().Msgf(
		"Request data for get chain tokens; %+v",
		req.Msg,
	)

	tokens, err := s.DenomResolver.GetChainTokens(req.Msg.ChainId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	nativeTokens := make([]*v2beta.TokenDetails, len(tokens.NativeTokens))
	for i, t := range tokens.NativeTokens {
		nativeTokens[i] = &v2beta.TokenDetails{
			Denom:       t.Denom,
			Symbol:      t.Symbol,
			BaseDenom:   t.BaseDenom,
			OriginChain: t.OriginChain,
			Decimals:    int32(t.Decimals), //nolint:gosec // G115: Decimals is chain metadata, always within int32 range
			IsNative:    t.IsNative,
		}
	}

	ibcTokens := make([]*v2beta.TokenDetails, len(tokens.IBCTokens))
	for i, t := range tokens.IBCTokens {
		ibcTokens[i] = &v2beta.TokenDetails{
			Denom:       t.Denom,
			Symbol:      t.Symbol,
			BaseDenom:   t.BaseDenom,
			OriginChain: t.OriginChain,
			Decimals:    int32(t.Decimals), //nolint:gosec // G115: Decimals is chain metadata, always within int32 range
			IsNative:    t.IsNative,
		}
	}

	return connect.NewResponse(&v2beta.GetChainTokensResponse{
		ChainId:      tokens.ChainID,
		ChainName:    tokens.ChainName,
		NativeTokens: nativeTokens,
		IbcTokens:    ibcTokens,
	}), nil
}

// GetChainInfo returns the information about a specific chain.
// Returns CodeNotFound if the chain is not supported.
func (s *QueryServer) GetChainInfo(
	ctx context.Context,
	req *connect.Request[v2beta.GetChainInfoRequest],
) (*connect.Response[v2beta.GetChainInfoResponse], error) {

	s.Logger.Info().Msgf(
		"Request data for get chain info; %+v",
		req.Msg,
	)

	chain, err := s.Pathfinder.GetChainInfo(req.Msg.ChainId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	return connect.NewResponse(&v2beta.GetChainInfoResponse{
		ChainInfo: convertToProtoChainInfo(&chain, &req.Msg.ShowSymbols),
	}), nil
}

// ListSupportedChains returns the list of all supported chain ids
func (s *QueryServer) ListSupportedChains(
	ctx context.Context,
	req *connect.Request[emptypb.Empty],
) (*connect.Response[v2beta.ListSupportedChainsResponse], error) {
	chains := s.Pathfinder.GetAllChains()
	return connect.NewResponse(&v2beta.ListSupportedChainsResponse{
		ChainIds: chains,
	}), nil
}
