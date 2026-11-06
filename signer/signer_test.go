package signer_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Cogwheel-Validator/spectra-portal/signer"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zeebo/assert"
)

type fakeRemote struct{ calls []string }

func (f *fakeRemote) CallTool(_ context.Context, name string, _ map[string]any) (*sdk.CallToolResult, error) {
	f.calls = append(f.calls, name)
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: `{"required_chains":["cosmoshub-4","osmosis-1"]}`}}}, nil
}

func validReq() *signer.Request {
	return &signer.Request{
		PathfinderURL: "https://pathfinder.example.com",
		Transfer:      signer.Transfer{ChainFrom: "cosmoshub-4", TokenFrom: "uatom", AmountIn: "1000000", ChainTo: "osmosis-1"},
	}
}

func start(t *testing.T) (*signer.Server, string, *fakeRemote) {
	t.Helper()
	f := &fakeRemote{}
	s, err := signer.New(validReq(), f)
	assert.NoError(t, err)
	link, err := s.Listen(0)
	assert.NoError(t, err)
	go s.Serve(context.Background(), time.Minute)
	return s, link, f
}

func do(t *testing.T, method, url, token string, body string, mod func(*http.Request)) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	if token != "" {
		req.Header.Set("X-Spectra-Token", token)
	}
	if mod != nil {
		mod(req)
	}
	resp, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(b)
}

func split(link string) (base, token string) {
	i := strings.Index(link, "/?t=")
	return link[:i], link[i+4:]
}

func TestValidate(t *testing.T) {
	assert.NoError(t, validReq().Validate())
	local := validReq()
	local.PathfinderURL = "http://127.0.0.1:8080"
	assert.NoError(t, local.Validate())

	for name, mut := range map[string]func(*signer.Request){
		"http remote":   func(r *signer.Request) { r.PathfinderURL = "http://pathfinder.example.com" },
		"no scheme":     func(r *signer.Request) { r.PathfinderURL = "pathfinder.example.com" },
		"zero amount":   func(r *signer.Request) { r.Transfer.AmountIn = "000" },
		"decimal":       func(r *signer.Request) { r.Transfer.AmountIn = "1.5" },
		"negative":      func(r *signer.Request) { r.Transfer.AmountIn = "-1" },
		"missing chain": func(r *signer.Request) { r.Transfer.ChainTo = "" },
		"bad mode":      func(r *signer.Request) { r.Transfer.Mode = "turbo" },
	} {
		r := validReq()
		mut(r)
		if r.Validate() == nil {
			t.Errorf("%s: expected a validation error", name)
		}
	}
}

func TestPageNeedsTokenAndInjectsBootstrap(t *testing.T) {
	_, link, _ := start(t)
	base, token := split(link)

	resp, _ := do(t, "GET", base+"/", "", "", nil)
	assert.Equal(t, resp.StatusCode, http.StatusUnauthorized)
	resp, _ = do(t, "GET", base+"/?t=wrong", "", "", nil)
	assert.Equal(t, resp.StatusCode, http.StatusUnauthorized)

	resp, body := do(t, "GET", link, "", "", nil)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	assert.True(t, strings.Contains(body, `window.__SPECTRA_LOCAL__={"token":"`+token+`"}`))
	assert.True(t, strings.Contains(body, "window.keplr"))
	assert.True(t, strings.Contains(resp.Header.Get("Content-Security-Policy"), "connect-src 'self'"))
}

func TestHostAndOriginAreChecked(t *testing.T) {
	_, link, _ := start(t)
	base, token := split(link)

	resp, _ := do(t, "GET", base+"/api/request", token, "", func(r *http.Request) { r.Host = "evil.example.com" })
	assert.Equal(t, resp.StatusCode, http.StatusForbidden)
	resp, _ = do(t, "GET", base+"/api/request", token, "", func(r *http.Request) { r.Header.Set("Origin", "https://evil.example.com") })
	assert.Equal(t, resp.StatusCode, http.StatusForbidden)
	resp, _ = do(t, "GET", base+"/api/request", token, "", nil)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
}

func TestAPIRequiresTokenAndReturnsPreview(t *testing.T) {
	_, link, f := start(t)
	base, token := split(link)

	resp, _ := do(t, "GET", base+"/api/request", "", "", nil)
	assert.Equal(t, resp.StatusCode, http.StatusUnauthorized)

	_, body := do(t, "GET", base+"/api/request", token, "", nil)
	var got struct {
		Request signer.Transfer `json:"request"`
		Preview struct {
			Required []string `json:"required_chains"`
		} `json:"preview"`
	}
	assert.NoError(t, json.Unmarshal([]byte(body), &got))
	assert.Equal(t, got.Request.AmountIn, "1000000")
	assert.Equal(t, len(got.Preview.Required), 2)
	assert.Equal(t, f.calls[0], "find_route")
}

func TestCallProxyIsAllowlisted(t *testing.T) {
	_, link, f := start(t)
	base, token := split(link)

	resp, _ := do(t, "POST", base+"/api/call", token, `{"name":"build_sign_doc","arguments":{}}`, nil)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	assert.Equal(t, f.calls[len(f.calls)-1], "build_sign_doc")

	for _, name := range []string{"get_account", "decode_tx", "list_chains", "not_a_tool"} {
		resp, _ = do(t, "POST", base+"/api/call", token, `{"name":"`+name+`","arguments":{}}`, nil)
		assert.Equal(t, resp.StatusCode, http.StatusForbidden)
	}
}

func TestResultEndsTheRun(t *testing.T) {
	s, err := signer.New(validReq(), &fakeRemote{})
	assert.NoError(t, err)
	link, err := s.Listen(0)
	assert.NoError(t, err)
	base, token := split(link)

	done := make(chan signer.Result, 1)
	go func() { done <- s.Serve(context.Background(), time.Minute) }()

	// Results without transactions are rejected and do not end the run.
	resp, _ := do(t, "POST", base+"/api/result", token, `{"status":"completed"}`, nil)
	assert.Equal(t, resp.StatusCode, http.StatusBadRequest)
	resp, _ = do(t, "POST", base+"/api/result", token, `{"status":"completed","transactions":[{"chain_id":"cosmoshub-4","tx_hash":"ABC"}]}`, nil)
	assert.Equal(t, resp.StatusCode, http.StatusNoContent)

	select {
	case r := <-done:
		assert.Equal(t, r.Status, "completed")
		assert.Equal(t, r.Transactions[0].TxHash, "ABC")
	case <-time.After(5 * time.Second):
		t.Fatal("run did not finish")
	}
}

func TestTimeout(t *testing.T) {
	s, err := signer.New(validReq(), &fakeRemote{})
	assert.NoError(t, err)
	_, err = s.Listen(0)
	assert.NoError(t, err)
	r := s.Serve(context.Background(), 50*time.Millisecond)
	assert.Equal(t, r.Status, "timeout")
}
