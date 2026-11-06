package mcp_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	pfmcp "github.com/Cogwheel-Validator/spectra-portal/pathfinder/mcp"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/mcp/cosmostx"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/routeindex"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/handlers/common"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"
	"github.com/zeebo/assert"
)

func connect(t *testing.T) *sdk.ClientSession {
	t.Helper()
	return connectWith(t, pfmcp.Options{Version: "test"})
}

func connectWith(t *testing.T, opts pfmcp.Options) *sdk.ClientSession {
	t.Helper()
	chains := []routeindex.PathfinderChain{
		{Name: "Cosmos Hub", Id: "cosmoshub-4", Bech32Prefix: "cosmos", Slip44: 118},
		{Name: "Osmosis", Id: "osmosis-1", Bech32Prefix: "osmo", Slip44: 118},
	}
	pf := router.NewPathfinder(chains, routeindex.NewRouteIndex(), nil)
	server := pfmcp.NewServer(common.Deps{Pathfinder: pf}, opts)

	ctx := context.Background()
	st, ct := sdk.NewInMemoryTransports()
	_, err := server.Connect(ctx, st, nil)
	assert.NoError(t, err)
	session, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, ct, nil)
	assert.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func call(t *testing.T, s *sdk.ClientSession, name string, args map[string]any) *sdk.CallToolResult {
	t.Helper()
	res, err := s.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	assert.NoError(t, err)
	return res
}

func text(t *testing.T, res *sdk.CallToolResult) string {
	t.Helper()
	assert.Equal(t, len(res.Content), 1)
	tc, ok := res.Content[0].(*sdk.TextContent)
	assert.True(t, ok)
	return tc.Text
}

const readOnlyTools = "address_info,assemble_signed_tx,build_sign_doc,build_transaction,compute_ibc_denom,convert_address," +
	"decode_tx,find_route,get_account,get_chain_endpoints,get_chain_info,get_chain_tokens,get_execution_guide," +
	"get_token_denoms,get_tx,list_chains,lookup_denom,prepare_transfer,track_transfer"

func toolNames(t *testing.T, s *sdk.ClientSession) (names []string, byName map[string]*sdk.Tool) {
	t.Helper()
	list, err := s.ListTools(context.Background(), nil)
	assert.NoError(t, err)
	byName = map[string]*sdk.Tool{}
	for _, tool := range list.Tools {
		names = append(names, tool.Name)
		byName[tool.Name] = tool
	}
	sort.Strings(names)
	return names, byName
}

func TestDefaultToolsAreAllReadOnlyAndCannotBroadcast(t *testing.T) {
	names, byName := toolNames(t, connect(t))
	assert.Equal(t, strings.Join(names, ","), readOnlyTools)
	for _, tool := range byName {
		assert.NotNil(t, tool.Annotations)
		assert.True(t, tool.Annotations.ReadOnlyHint)
	}
}

func TestBroadcastToolOnlyWhenEnabledAndMarkedDestructive(t *testing.T) {
	names, byName := toolNames(t, connectWith(t, pfmcp.Options{Broadcast: true}))
	assert.Equal(t, strings.Join(names, ","), strings.Replace(readOnlyTools, "build_sign_doc", "broadcast_signed_tx,build_sign_doc", 1))
	b := byName["broadcast_signed_tx"]
	assert.False(t, b.Annotations.ReadOnlyHint)
	assert.NotNil(t, b.Annotations.DestructiveHint)
	assert.True(t, *b.Annotations.DestructiveHint)
	for name, tool := range byName {
		if name != "broadcast_signed_tx" {
			assert.True(t, tool.Annotations.ReadOnlyHint)
		}
	}
}

