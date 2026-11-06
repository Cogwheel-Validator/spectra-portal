// Package signer implements pathfinder-sign, a small program that runs on the
// user's own machine and lets their browser wallet (Keplr) sign a transfer
// that a remote Pathfinder MCP server prepared.
//
// It exists because a browser wallet only talks to pages on origins it trusts:
// 127.0.0.1 is trusted, while the sandboxed origin a chat host gives an inline
// MCP App is not. The program holds no keys. It serves the signer page on
// loopback, forwards a fixed allowlist of tool calls to Pathfinder, and reports
// the transaction hashes back on stdout when the user is done.
package signer

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Cogwheel-Validator/spectra-portal/signer/web"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// placeholder in signer.html where the local-mode bootstrap is injected.
const placeholder = "<!--SPECTRA_LOCAL-->"

// allowedTools are the only tools the page may call through the proxy.
var allowedTools = map[string]bool{
	"find_route": true, "build_transaction": true, "build_sign_doc": true,
	"assemble_signed_tx": true, "broadcast_signed_tx": true, "track_transfer": true,
}

// Transfer is the route the agent asked for. The recipient is optional and,
// when set, was chosen by the agent: the page shows a warning for it.
type Transfer struct {
	ChainFrom   string `json:"chain_from"`
	TokenFrom   string `json:"token_from"`
	AmountIn    string `json:"amount_in"`
	ChainTo     string `json:"chain_to"`
	TokenTo     string `json:"token_to,omitempty"`
	Recipient   string `json:"recipient,omitempty"`
	Mode        string `json:"mode,omitempty"`
	SlippageBps uint32 `json:"slippage_bps,omitempty"`
}

// Request is the document `prepare_transfer` returns and `pathfinder-sign --request` reads.
type Request struct {
	// PathfinderURL is the base URL of the Pathfinder serving /mcp.
	PathfinderURL string   `json:"pathfinder_url"`
	Transfer      Transfer `json:"request"`
}

// TxRef identifies one broadcast transaction.
type TxRef struct {
	ChainID string `json:"chain_id"`
	TxHash  string `json:"tx_hash"`
}

// Result is printed to stdout when the run ends.
type Result struct {
	Status       string  `json:"status"` // completed | failed | timeout | cancelled | error
	Transactions []TxRef `json:"transactions,omitempty"`
	Error        string  `json:"error,omitempty"`
}

var amountRe = regexp.MustCompile(`^[0-9]{1,40}$`)

var urlRe = regexp.MustCompile(`^https?://[A-Za-z0-9._:/\[\]-]{1,200}$`)

// safeRe is the character set allowed in chain IDs, denoms and addresses.
// The values come from an AI agent and end up in a command line the user may
// run, so anything a shell treats specially (quotes, spaces, ; & | $ etc.) is
// rejected here.
var safeRe = regexp.MustCompile(`^[A-Za-z0-9/_.:@-]{1,160}$`)

// Command returns the pathfinder-sign command line for this request, using plain
// flags so it needs no JSON quoting in any shell. Call it only after Validate.
func (r *Request) Command() string {
	t := r.Transfer
	parts := []string{"pathfinder-sign",
		"--pathfinder-url", r.PathfinderURL,
		"--chain-from", t.ChainFrom, "--token-from", t.TokenFrom, "--amount", t.AmountIn,
		"--chain-to", t.ChainTo}
	add := func(flag, v string) {
		if v != "" {
			parts = append(parts, flag, v)
		}
	}
	add("--token-to", t.TokenTo)
	add("--recipient", t.Recipient)
	add("--mode", t.Mode)
	if t.SlippageBps != 0 {
		parts = append(parts, "--slippage-bps", fmt.Sprint(t.SlippageBps))
	}
	return strings.Join(parts, " ")
}

