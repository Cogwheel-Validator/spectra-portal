// Package mcp exposes the Pathfinder to AI agents through the Model Context
// Protocol.
//
// The tools are thin wrappers over the v2beta ConnectRPC handlers, called
// in-process, so validation and behaviour match the RPC API exactly.
//
// The server never holds keys and never signs or broadcasts. For a transfer it
// returns the unsigned execution data (messages, memos) produced by FindPath;
// the user's wallet must sign and submit it.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/handlers/common"
	v2betahandlers "github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/handlers/v2beta"
	v2beta "github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/services/pathfinder/v2beta"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Path is where the streamable HTTP MCP endpoint is mounted.
const Path = "/mcp"

// Options configures the MCP server.
type Options struct {
	// Version is reported to MCP clients.
	Version string
	// Broadcast enables broadcast_signed_tx, the only tool that submits
	// anything to a chain. It is off unless the operator opts in.
	Broadcast bool
	// PublicURL is the externally reachable base URL of this server (no
	// trailing /mcp), put into the signing requests returned by
	// prepare_transfer. When empty it is derived from the request headers.
	PublicURL string
	// InlineUI serves the experimental inline signer view (MCP Apps).
	InlineUI bool
}

// NewServer builds an MCP server exposing the Pathfinder tools.
func NewServer(deps common.Deps, opts Options) *sdk.Server {
	if opts.Version == "" {
		opts.Version = "dev"
	}
	s := sdk.NewServer(&sdk.Implementation{Name: "spectra-pathfinder", Version: opts.Version}, &sdk.ServerOptions{
		Instructions: serverInstructions(opts),
	})
	s.AddReceivingMiddleware(loggingMiddleware(deps.Logger))
	t := newTools(deps, opts)
	t.register(s)
	t.registerTx(s)
	t.registerUtil(s)
	t.registerSigning(s)
	if opts.InlineUI {
		t.registerInlineUI(s)
	}
	registerGuide(s)
	return s
}

func serverInstructions(opts Options) string {
	msg := "Spectra Pathfinder is a planning, building and tracking layer for cross-chain (IBC) transfers and DEX swaps between Cosmos chains. " +
		"Read get_execution_guide first. It finds routes (list_chains, get_chain_tokens, find_route), builds unsigned transactions (build_transaction), " +
		"and follows transfers to completion (track_transfer). " +
		"It never holds keys and never signs: the user's own wallet signs. The easiest path is prepare_transfer, which validates the route and returns a " +
		"`pathfinder-sign` command the user approves in Keplr; get_signing_options lists the other ways to sign. "
	if opts.Broadcast {
		return msg + "broadcast_signed_tx submits an already-signed transaction (restricted to Spectra route message types)."
	}
	return msg + "Broadcasting is disabled here: submit signed transactions through your own RPC/REST endpoint (see get_chain_endpoints) or let the wallet broadcast."
}

func newTools(deps common.Deps, opts Options) *tools {
	api := newChainAPI(deps.Pathfinder.GetChainInfo)
	return &tools{
		deps:      deps,
		opts:      opts,
		query:     v2betahandlers.NewQueryServer(deps),
		findPath:  v2betahandlers.NewFindPathServer(deps),
		api:       api,
		now:       time.Now,
		trackerFn: &tracker{api: api, now: time.Now},
	}
}

// Handler returns an http.Handler serving the MCP streamable HTTP transport.
func Handler(server *sdk.Server) http.Handler {
	opts := sdk.StreamableHTTPOptions{
		DisableLocalhostProtection: true,
	}
	h := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, &opts)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Go keeps Host out of the header map (it lives in r.Host), so tools could never see it.
		// Always overwrite, so a client cannot supply its own value.
		r.Header.Set(hostHeader, r.Host)
		h.ServeHTTP(w, r)
	})
}

// hostHeader carries the request's Host to the tools (set by Handler).
const hostHeader = "X-Spectra-Request-Host"

type tools struct {
	deps      common.Deps
	opts      Options
	query     *v2betahandlers.QueryServer
	findPath  *v2betahandlers.FindPathServer
	api       *chainAPI
	trackerFn *tracker
	now       func() time.Time
}

type noInput struct{}

type chainInput struct {
	ChainID string `json:"chain_id" jsonschema:"chain ID, e.g. osmosis-1"`
}

type chainInfoInput struct {
	ChainID     string `json:"chain_id" jsonschema:"chain ID, e.g. osmosis-1"`
	ShowSymbols bool   `json:"show_symbols,omitempty" jsonschema:"include token symbols"`
}

type lookupDenomInput struct {
	ChainID string `json:"chain_id" jsonschema:"chain the denom is seen on"`
	Denom   string `json:"denom" jsonschema:"native denom (uatom) or IBC denom (ibc/HASH)"`
}

type tokenDenomsInput struct {
	BaseDenom   string `json:"base_denom" jsonschema:"base denom, e.g. uatom"`
	OriginChain string `json:"origin_chain" jsonschema:"chain where the token is native"`
	OnChainID   string `json:"on_chain_id,omitempty" jsonschema:"optionally restrict to one chain"`
}

type addressInput struct {
	ChainID string `json:"chain_id"`
	Address string `json:"address" jsonschema:"bech32 address valid on chain_id"`
}