func TestGuideIsToolAndResource(t *testing.T) {
	s := connect(t)
	res := call(t, s, "get_execution_guide", nil)
	assert.False(t, res.IsError)
	assert.True(t, strings.Contains(text(t, res), "Safety rules"))

	rr, err := s.ReadResource(context.Background(), &sdk.ReadResourceParams{URI: "spectra://guide/execution"})
	assert.NoError(t, err)
	assert.True(t, strings.Contains(rr.Contents[0].Text, "build_sign_doc"))
}

func TestComputeIBCDenomMatchesKnownHash(t *testing.T) {
	// ATOM on Osmosis over channel-0: the well-known denom.
	res := call(t, connect(t), "compute_ibc_denom", map[string]any{"base_denom": "uatom", "trace_path": "transfer/channel-0"})
	assert.False(t, res.IsError)
	assert.True(t, strings.Contains(text(t, res), "ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2"))

	bad := call(t, connect(t), "compute_ibc_denom", map[string]any{"base_denom": "uatom", "trace_path": "transfer"})
	assert.True(t, bad.IsError)
}

func TestAddressToolsAndDecodeTx(t *testing.T) {
	s := connect(t)
	addr := "cosmos1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5lzv7xu"
	res := call(t, s, "address_info", map[string]any{"address": addr})
	assert.True(t, strings.Contains(text(t, res), `"valid": true`))
	assert.True(t, strings.Contains(text(t, res), "cosmoshub-4"))
	res = call(t, s, "address_info", map[string]any{"address": "cosmos1notvalid"})
	assert.True(t, strings.Contains(text(t, res), `"valid": false`))

	res = call(t, s, "convert_address", map[string]any{"address": addr, "target_chain_id": "osmosis-1"})
	assert.False(t, res.IsError)
	assert.True(t, strings.Contains(text(t, res), "osmo1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5"))

	// decode_tx round trip through the real tool surface.
	msg := &cosmostx.MsgTransfer{SourcePort: "transfer", SourceChannel: "channel-1", Token: cosmostx.Coin{Denom: "uatom", Amount: "9"}, Sender: "a", Receiver: "b"}
	tx := cosmostx.BuildTxRaw(cosmostx.BuildTxBody([]cosmostx.Msg{msg}, "hi", 0), nil)
	res = call(t, s, "decode_tx", map[string]any{"tx_base64": base64.StdEncoding.EncodeToString(tx)})
	assert.False(t, res.IsError)
	out := text(t, res)
	assert.True(t, strings.Contains(out, "MsgTransfer"))
	assert.True(t, strings.Contains(out, `"amount": "9"`))
	res = call(t, s, "decode_tx", map[string]any{"tx_hex": hex.EncodeToString(tx)})
	assert.False(t, res.IsError)
	assert.True(t, call(t, s, "decode_tx", map[string]any{"tx_base64": "AAAA"}).IsError)
	assert.True(t, call(t, s, "decode_tx", nil).IsError)
}

func TestChainEndpointsToolReportsMissingConfig(t *testing.T) {
	res := call(t, connect(t), "get_chain_endpoints", map[string]any{"chain_id": "osmosis-1"})
	assert.False(t, res.IsError)
	assert.True(t, strings.Contains(text(t, res), "Regenerate"))
	assert.True(t, call(t, connect(t), "get_chain_endpoints", map[string]any{"chain_id": "nope"}).IsError)
}

func TestListChains(t *testing.T) {
	session := connect(t)
	res, err := session.CallTool(context.Background(), &sdk.CallToolParams{Name: "list_chains"})
	assert.NoError(t, err)
	assert.False(t, res.IsError)
	out := text(t, res)
	assert.True(t, strings.Contains(out, "cosmoshub-4"))
	assert.True(t, strings.Contains(out, "osmosis-1"))
}

func TestGetChainInfoUnknownChainIsToolError(t *testing.T) {
	session := connect(t)
	res, err := session.CallTool(context.Background(), &sdk.CallToolParams{
		Name:      "get_chain_info",
		Arguments: map[string]any{"chain_id": "nope-1"},
	})
	assert.NoError(t, err)
	assert.True(t, res.IsError)
	assert.True(t, strings.Contains(text(t, res), "not_found"))
}

