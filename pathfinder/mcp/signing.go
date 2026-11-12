package mcp

import (
	"context"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	v2beta "github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/services/pathfinder/v2beta"
	"github.com/Cogwheel-Validator/spectra-portal/signer"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// releasesURL is where pathfinder-sign binaries are published.
const releasesURL = "https://github.com/Cogwheel-Validator/spectra-portal/releases/latest"

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

// registerSigning adds the tools that hand a transfer to the user's own wallet.
func (t *tools) registerSigning(s *sdk.Server) {
	tool := &sdk.Tool{
		Name: "prepare_transfer",
		Description: "Validate a transfer and get everything needed to have the USER sign it with their browser wallet (Keplr), without asking for addresses. " +
			"Returns a ready-to-run `pathfinder-sign` command (a small program on the user's machine that opens a page where they approve in Keplr and prints the tx hashes). " +
			"Run it if you can execute commands on the user's computer, otherwise give it to the user to run, then call track_transfer with the hashes. " +
			"Use this instead of build_transaction/build_sign_doc unless the user wants to sign some other way (call get_signing_options). Nothing is signed or sent by this tool.",
		Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true},
	}
	if t.opts.InlineUI {
		tool.Meta = signerUIMeta()
	}
	sdk.AddTool(s, tool, t.prepareTransfer)

	sdk.AddTool(s, &sdk.Tool{
		Name: "get_signing_options",
		Description: "Explains the ways a user can sign a transfer prepared by this server (local signer program, manual CLI signing, experimental inline view), " +
			"what each needs, and where to download pathfinder-sign. Call it when prepare_transfer's command is not found or the user cannot run it.",
		Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, req *sdk.CallToolRequest, _ noInput) (*sdk.CallToolResult, any, error) {
		return jsonResult(t.signingOptions(req))
	})
}

func (t *tools) prepareTransfer(ctx context.Context, req *sdk.CallToolRequest, in prepareTransferInput) (*sdk.CallToolResult, any, error) {
	sreq := signer.Request{
		PathfinderURL: t.baseURL(req),
		Transfer: signer.Transfer{
			ChainFrom: in.ChainFrom, TokenFrom: in.TokenFrom, AmountIn: in.AmountIn, ChainTo: in.ChainTo,
			TokenTo: in.TokenTo, Recipient: in.Recipient, Mode: in.Mode, SlippageBps: in.SlippageBps,
		},
	}
	if sreq.PathfinderURL == "" {
		return nil, nil, fmt.Errorf("this server does not know its own public URL; the operator must set mcp_public_url. Meanwhile use build_transaction / build_sign_doc (see get_execution_guide)")
	}
	if err := sreq.Validate(); err != nil {
		return nil, nil, err
	}
	// Check the route now so the agent gets routing errors immediately.
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
	cmd := sreq.Command()

	var sb strings.Builder
	sb.WriteString("Route found. Nothing is signed yet: the user signs with their own wallet.\n\n")
	sb.WriteString("Command for the user's machine (opens a page on 127.0.0.1 where they connect Keplr, review and approve; it prints a JSON result on stdout and can take several minutes, so allow a long timeout):\n\n")
	sb.WriteString("    " + cmd + "\n\n")
	sb.WriteString("- If you can run commands on the user's computer, run it. Otherwise ask the user to run it in a terminal and paste the JSON it prints.\n")
	sb.WriteString("- If the command is not found, call get_signing_options (download, other ways to sign).\n")
	sb.WriteString("- Tell the user to check the amounts, chains and recipient in the page before approving.\n")
	sb.WriteString("- On {\"status\":\"completed\",\"transactions\":[{chain_id,tx_hash}]}, call track_transfer for each transaction and report the outcome.\n")
	if in.Recipient != "" {
		sb.WriteString("- The recipient " + in.Recipient + " was supplied by you: confirm it with the user before they sign.\n")
	}

	return &sdk.CallToolResult{
		Content: []sdk.Content{&sdk.TextContent{Text: sb.String()}},
		StructuredContent: map[string]any{
			"command":         cmd,
			"signing_request": sreq,
			// Also consumed by the experimental inline view.
			"request": sreq.Transfer,
			"preview": rawJSON(preview),
		},
	}, nil, nil
}

