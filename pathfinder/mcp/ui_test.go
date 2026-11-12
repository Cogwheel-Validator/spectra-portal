package mcp_test

import (
	"context"
	"strings"
	"testing"

	pfmcp "github.com/Cogwheel-Validator/spectra-portal/pathfinder/mcp"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zeebo/assert"
)

func TestInlineViewIsOffByDefault(t *testing.T) {
	s := connect(t)
	_, byName := toolNames(t, s)
	assert.Equal(t, len(byName["prepare_transfer"].Meta), 0)
	_, err := s.ReadResource(context.Background(), &sdk.ReadResourceParams{URI: "ui://spectra/signer"})
	assert.Error(t, err)
}

func TestInlineViewResourceIsMCPApp(t *testing.T) {
	s := connectWith(t, pfmcp.Options{Version: "test", InlineUI: true})
	res, err := s.ReadResource(context.Background(), &sdk.ReadResourceParams{URI: "ui://spectra/signer"})
	assert.NoError(t, err)
	c := res.Contents[0]
	assert.Equal(t, c.MIMEType, "text/html;profile=mcp-app")
	assert.True(t, strings.Contains(c.Text, "window.keplr"))
	// Self-contained: the page must not load anything from the network.
	assert.False(t, strings.Contains(c.Text, "src=\"http"))
	assert.False(t, strings.Contains(c.Text, ".innerHTML"))
	ui, ok := c.Meta["ui"].(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, len(ui["csp"].(map[string]any)["connectDomains"].([]any)), 0)

	_, byName := toolNames(t, s)
	link, ok := byName["prepare_transfer"].Meta["ui"].(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, link["resourceUri"], "ui://spectra/signer")
}

func TestPrepareTransferAndSigningOptionsAreReadOnlyTools(t *testing.T) {
	_, byName := toolNames(t, connect(t))
	assert.True(t, byName["prepare_transfer"].Annotations.ReadOnlyHint)
	assert.True(t, byName["get_signing_options"].Annotations.ReadOnlyHint)
}

func TestGetSigningOptionsPointsToReleases(t *testing.T) {
	out := text(t, call(t, connect(t), "get_signing_options", map[string]any{}))
	assert.True(t, strings.Contains(out, "releases/latest"))
	assert.True(t, strings.Contains(out, "pathfinder-sign"))
	assert.True(t, strings.Contains(out, "cli_signing"))
}
