// Package v2betahandlers implements the pathfinder.v2beta ConnectRPC services.
//
// Unlike v1, FindPath takes an explicit address per chain (no slip-44-bound
// derivation). An empty addresses array on the unary FindPath is a read-only
// discovery request: the route is computed with placeholder addresses, the
// response is marked RESPONSE_CODE_MOCK_ADDRESSES, execution data and memos
// are omitted, and required_chains lists what a real request must supply.
// The streaming variant always requires real addresses.
package v2betahandlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"connectrpc.com/connect"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/models"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/handlers/common"
	v2beta "github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/services/pathfinder/v2beta"
	v2betaconnect "github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/services/pathfinder/v2beta/v2betaconnect"
)

// FindPathServer implements the ConnectRPC FindPathServiceHandler interface
type FindPathServer struct {
	common.Deps
}

// Verify that FindPathServer implements the interface
var _ v2betaconnect.FindPathServiceHandler = (*FindPathServer)(nil)

// NewFindPathServer creates a new v2beta find-path handler server
func NewFindPathServer(deps common.Deps) *FindPathServer {
	return &FindPathServer{Deps: deps}
}

// findPathParams is the transport-neutral projection of the unary and
// streaming request types, which carry identical fields.
type findPathParams struct {
	ChainFrom      string
	TokenFromDenom string
	AmountIn       string
	ChainTo        string
	TokenToDenom   string
	Addresses      []*v2beta.ChainAddress
	SmartRoute     bool
	SlippageBps    uint32
}

func paramsFromFindPathRequest(req *v2beta.FindPathRequest) findPathParams {
	return findPathParams{
		ChainFrom:      req.ChainFrom,
		TokenFromDenom: req.TokenFromDenom,
		AmountIn:       req.AmountIn,
		ChainTo:        req.ChainTo,
		TokenToDenom:   req.TokenToDenom,
		Addresses:      req.Addresses,
		SmartRoute:     req.SmartRoute,
		SlippageBps:    req.SlippageBps,
	}
}

func paramsFromStreamingRequest(req *v2beta.FindPathStreamingRequest) findPathParams {
	return findPathParams{
		ChainFrom:      req.ChainFrom,
		TokenFromDenom: req.TokenFromDenom,
		AmountIn:       req.AmountIn,
		ChainTo:        req.ChainTo,
		TokenToDenom:   req.TokenToDenom,
		Addresses:      req.Addresses,
		SmartRoute:     req.SmartRoute,
		SlippageBps:    req.SlippageBps,
	}
}

// validateRequest validates the shared request parameters and converts the
// address list into the chainID->address map the router consumes.
//
// Address rules: every entry must name a known chain, carry a valid bech32
// address whose prefix matches that chain, and no chain may appear twice.
// A non-empty list must include entries for both chainFrom and chainTo.
// An empty list is only allowed when allowEmpty is true (unary mock mode)
// and yields a nil map.
func (s *FindPathServer) validateRequest(p findPathParams, allowEmpty bool) (map[string]string, error) {
	if _, err := s.Pathfinder.GetChainInfo(p.ChainFrom); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("unknown source chain: %s", p.ChainFrom))
	}
	if _, err := s.Pathfinder.GetChainInfo(p.ChainTo); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("unknown destination chain: %s", p.ChainTo))
	}

	if p.AmountIn == "" || p.AmountIn == "0" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("amount_in must be a positive number"))
	}

	if len(p.Addresses) == 0 {
		if !allowEmpty {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("addresses must not be empty"))
		}
		return nil, nil
	}

	addresses := make(map[string]string, len(p.Addresses))
	for _, entry := range p.Addresses {
		chain, err := s.Pathfinder.GetChainInfo(entry.ChainId)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("unknown chain in addresses: %s", entry.ChainId))
		}
		if _, exists := addresses[entry.ChainId]; exists {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("duplicate address entry for chain %s", entry.ChainId))
		}
		prefix, err := common.ValidateBech32Address(entry.Address)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid address '%s' for chain %s: %w", entry.Address, entry.ChainId, err))
		}
		if chain.Bech32Prefix != "" && prefix != chain.Bech32Prefix {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("address prefix '%s' does not match chain '%s' (expected prefix: %s)",
					prefix, entry.ChainId, chain.Bech32Prefix))
		}
		addresses[entry.ChainId] = entry.Address
	}

	var missing []string
	if _, ok := addresses[p.ChainFrom]; !ok {
		missing = append(missing, p.ChainFrom)
	}
	if _, ok := addresses[p.ChainTo]; !ok {
		missing = append(missing, p.ChainTo)
	}
	if len(missing) > 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("addresses must include entries for the source and destination chains, missing: %v", missing))
	}

	return addresses, nil
}