func TestFindRouteValidationIsToolError(t *testing.T) {
	session := connect(t)
	res, err := session.CallTool(context.Background(), &sdk.CallToolParams{
		Name: "find_route",
		Arguments: map[string]any{
			"chain_from": "nope-1", "token_from": "uatom", "amount_in": "1", "chain_to": "osmosis-1",
		},
	})
	assert.NoError(t, err)
	assert.True(t, res.IsError)
	assert.True(t, strings.Contains(text(t, res), "unknown source chain"))
}

func TestRequestLogShowsToolAndArgNamesButNeverValues(t *testing.T) {
	var buf bytes.Buffer
	chains := []routeindex.PathfinderChain{{Name: "Osmosis", Id: "osmosis-1", Bech32Prefix: "osmo", Slip44: 118}}
	pf := router.NewPathfinder(chains, routeindex.NewRouteIndex(), nil)
	server := pfmcp.NewServer(common.Deps{Pathfinder: pf, Logger: zerolog.New(&buf)}, pfmcp.Options{})

	ctx := context.Background()
	st, ct := sdk.NewInMemoryTransports()
	_, err := server.Connect(ctx, st, nil)
	assert.NoError(t, err)
	s, err := sdk.NewClient(&sdk.Implementation{Name: "my-agent", Version: "1.2"}, nil).Connect(ctx, ct, nil)
	assert.NoError(t, err)
	defer s.Close()

	secret := "osmo1supersecretaddressvalue"
	call(t, s, "address_info", map[string]any{"address": secret})
	call(t, s, "get_chain_endpoints", map[string]any{"chain_id": "nope"}) // tool error
	_, err = s.ReadResource(ctx, &sdk.ReadResourceParams{URI: "spectra://guide/execution"})
	assert.NoError(t, err)

	log := buf.String()
	assert.True(t, strings.Contains(log, `"client":"my-agent/1.2"`))
	assert.True(t, strings.Contains(log, `"mcp_method":"tools/call"`))
	assert.True(t, strings.Contains(log, `"tool":"address_info","args":["address"]`))
	assert.True(t, strings.Contains(log, `"tool":"get_chain_endpoints"`))
	assert.True(t, strings.Contains(log, `"tool_error":true`))
	assert.True(t, strings.Contains(log, `"resource":"spectra://guide/execution"`))
	assert.True(t, strings.Contains(log, `"duration"`))
	// Values must never be logged.
	assert.False(t, strings.Contains(log, secret))
	assert.False(t, strings.Contains(log, "nope"))
}

// Over the real HTTP transport (initialize handshake + session id), which is
// what curl-style and most deployed clients use.
func TestRequestLogOverHTTPTransport(t *testing.T) {
	var buf bytes.Buffer
	chains := []routeindex.PathfinderChain{{Name: "Osmosis", Id: "osmosis-1", Bech32Prefix: "osmo", Slip44: 118}}
	pf := router.NewPathfinder(chains, routeindex.NewRouteIndex(), nil)
	srv := httptest.NewServer(pfmcp.Handler(pfmcp.NewServer(common.Deps{Pathfinder: pf, Logger: zerolog.New(&buf)}, pfmcp.Options{})))
	defer srv.Close()

	ctx := context.Background()
	s, err := sdk.NewClient(&sdk.Implementation{Name: "http-agent", Version: "9"}, nil).
		Connect(ctx, &sdk.StreamableClientTransport{Endpoint: srv.URL}, nil)
	assert.NoError(t, err)
	defer s.Close()
	call(t, s, "list_chains", nil)

	log := buf.String()
	assert.True(t, strings.Contains(log, `"tool":"list_chains"`))
	assert.True(t, strings.Contains(log, `"client":"http-agent/9"`))
	assert.True(t, strings.Contains(log, `"mcp_method":"tools/call"`))
}
