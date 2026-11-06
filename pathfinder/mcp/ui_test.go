package mcp_test

import (
	"context"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zeebo/assert"
)

func TestSignerResourceIsMCPApp(t *testing.T) {
	s := connect(t)
	res, err := s.ReadResource(context.Background(), &sdk.ReadResourceParams{URI: "ui://spectra/signer"})
	assert.NoError(t, err)
	c := res.Contents[0]
	assert.Equal(t, c.MIMEType, "text/html;profile=mcp-app")
	assert.True(t, strings.Contains(c.Text, "window.keplr"))
	// Self-contained: the view must not load anything from the network.
	assert.False(t, strings.Contains(c.Text, "src=\"http"))
	assert.False(t, strings.Contains(c.Text, ".innerHTML"))
	ui, ok := c.Meta["ui"].(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, len(ui["csp"].(map[string]any)["connectDomains"].([]any)), 0)
}

func TestPrepareTransferLinksToViewAndValidates(t *testing.T) {
	s := connect(t)
	_, byName := toolNames(t, s)
	tool := byName["prepare_transfer"]
	ui, ok := tool.Meta["ui"].(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, ui["resourceUri"], "ui://spectra/signer")
	assert.True(t, tool.Annotations.ReadOnlyHint)

	res := call(t, s, "prepare_transfer", map[string]any{
		"chain_from": "nope-1", "token_from": "uatom", "amount_in": "1", "chain_to": "osmosis-1",
	})
	assert.True(t, res.IsError)
}
