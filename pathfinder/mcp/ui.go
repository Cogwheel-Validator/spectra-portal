package mcp

import (
	"context"
	_ "embed"
	"fmt"

	"connectrpc.com/connect"
	v2beta "github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/services/pathfinder/v2beta"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// signerURI is the MCP Apps (ui://) resource holding the inline signer view.
const signerURI = "ui://spectra/signer"

// appMIME is the MIME type MCP Apps hosts recognise as an embeddable app.
const appMIME = "text/html;profile=mcp-app"

// signerHTML is a single self-contained page: no external scripts, styles or
// network access. It signs with the user's Keplr and talks to this server only
// through the host.
//
//go:embed ui/signer.html
var signerHTML string

type prepareTransferInput struct {
	ChainFrom   string `json:"chain_from" jsonschema:"source chain ID"`
	TokenFrom   string `json:"token_from" jsonschema:"denom, IBC denom, or symbol@origin_chain"`
	AmountIn    string `json:"amount_in" jsonschema:"amount in base units (e.g. 1000000 for 1 ATOM)"`
	ChainTo     string `json:"chain_to" jsonschema:"destination chain ID"`
	TokenTo     string `json:"token_to,omitempty" jsonschema:"destination token; empty keeps the same token"`
	Recipient   string `json:"recipient,omitempty" jsonschema:"destination address ONLY if the user explicitly gave one; omit to send to the user's own wallet address on chain_to"`
	Mode        string `json:"mode,omitempty" jsonschema:"smart (default) or manual"`
	SlippageBps uint32 `json:"slippage_bps,omitempty"`
}

// registerUI adds the inline signer: a ui:// resource and the tool that opens it.
func (t *tools) registerUI(s *sdk.Server) {
	s.AddResource(&sdk.Resource{
		URI: signerURI, Name: "spectra-signer", MIMEType: appMIME,
		Description: "Inline view that connects the user's Keplr wallet, shows the transfer and signs it.",
	}, func(_ context.Context, _ *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
		return &sdk.ReadResourceResult{Contents: []*sdk.ResourceContents{{
			URI: signerURI, MIMEType: appMIME, Text: signerHTML,
			// No external origins: Keplr signs and broadcasts through the
			// extension, everything else goes through the host.
			Meta: sdk.Meta{"ui": map[string]any{
				"csp":           map[string]any{"connectDomains": []string{}, "resourceDomains": []string{}},
				"prefersBorder": true,
			}},
		}}}, nil
	})

	sdk.AddTool(s, &sdk.Tool{
		Name: "prepare_transfer",
		Description: "PREFERRED in chat UIs that render MCP Apps: opens an inline signer where the user connects Keplr, reviews the transfer and signs it. " +
			"Call it once with the transfer details; do NOT ask for addresses and do NOT call build_transaction / build_sign_doc yourself, the view does all of it " +
			"and reports the result back. If the view cannot render or Keplr is unavailable, fall back to the manual flow in get_execution_guide.",
		Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true},
		Meta: sdk.Meta{
			"ui":             map[string]any{"resourceUri": signerURI},
			"ui/resourceUri": signerURI, // legacy flat key used by early hosts
		},
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in prepareTransferInput) (*sdk.CallToolResult, any, error) {
		// Validate the route now so the agent gets errors immediately.
		resp, err := t.findPath.FindPath(ctx, connect.NewRequest(&v2beta.FindPathRequest{
			ChainFrom: in.ChainFrom, TokenFromDenom: in.TokenFrom, AmountIn: in.AmountIn,
			ChainTo: in.ChainTo, TokenToDenom: in.TokenTo, SmartRoute: in.Mode != "manual", SlippageBps: in.SlippageBps,
		}))
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %s", connect.CodeOf(err).String(), errMessage(err))
		}
		if !resp.Msg.Success {
			return nil, nil, fmt.Errorf("no route: %s", resp.Msg.ErrorMessage)
		}
		preview, err := protoJSON(resp.Msg)
		if err != nil {
			return nil, nil, err
		}
		return &sdk.CallToolResult{
			Content: []sdk.Content{&sdk.TextContent{Text: "The signer view is open for the user. They connect Keplr, review and sign there; " +
				"you will receive a message when it finishes. Do not call other tools to sign or broadcast. Briefly tell the user to review the recipient and amounts in the view."}},
			StructuredContent: map[string]any{
				"request": map[string]any{
					"chain_from": in.ChainFrom, "token_from": in.TokenFrom, "amount_in": in.AmountIn,
					"chain_to": in.ChainTo, "token_to": in.TokenTo, "recipient": in.Recipient,
					"mode": in.Mode, "slippage_bps": in.SlippageBps,
				},
				"preview": rawJSON(preview),
			},
		}, nil, nil
	})
}

// rawJSON lets already-encoded JSON be embedded in a structured result.
type rawJSON []byte

func (r rawJSON) MarshalJSON() ([]byte, error) { return r, nil }
