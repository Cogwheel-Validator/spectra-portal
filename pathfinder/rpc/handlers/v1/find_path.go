// Package v1handlers implements the pathfinder.v1 ConnectRPC services.
//
// v1 keeps the legacy single sender/receiver address pair: addresses for the
// broker and intermediate chains are derived via slip-44-guarded bech32
// conversion (RouteRequest.DeriveMissing). It is kept unchanged until it can
// be properly decommissioned; new consumers should use v2beta.
package v1handlers

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/models"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/handlers/common"
	v1 "github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/services/pathfinder/v1"
	v1connect "github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/services/pathfinder/v1/v1connect"
)

// Server implements the ConnectRPC PathfinderServiceHandler interface
type Server struct {
	common.Deps
}

// Verify that Server implements the interface
var _ v1connect.PathfinderServiceHandler = (*Server)(nil)

// NewServer creates a new v1 handler server
func NewServer(deps common.Deps) *Server {
	return &Server{Deps: deps}
}

// findPathInternal resolves denoms and runs the pathfinder for a given request.
func (s *Server) findPathInternal(req *v1.FindPathRequest) (*v1.FindPathResponse, error) {
	if err := s.validateFindPathRequest(req); err != nil {
		return nil, err
	}

	resolvedFromDenom, resolvedToDenom, err := s.ResolveDenoms(
		req.ChainFrom, req.TokenFromDenom, req.ChainTo, req.TokenToDenom)
	if err != nil {
		return nil, err
	}

	internalReq := models.RouteRequest{
		ChainFrom:      req.ChainFrom,
		TokenFromDenom: resolvedFromDenom,
		AmountIn:       req.AmountIn,
		ChainTo:        req.ChainTo,
		TokenToDenom:   resolvedToDenom,
		Addresses: map[string]string{
			req.ChainFrom: req.SenderAddress,
			req.ChainTo:   req.ReceiverAddress,
		},
		// Legacy v1 behavior: derive broker/intermediate addresses from the
		// sender/receiver pair via slip-44-guarded conversion.
		DeriveMissing: true,
		SmartRoute:    &req.SmartRoute,
		SlippageBps:   &req.SlippageBps,
	}

	internalResp := s.Pathfinder.FindPath(internalReq)
	if len(internalResp.MissingAddressChains) > 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("%s", internalResp.ErrorMessage))
	}
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
func (s *Server) FindPath(
	ctx context.Context,
	req *connect.Request[v1.FindPathRequest],
) (*connect.Response[v1.FindPathResponse], error) {
	s.Logger.Info().Msgf("Request data for find path; %+v", req.Msg)

	protoResp, err := s.findPathInternal(req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(protoResp), nil
}

// validateFindPathRequest validates the request parameters
// Returns a ConnectRPC error (which translates to HTTP 400) for invalid input
func (s *Server) validateFindPathRequest(req *v1.FindPathRequest) error {
	// Validate chain IDs exist and get chain info for prefix validation
	sourceChain, err := s.Pathfinder.GetChainInfo(req.ChainFrom)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("unknown source chain: %s", req.ChainFrom))
	}
	destChain, err := s.Pathfinder.GetChainInfo(req.ChainTo)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("unknown destination chain: %s", req.ChainTo))
	}

	// Validate sender address format (must be valid bech32)
	senderPrefix, err := common.ValidateBech32Address(req.SenderAddress)
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
	receiverPrefix, err := common.ValidateBech32Address(req.ReceiverAddress)
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