// baseURL is the public URL of this server: configured, or taken from the
// request headers (behind a proxy the operator should configure it).
func (t *tools) baseURL(req *sdk.CallToolRequest) string {
	if t.opts.PublicURL != "" {
		return strings.TrimRight(t.opts.PublicURL, "/")
	}
	if req == nil || req.Extra == nil || req.Extra.Header == nil {
		return ""
	}
	h := req.Extra.Header
	host := firstNonEmpty(h.Get("X-Forwarded-Host"), h.Get(hostHeader))
	if host == "" {
		return ""
	}
	proto := h.Get("X-Forwarded-Proto")
	if proto == "" {
		proto = "https"
		if isLocalHost(host) {
			proto = "http"
		}
	}
	return proto + "://" + host
}

func isLocalHost(hostport string) bool {
	h := hostport
	if i := strings.LastIndex(h, ":"); i > 0 && !strings.Contains(h[i:], "]") {
		h = h[:i]
	}
	return h == "localhost" || h == "127.0.0.1" || h == "[::1]"
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

func (t *tools) signingOptions(_ *sdk.CallToolRequest) map[string]any {
	return map[string]any{
		"summary": "This server never signs. A transfer is signed by the user's own wallet. Pick the first option that fits the user.",
		"options": []map[string]any{
			{
				"id":        "local_signer",
				"name":      "pathfinder-sign (recommended)",
				"needs":     "A browser with the Keplr extension, and the small pathfinder-sign program on the same computer. No Node/Python needed.",
				"how":       "prepare_transfer returns the exact command. It opens http://127.0.0.1:<port>, the user connects Keplr, approves, and the program prints the tx hashes as JSON.",
				"download":  releasesURL,
				"assets":    "pathfinder-sign-<version>-{linux-amd64,linux-arm64,darwin-amd64,darwin-arm64,windows-amd64.exe}; verify with pathfinder-sign-<version>-checksums.txt",
				"install":   "Linux/macOS: chmod +x the file and put it on PATH as pathfinder-sign. Windows: rename to pathfinder-sign.exe and run it from a terminal.",
				"agent_tip": "Coding agents with a shell can run it directly. Chat apps without a shell: give the user the command and ask them to paste the JSON output back.",
			},
			{
				"id":    "cli_signing",
				"name":  "Sign with a CLI wallet or Ledger",
				"needs": "gaiad/osmosisd (or any signer that signs a Cosmos SignDoc) on the user's machine.",
				"how":   "build_transaction -> build_sign_doc (sign_mode direct, or amino for Ledger) -> user signs the sign doc and returns the 64-byte signature -> assemble_signed_tx -> broadcast -> track_transfer. Never ask for a key or mnemonic.",
			},
			{
				"id":      "inline_view",
				"name":    "Inline view in the chat (experimental)",
				"enabled": t.opts.InlineUI,
				"caveat":  "Chat apps run MCP Apps on their own sandbox origin, which browser wallets usually refuse to inject into, so Keplr is typically unavailable there. If the view reports that Keplr is missing, use pathfinder-sign.",
				"how":     "Shown automatically when prepare_transfer is called in a host that renders MCP Apps.",
			},
		},
		"limits": "Injective is supported with a software wallet (direct signing); a Ledger on Injective needs EIP-712 and is not supported. Other non-standard account types are not supported by build_sign_doc or the signer.",
	}
}

// rawJSON lets already-encoded JSON be embedded in a structured result.
type rawJSON []byte

func (r rawJSON) MarshalJSON() ([]byte, error) { return r, nil }