// Validate checks the request before anything is opened or contacted.
func (r *Request) Validate() error {
	if !urlRe.MatchString(r.PathfinderURL) {
		return fmt.Errorf("pathfinder_url %q has characters that are not allowed", r.PathfinderURL)
	}
	u, err := url.Parse(r.PathfinderURL)
	if err != nil || u.Host == "" {
		return fmt.Errorf("pathfinder_url %q is not a valid URL", r.PathfinderURL)
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && isLoopback(u.Hostname())) {
		return fmt.Errorf("pathfinder_url must be https (plain http is only allowed for localhost)")
	}
	t := r.Transfer
	if t.ChainFrom == "" || t.TokenFrom == "" || t.ChainTo == "" {
		return errors.New("request needs chain_from, token_from and chain_to")
	}
	for name, v := range map[string]string{
		"chain_from": t.ChainFrom, "token_from": t.TokenFrom, "chain_to": t.ChainTo,
		"token_to": t.TokenTo, "recipient": t.Recipient,
	} {
		if v != "" && !safeRe.MatchString(v) {
			return fmt.Errorf("%s contains characters that are not allowed (letters, digits and / _ . : @ - only)", name)
		}
	}
	if !amountRe.MatchString(t.AmountIn) || strings.Trim(t.AmountIn, "0") == "" {
		return errors.New("amount_in must be a positive integer in base units")
	}
	if t.Mode != "" && t.Mode != "smart" && t.Mode != "manual" {
		return errors.New(`mode must be "smart" or "manual"`)
	}
	return nil
}

func isLoopback(h string) bool {
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// Caller calls tools on the remote Pathfinder MCP server.
type Caller interface {
	CallTool(ctx context.Context, name string, args map[string]any) (*sdk.CallToolResult, error)
}

// RemoteCaller connects to a Pathfinder MCP endpoint over streamable HTTP.
type RemoteCaller struct {
	session *sdk.ClientSession
}

// Dial connects to <baseURL>/mcp.
func Dial(ctx context.Context, baseURL, version string) (*RemoteCaller, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/mcp"
	c := sdk.NewClient(&sdk.Implementation{Name: "pathfinder-sign", Version: version}, nil)
	s, err := c.Connect(ctx, &sdk.StreamableClientTransport{Endpoint: endpoint}, nil)
	if err != nil {
		return nil, fmt.Errorf("could not connect to %s: %w", endpoint, err)
	}
	return &RemoteCaller{session: s}, nil
}

// CallTool implements Caller.
func (r *RemoteCaller) CallTool(ctx context.Context, name string, args map[string]any) (*sdk.CallToolResult, error) {
	return r.session.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
}

// Close ends the MCP session.
func (r *RemoteCaller) Close() error { return r.session.Close() }

// Server is the loopback web server for one signing run.
type Server struct {
	req    *Request
	remote Caller
	token  string

	mu     sync.Mutex
	listen net.Listener
	result chan Result
	once   sync.Once
}

// New builds a Server. The request must already be valid.
func New(req *Request, remote Caller) (*Server, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return &Server{req: req, remote: remote, token: hex.EncodeToString(b), result: make(chan Result, 1)}, nil
}

// Listen binds 127.0.0.1 (port 0 picks a free port) and returns the URL to open.
func (s *Server) Listen(port int) (string, error) {
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.listen = l
	s.mu.Unlock()
	return fmt.Sprintf("http://%s/?t=%s", l.Addr().String(), s.token), nil
}

// Serve runs until ctx ends or the page reports a result, and returns it.
func (s *Server) Serve(ctx context.Context, timeout time.Duration) Result {
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(s.listen) }()
	defer func() {
		sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	select {
	case r := <-s.result:
		time.Sleep(300 * time.Millisecond) // let the page receive its response
		return r
	case <-time.After(timeout):
		return Result{Status: "timeout", Error: fmt.Sprintf("no result within %s", timeout)}
	case <-ctx.Done():
		return Result{Status: "cancelled", Error: "interrupted"}
	}
}

// Handler returns the HTTP handler (exported for tests).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.page)
	mux.HandleFunc("GET /api/request", s.guard(s.getRequest))
	mux.HandleFunc("POST /api/call", s.guard(s.call))
	mux.HandleFunc("POST /api/result", s.guard(s.postResult))
	return s.hostCheck(mux)
}

