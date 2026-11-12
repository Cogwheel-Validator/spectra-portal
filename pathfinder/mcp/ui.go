package mcp

import (
	"context"

	"github.com/Cogwheel-Validator/spectra-portal/signer/web"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// signerURI is the MCP Apps (ui://) resource holding the inline signer view.
const signerURI = "ui://spectra/signer"

// appMIME is the MIME type MCP Apps hosts recognise as an embeddable app.
const appMIME = "text/html;profile=mcp-app"

// signerUIMeta links prepare_transfer to the inline view.
func signerUIMeta() sdk.Meta {
	return sdk.Meta{
		"ui":             map[string]any{"resourceUri": signerURI},
		"ui/resourceUri": signerURI, // legacy flat key used by early hosts
	}
}

// registerInlineUI serves the signer page as an MCP Apps resource. It is
// experimental: browser wallets only inject themselves into origins they
// trust, and chat hosts run apps on their own sandbox origin, so Keplr is
// usually unavailable there. See docs/MCP_SIGNING.md.
func (t *tools) registerInlineUI(s *sdk.Server) {
	s.AddResource(&sdk.Resource{
		URI: signerURI, Name: "pathfinder-signer", MIMEType: appMIME,
		Description: "Experimental inline view that connects the user's Keplr wallet, shows the transfer and signs it.",
	}, func(_ context.Context, _ *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
		return &sdk.ReadResourceResult{Contents: []*sdk.ResourceContents{{
			URI: signerURI, MIMEType: appMIME, Text: web.SignerHTML,
			// No external origins: Keplr signs and broadcasts through the
			// extension, everything else goes through the host.
			Meta: sdk.Meta{"ui": map[string]any{
				"csp":           map[string]any{"connectDomains": []string{}, "resourceDomains": []string{}},
				"prefersBorder": true,
			}},
		}}}, nil
	})
}
