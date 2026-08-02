package rpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"connectrpc.com/connect"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/models"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router"
	v1 "github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/v1"
	v1connect "github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/v1/v1connect"
	"google.golang.org/protobuf/types/known/emptypb"
)

// PathfinderServer implements the ConnectRPC PathfinderServiceHandler interface
type PathfinderServer struct {
	pathfinder    *router.Pathfinder
	denomResolver *router.DenomResolver
}

// Verify that PathfinderServer implements the interface
var _ v1connect.PathfinderServiceHandler = (*PathfinderServer)(nil)
var _ v1connect.PathfinderStreamingSerivceHandler = (*PathfinderServer)(nil)

// NewPathfinderServer creates a new PathfinderServer
func NewPathfinderServer(pathfinder *router.Pathfinder, denomResolver *router.DenomResolver) *PathfinderServer {
	return &PathfinderServer{
		pathfinder:    pathfinder,
		denomResolver: denomResolver,
	}
}

// findPathInternal resolves denoms and runs the pathfinder for a given request.
// Shared by FindPath (unary) and FindPathStream (bidi streaming).
func (s *PathfinderServer) findPathInternal(req *v1.FindPathRequest) (*v1.FindPathResponse, error) {
	if err := s.validateFindPathRequest(req); err != nil {
		return nil, err
	}

	resolvedFromDenom, err := s.denomResolver.ResolveToChainDenom(req.ChainFrom, req.TokenFromDenom)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("could not resolve source token '%s' on chain '%s': %w",
				req.TokenFromDenom, req.ChainFrom, err))
	}

	var resolvedToDenom string
	if req.TokenToDenom == "" {
		resolvedToDenom, err = s.denomResolver.InferTokenToDenom(req.ChainFrom, resolvedFromDenom, req.ChainTo)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("could not infer destination token: %w", err))
		}
	} else {
		resolvedToDenom, err = s.denomResolver.ResolveToChainDenom(req.ChainTo, req.TokenToDenom)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("could not resolve destination token '%s' on chain '%s': %w",
					req.TokenToDenom, req.ChainTo, err))
		}
	}

	internalReq := models.RouteRequest{
		ChainFrom:       req.ChainFrom,
		TokenFromDenom:  resolvedFromDenom,
		AmountIn:        req.AmountIn,
		ChainTo:         req.ChainTo,
		TokenToDenom:    resolvedToDenom,
		SenderAddress:   req.SenderAddress,
		ReceiverAddress: req.ReceiverAddress,
		SmartRoute:      &req.SmartRoute,
		SlippageBps:     &req.SlippageBps,
	}

	internalResp := s.pathfinder.FindPath(internalReq)
	return convertToProtoResponse(&internalResp), nil
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
	Logger.Info().Msgf("Request data for find path; %+v", req.Msg)

	protoResp, err := s.findPathInternal(req.Msg)
	if err != nil {
		return nil, err
	}
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
		//nolint:nilerr // unresolved denom is a valid "not found" response, not an error
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

// ListSupportedChains returns the list of all supported chains
//
// Returns:
// - []string: the list of all chain ids
func (s *PathfinderServer) ListSupportedChains(
	ctx context.Context,
	req *connect.Request[emptypb.Empty],
) (*connect.Response[v1.PathfinderSupportedChainsResponse], error) {
	chains := s.pathfinder.GetAllChains()
	return connect.NewResponse(&v1.PathfinderSupportedChainsResponse{
		ChainIds: chains,
	}), nil
}

// FindPathStream implements a bidirectional streaming handler for path finding.
//
// Behavior:
//   - Client sends a FindPathRequest → server immediately computes and streams back the result.
//   - If the client goes silent for 15 seconds, the server re-runs the last request and pushes
//     fresh data automatically so the client never needs to poll.
//   - The 15-second countdown resets each time the client sends a new request.
//   - Stream ends when the client closes their side (EOF) or the context is cancelled.
//   - If no first request arrives within 60 seconds, the stream is closed to prevent
//     goroutine accumulation from clients that open a stream but never send.
//   - Streams are hard-capped at 1 hour regardless of client behaviour; the client
//     can reconnect and send a new request to continue.
func (s *PathfinderServer) FindPathStream(
	ctx context.Context,
	stream *connect.BidiStream[v1.FindPathRequest, v1.FindPathResponse],
) error {
	const (
		idleRefreshInterval = 15 * time.Second
		firstMsgTimeout     = 60 * time.Second
		maxStreamLifetime   = 1 * time.Hour
	)

	type receiveResult struct {
		req *v1.FindPathRequest
		err error
	}

	recvCh := make(chan receiveResult, 1)

	// Read incoming messages in a separate goroutine so we can select on it.
	go func() {
		for {
			req, err := stream.Receive()
			recvCh <- receiveResult{req, err}
			if err != nil {
				return
			}
		}
	}()

	// firstMsgTimer fires if the client never sends an initial request.
	// Stopped and replaced by the refresh ticker once the first message arrives.
	firstMsgTimer := time.NewTimer(firstMsgTimeout)
	defer firstMsgTimer.Stop()

	// lifetimeTimer is a hard cap on how long any stream can run, regardless of client behaviour.
	lifetimeTimer := time.NewTimer(maxStreamLifetime)
	defer lifetimeTimer.Stop()

	var (
		lastReq *v1.FindPathRequest
		ticker  *time.Ticker
		tickerC <-chan time.Time // nil until first message received; nil channel blocks in select
	)

	sendResponse := func(req *v1.FindPathRequest) error {
		resp, err := s.findPathInternal(req)
		if err != nil {
			return err
		}
		return stream.Send(resp)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-firstMsgTimer.C:
			return connect.NewError(connect.CodeDeadlineExceeded,
				fmt.Errorf("stream closed: no request received within %s", firstMsgTimeout))

		case <-lifetimeTimer.C:
			return connect.NewError(connect.CodeResourceExhausted,
				fmt.Errorf("stream closed: maximum lifetime of %s reached, reconnect to continue", maxStreamLifetime))

		case result := <-recvCh:
			if result.err != nil {
				if errors.Is(result.err, io.EOF) {
					return nil
				}
				return result.err
			}
			Logger.Info().Msgf("Stream request received; %+v", result.req)
			if lastReq == nil {
				// First message — stop the first-message timer and start the refresh ticker.
				firstMsgTimer.Stop()
				ticker = time.NewTicker(idleRefreshInterval)
				defer ticker.Stop()
				tickerC = ticker.C
			} else {
				ticker.Reset(idleRefreshInterval)
			}
			lastReq = result.req
			if err := sendResponse(lastReq); err != nil {
				return err
			}

		case <-tickerC:
			Logger.Info().Msg("Auto-refreshing stream path response after idle timeout")
			if err := sendResponse(lastReq); err != nil {
				return err
			}
		}
	}
}