// hostCheck rejects requests whose Host or Origin is not this loopback server.
// That stops DNS-rebinding and other websites from talking to the program.
func (s *Server) hostCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		addr := s.listen.Addr().(*net.TCPAddr)
		s.mu.Unlock()
		okHost := map[string]bool{
			fmt.Sprintf("127.0.0.1:%d", addr.Port): true,
			fmt.Sprintf("localhost:%d", addr.Port): true,
		}
		if !okHost[r.Host] {
			http.Error(w, "bad host", http.StatusForbidden)
			return
		}
		if o := r.Header.Get("Origin"); o != "" && o != "http://"+r.Host {
			http.Error(w, "bad origin", http.StatusForbidden)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) tokenOK(got string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) == 1
}

func (s *Server) guard(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.tokenOK(r.Header.Get("X-Spectra-Token")) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}

func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	if !s.tokenOK(r.URL.Query().Get("t")) {
		http.Error(w, "unauthorized: open the exact link printed by pathfinder-sign", http.StatusUnauthorized)
		return
	}
	boot, _ := json.Marshal(map[string]string{"token": s.token})
	inject := "<script>window.__SPECTRA_LOCAL__=" + strings.ReplaceAll(string(boot), "<", `<`) + ";</script>"
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Only the page's own inline script runs; nothing is loaded from elsewhere.
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	_, _ = io.WriteString(w, strings.Replace(web.SignerHTML, placeholder, inject, 1))
}

// getRequest returns what the page needs to start: the transfer plus a route
// preview (which lists the chains that need an address).
func (s *Server) getRequest(w http.ResponseWriter, r *http.Request) {
	t := s.req.Transfer
	args := map[string]any{
		"chain_from": t.ChainFrom, "token_from": t.TokenFrom, "amount_in": t.AmountIn,
		"chain_to": t.ChainTo, "token_to": t.TokenTo, "smart_route": t.Mode != "manual",
		"slippage_bps": t.SlippageBps,
	}
	res, err := s.remote.CallTool(r.Context(), "find_route", args)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	text := resultText(res)
	if res.IsError {
		http.Error(w, "no route: "+text, http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"request": t, "preview": json.RawMessage(text)})
}

type callBody struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

func (s *Server) call(w http.ResponseWriter, r *http.Request) {
	var b callBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&b); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if !allowedTools[b.Name] {
		http.Error(w, "tool not allowed", http.StatusForbidden)
		return
	}
	res, err := s.remote.CallTool(r.Context(), b.Name, b.Arguments)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Server) postResult(w http.ResponseWriter, r *http.Request) {
	var res Result
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&res); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	switch res.Status {
	case "completed":
		if len(res.Transactions) == 0 {
			http.Error(w, "a completed result needs transactions", http.StatusBadRequest)
			return
		}
	case "failed":
		// A transaction was sent but did not end well. The hashes matter: the agent must be able to look into it.
		if res.Error == "" {
			http.Error(w, "a failed result needs an error", http.StatusBadRequest)
			return
		}
		if len(res.Error) > 600 {
			res.Error = res.Error[:600]
		}
	default:
		http.Error(w, "status must be completed or failed", http.StatusBadRequest)
		return
	}
	s.once.Do(func() { s.result <- res })
	w.WriteHeader(http.StatusNoContent)
}

func resultText(res *sdk.CallToolResult) string {
	var sb strings.Builder
	for _, c := range res.Content {
		if t, ok := c.(*sdk.TextContent); ok {
			sb.WriteString(t.Text)
		}
	}
	return sb.String()
}
