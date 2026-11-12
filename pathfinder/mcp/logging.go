package mcp

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"
)

// loggingMiddleware logs every MCP request at the protocol level, so operators
// can see which tool an agent called (plain HTTP logs only show "POST /mcp").
//
// Only the method, tool/resource name, argument NAMES, outcome and duration are
// logged. Argument values are never logged: they can contain addresses,
// transaction bytes and public keys.
func loggingMiddleware(l zerolog.Logger) sdk.Middleware {
	return func(next sdk.MethodHandler) sdk.MethodHandler {
		return func(ctx context.Context, method string, req sdk.Request) (sdk.Result, error) {
			start := time.Now()
			res, err := next(ctx, method, req)

			ev := l.Info()
			switch {
			case err != nil:
				ev = l.Warn().Err(err)
			case method == "ping" || strings.HasPrefix(method, "notifications/"):
				ev = l.Debug()
			}
			ev = ev.Str("component", "mcp").Str("mcp_method", method)
			if s := req.GetSession(); s != nil {
				ev = ev.Str("session", shortID(s.ID()))
			}
			client := ""
			switch p := req.GetParams().(type) {
			case *sdk.CallToolParamsRaw:
				ev = ev.Str("tool", p.Name).Strs("args", argNames(p.Arguments))
			case *sdk.ReadResourceParams:
				ev = ev.Str("resource", p.URI)
			case *sdk.InitializeParams:
				if p.ClientInfo != nil {
					client = p.ClientInfo.Name + "/" + p.ClientInfo.Version
				}
			}
			if ci, ok := req.(interface{ ClientInfo() *sdk.Implementation }); ok && client == "" {
				if c := ci.ClientInfo(); c != nil {
					client = c.Name + "/" + c.Version
				}
			}
			if client != "" {
				ev = ev.Str("client", client)
			}
			if r, ok := res.(*sdk.CallToolResult); ok && r != nil {
				ev = ev.Bool("tool_error", r.IsError)
			}
			ev.Dur("duration", time.Since(start)).Msg("MCP request")
			return res, err
		}
	}
}

// argNames returns the sorted top-level argument names of a tool call.
func argNames(raw json.RawMessage) []string {
	var m map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &m) != nil {
		return nil
	}
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// shortID shortens a session ID enough to correlate log lines without
// exposing a usable session ID.
func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