type findRouteInput struct {
	ChainFrom   string         `json:"chain_from" jsonschema:"source chain ID"`
	TokenFrom   string         `json:"token_from" jsonschema:"denom, IBC denom, or symbol@origin_chain"`
	AmountIn    string         `json:"amount_in" jsonschema:"amount in base units (e.g. 1000000 for 1 ATOM)"`
	ChainTo     string         `json:"chain_to" jsonschema:"destination chain ID"`
	TokenTo     string         `json:"token_to,omitempty" jsonschema:"destination token; empty keeps the same token (no swap)"`
	Addresses   []addressInput `json:"addresses,omitempty" jsonschema:"one address per chain on the route. Omit to preview the route without execution data; supply to get the unsigned transaction"`
	SmartRoute  bool           `json:"smart_route,omitempty" jsonschema:"set to true to use multihop / broker swap and merge in one transaction. False marks that this is to be executed with several small transactions, one per leg."`
	SlippageBps uint32         `json:"slippage_bps,omitempty" jsonschema:"max swap slippage in basis points"`
}

func (t *tools) register(s *sdk.Server) {
	readOnly := &sdk.ToolAnnotations{ReadOnlyHint: true}

	sdk.AddTool(s, &sdk.Tool{
		Name:        "list_chains",
		Description: "List the IDs of all chains the Pathfinder supports.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *sdk.CallToolRequest, _ noInput) (*sdk.CallToolResult, any, error) {
		return call(t.query.ListSupportedChains(ctx, connect.NewRequest(&emptypb.Empty{})))
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "get_chain_info",
		Description: "Get information about one supported chain (name, prefix, connected chains, tokens).",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in chainInfoInput) (*sdk.CallToolResult, any, error) {
		return call(t.query.GetChainInfo(ctx, connect.NewRequest(&v2beta.GetChainInfoRequest{
			ChainId: in.ChainID, ShowSymbols: in.ShowSymbols,
		})))
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "get_chain_tokens",
		Description: "List native and IBC tokens available on a chain, with their denoms.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in chainInput) (*sdk.CallToolResult, any, error) {
		return call(t.query.GetChainTokens(ctx, connect.NewRequest(&v2beta.GetChainTokensRequest{ChainId: in.ChainID})))
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "lookup_denom",
		Description: "Resolve a denom (native or ibc/...) on a chain to its base denom, origin chain and where else it exists.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in lookupDenomInput) (*sdk.CallToolResult, any, error) {
		return call(t.query.LookupDenom(ctx, connect.NewRequest(&v2beta.LookupDenomRequest{
			ChainId: in.ChainID, Denom: in.Denom,
		})))
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "get_token_denoms",
		Description: "Get the denom of a token on every supported chain.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in tokenDenomsInput) (*sdk.CallToolResult, any, error) {
		return call(t.query.GetTokenDenoms(ctx, connect.NewRequest(&v2beta.GetTokenDenomsRequest{
			BaseDenom: in.BaseDenom, OriginChain: in.OriginChain, OnChainId: in.OnChainID,
		})))
	})

	sdk.AddTool(s, &sdk.Tool{
		Name: "find_route",
		Description: "Find a route to move a token between chains (direct IBC, multi-hop, or with a DEX swap). " +
			"Without addresses it is a read-only preview and lists required_chains. With an address per chain it " +
			"returns the UNSIGNED transaction data (messages/memo). Nothing is signed or broadcast: the user must " +
			"review the route and sign it in their own wallet. Always show the user the amounts, chains and " +
			"recipient before they sign.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in findRouteInput) (*sdk.CallToolResult, any, error) {
		addrs := make([]*v2beta.ChainAddress, len(in.Addresses))
		for i, a := range in.Addresses {
			addrs[i] = &v2beta.ChainAddress{ChainId: a.ChainID, Address: a.Address}
		}
		return call(t.findPath.FindPath(ctx, connect.NewRequest(&v2beta.FindPathRequest{
			ChainFrom:      in.ChainFrom,
			TokenFromDenom: in.TokenFrom,
			AmountIn:       in.AmountIn,
			ChainTo:        in.ChainTo,
			TokenToDenom:   in.TokenTo,
			Addresses:      addrs,
			SmartRoute:     in.SmartRoute,
			SlippageBps:    in.SlippageBps,
		})))
	})
}

// call converts a ConnectRPC handler result into an MCP tool result carrying
// the protobuf message as JSON. Handler errors become tool errors so the
// model can read and react to them.
func call[T any, P interface {
	*T
	proto.Message
}](resp *connect.Response[T], err error) (*sdk.CallToolResult, any, error) {
	if err != nil {
		return nil, nil, fmt.Errorf("%s", connect.CodeOf(err).String()+": "+errMessage(err))
	}
	b, err := protoJSON(P(resp.Msg))
	if err != nil {
		return nil, nil, err
	}
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: string(b)}}}, nil, nil
}

func errMessage(err error) string {
	var ce *connect.Error
	if errors.As(err, &ce) {
		return ce.Message()
	}
	return err.Error()
}

// jsonResult wraps any value as a JSON text tool result.
func jsonResult(v any) (*sdk.CallToolResult, any, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: string(b)}}}, nil, nil
}

// protoJSON renders a protobuf message with proto field names. Zero values are
// emitted so that e.g. {"found": false} is explicit rather than an empty object
// an agent could mistake for missing data.
func protoJSON(m proto.Message) ([]byte, error) {
	return protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true}.Marshal(m)
}