// findPath validates the request, resolves denoms and runs the pathfinder.
// A route whose address map is missing required chains is rejected with
// CodeInvalidArgument naming the missing chain IDs.
func (s *FindPathServer) findPath(p findPathParams, allowEmpty bool) (*models.RouteResponse, error) {
	addresses, err := s.validateRequest(p, allowEmpty)
	if err != nil {
		return nil, err
	}

	resolvedFromDenom, resolvedToDenom, err := s.ResolveDenoms(
		p.ChainFrom, p.TokenFromDenom, p.ChainTo, p.TokenToDenom)
	if err != nil {
		return nil, err
	}

	internalReq := models.RouteRequest{
		ChainFrom:      p.ChainFrom,
		TokenFromDenom: resolvedFromDenom,
		AmountIn:       p.AmountIn,
		ChainTo:        p.ChainTo,
		TokenToDenom:   resolvedToDenom,
		Addresses:      addresses,
		SmartRoute:     &p.SmartRoute,
		SlippageBps:    &p.SlippageBps,
	}

	internalResp := s.Pathfinder.FindPath(internalReq)
	if len(internalResp.MissingAddressChains) > 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("route requires addresses for chains %v; supply them in the addresses field "+
				"(send an empty addresses array to discover required chains)",
				internalResp.MissingAddressChains))
	}
	return &internalResp, nil
}

// FindPath implements the ConnectRPC handler for finding paths.
//
// Returns:
//   - 400 Bad Request: Invalid input (bad address, unknown chain, missing address for a required chain)
//   - 200 OK with success=false: Valid query but no route exists
//   - 200 OK with success=true: Route found; response_code distinguishes real
//     (RESPONSE_CODE_OK) from mock discovery (RESPONSE_CODE_MOCK_ADDRESSES) results
func (s *FindPathServer) FindPath(
	ctx context.Context,
	req *connect.Request[v2beta.FindPathRequest],
) (*connect.Response[v2beta.FindPathResponse], error) {
	s.Logger.Info().Msgf("Request data for find path; %+v", req.Msg)

	resp, err := s.findPath(paramsFromFindPathRequest(req.Msg), true)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(convertToProtoResponse(resp)), nil
}

// FindPathStreaming implements a bidirectional streaming handler for path finding.
//
// Behavior:
//   - Client sends a FindPathStreamingRequest → server immediately computes and streams back the result.
//   - If the client goes silent for 15 seconds, the server re-runs the last request and pushes
//     fresh data automatically so the client never needs to poll.
//   - The 15-second countdown resets each time the client sends a new request.
//   - Stream ends when the client closes their side (EOF) or the context is cancelled.
//   - If no first request arrives within 60 seconds, the stream is closed to prevent
//     goroutine accumulation from clients that open a stream but never send.
//   - Streams are hard-capped at 1 hour regardless of client behaviour; the client
//     can reconnect and send a new request to continue.
//
// Unlike the unary FindPath there is no mock/discovery mode: every request on
// the stream must carry real addresses.
func (s *FindPathServer) FindPathStreaming(
	ctx context.Context,
	stream *connect.BidiStream[v2beta.FindPathStreamingRequest, v2beta.FindPathStreamingResponse],
) error {
	const (
		idleRefreshInterval = 15 * time.Second
		firstMsgTimeout     = 60 * time.Second
		maxStreamLifetime   = 1 * time.Hour
	)

	type receiveResult struct {
		req *v2beta.FindPathStreamingRequest
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
		lastReq *v2beta.FindPathStreamingRequest
		ticker  *time.Ticker
		tickerC <-chan time.Time // nil until first message received; nil channel blocks in select
	)

	sendResponse := func(req *v2beta.FindPathStreamingRequest) error {
		// Validate per message; the protovalidate interceptor is not relied
		// on for bidi stream messages.
		resp, err := s.findPath(paramsFromStreamingRequest(req), false)
		if err != nil {
			return err
		}
		return stream.Send(convertToProtoStreamingResponse(resp))
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
			s.Logger.Info().Msgf("Stream request received; %+v", result.req)
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
			s.Logger.Info().Msg("Auto-refreshing stream path response after idle timeout")
			if err := sendResponse(lastReq); err != nil {
				return err
			}
		}
	}
}
