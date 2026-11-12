package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/routeindex"
)

// maxResponseBytes bounds what we read from a chain endpoint.
const maxResponseBytes = 4 << 20

// ChainInfoFunc looks up a chain's configuration by ID.
type ChainInfoFunc func(chainID string) (routeindex.PathfinderChain, error)

// chainAPI queries a chain's REST (LCD) endpoints, with failover across the
// healthy endpoints from the chain config. Only endpoints from that config are
// ever contacted, never caller-supplied URLs.
type chainAPI struct {
	chain  ChainInfoFunc
	client *http.Client
}

func newChainAPI(chain ChainInfoFunc) *chainAPI {
	return &chainAPI{chain: chain, client: &http.Client{Timeout: 15 * time.Second}}
}

// errNoEndpoints is returned when a chain has no REST endpoints configured.
var errNoEndpoints = errors.New("no REST endpoints are configured for this chain (regenerate the pathfinder config with the config manager)")

// endpoints returns the configured REST endpoints for a chain.
func (c *chainAPI) endpoints(chainID string) ([]string, routeindex.PathfinderChain, error) {
	ch, err := c.chain(chainID)
	if err != nil {
		return nil, ch, fmt.Errorf("unknown chain %q", chainID)
	}
	if len(ch.Rests) == 0 {
		return nil, ch, errNoEndpoints
	}
	return ch.Rests, ch, nil
}

// apiError is a non-2xx response from a chain endpoint. 4xx other than 404 are
// authoritative (the node understood and refused), so there is no failover on them.
type apiError struct {
	Status int
	Body   string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("endpoint returned HTTP %d: %s", e.Status, e.Body)
}

// do sends a request to each endpoint in turn until one answers. Transport
// errors, 5xx and 404 fall through to the next endpoint; other 4xx return immediately.
func (c *chainAPI) do(ctx context.Context, chainID, method, path string, body []byte, out any) error {
	eps, _, err := c.endpoints(chainID)
	if err != nil {
		return err
	}
	// A "not found" only counts once every endpoint has said it: if any
	// endpoint failed for another reason, that failure is returned instead, so
	// a lagging node's 404 never hides a live node that was just unreachable.
	var notFound, other error
	for _, ep := range eps {
		var rdr io.Reader
		if body != nil {
			rdr = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(ep, "/")+path, rdr)
		if err != nil {
			other = err
			continue
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.client.Do(req)
		if err != nil {
			other = err
			continue
		}
		data, rerr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		_ = resp.Body.Close()
		if rerr != nil {
			other = rerr
			continue
		}
		// 5xx and 404 try the next endpoint: a pruned or lagging node answers
		// 404 for data that other nodes still have, so "not found" is only
		// real once every endpoint has said it.
		if resp.StatusCode >= 500 || resp.StatusCode == http.StatusNotFound {
			ae := &apiError{Status: resp.StatusCode, Body: snippet(data)}
			if isNotFound(ae) {
				notFound = ae
			} else {
				other = ae
			}
			continue
		}
		if resp.StatusCode >= 400 {
			return &apiError{Status: resp.StatusCode, Body: snippet(data)}
		}
		if out == nil {
			return nil
		}
		if err := json.Unmarshal(data, out); err != nil {
			other = fmt.Errorf("invalid JSON from endpoint: %w", err)
			continue
		}
		return nil
	}
	if other != nil {
		return fmt.Errorf("all REST endpoints failed for %s: %w", chainID, other)
	}
	if notFound != nil {
		return fmt.Errorf("every REST endpoint of %s reported not found: %w", chainID, notFound)
	}
	return fmt.Errorf("all REST endpoints failed for %s: %w", chainID, errNoEndpoints)
}

// isNotFound reports whether err is an endpoint saying the data does not exist.
func isNotFound(err error) bool {
	var ae *apiError
	return errors.As(err, &ae) && (ae.Status == http.StatusNotFound || strings.Contains(ae.Body, "not found"))
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		s = s[:300] + "..."
	}
	return s
}

// AccountInfo is what a signer needs to build a SignDoc.
type AccountInfo struct {
	Address       string `json:"address"`
	AccountNumber string `json:"account_number"`
	Sequence      string `json:"sequence"`
	// Exists is false for an address that has never been funded or used.
	Exists bool `json:"exists"`
}

func (c *chainAPI) account(ctx context.Context, chainID, address string) (*AccountInfo, error) {
	var resp struct {
		Account map[string]any `json:"account"`
	}
	err := c.do(ctx, chainID, http.MethodGet, "/cosmos/auth/v1beta1/accounts/"+url.PathEscape(address), nil, &resp)
	if isNotFound(err) {
		return &AccountInfo{Address: address, AccountNumber: "0", Sequence: "0", Exists: false}, nil
	}
	if err != nil {
		return nil, err
	}
	// Vesting and module wrappers nest the base account.
	acc := resp.Account
	for _, key := range []string{"base_vesting_account", "base_account"} {
		if inner, ok := acc[key].(map[string]any); ok {
			acc = inner
		}
	}
	info := &AccountInfo{Address: address, Exists: true, AccountNumber: asString(acc["account_number"]), Sequence: asString(acc["sequence"])}
	if info.AccountNumber == "" {
		info.AccountNumber = "0"
	}
	if info.Sequence == "" {
		info.Sequence = "0"
	}
	return info, nil
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return fmt.Sprintf("%.0f", t)
	}
	return ""
}

func (c *chainAPI) balances(ctx context.Context, chainID, address string) ([]map[string]string, error) {
	var resp struct {
		Balances []map[string]string `json:"balances"`
	}
	if err := c.do(ctx, chainID, http.MethodGet, "/cosmos/bank/v1beta1/balances/"+url.PathEscape(address)+"?pagination.limit=200", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Balances, nil
}

// txResult is the subset of GetTx response we surface.
type txResult struct {
	Found     bool             `json:"found"`
	Hash      string           `json:"tx_hash"`
	Height    string           `json:"height,omitempty"`
	Code      int              `json:"code"`
	Codespace string           `json:"codespace,omitempty"`
	RawLog    string           `json:"raw_log,omitempty"`
	GasWanted string           `json:"gas_wanted,omitempty"`
	GasUsed   string           `json:"gas_used,omitempty"`
	Timestamp string           `json:"timestamp,omitempty"`
	Events    []map[string]any `json:"-"`
}

func (c *chainAPI) tx(ctx context.Context, chainID, hash string) (*txResult, error) {
	var resp struct {
		TxResponse struct {
			Height    string           `json:"height"`
			TxHash    string           `json:"txhash"`
			Code      int              `json:"code"`
			Codespace string           `json:"codespace"`
			RawLog    string           `json:"raw_log"`
			GasWanted string           `json:"gas_wanted"`
			GasUsed   string           `json:"gas_used"`
			Timestamp string           `json:"timestamp"`
			Events    []map[string]any `json:"events"`
		} `json:"tx_response"`
	}
	err := c.do(ctx, chainID, http.MethodGet, "/cosmos/tx/v1beta1/txs/"+url.PathEscape(strings.ToUpper(hash)), nil, &resp)
	if isNotFound(err) {
		return &txResult{Found: false, Hash: strings.ToUpper(hash)}, nil
	}
	if err != nil {
		return nil, err
	}
	r := resp.TxResponse
	raw := r.RawLog
	if len(raw) > 600 {
		raw = raw[:600] + "..."
	}
	return &txResult{
		Found: true, Hash: r.TxHash, Height: r.Height, Code: r.Code, Codespace: r.Codespace,
		RawLog: raw, GasWanted: r.GasWanted, GasUsed: r.GasUsed, Timestamp: r.Timestamp, Events: r.Events,
	}, nil
}

// searchTxs queries transactions by event conditions. Cosmos SDK >= 0.50 uses
// a single `query` parameter; older versions repeat `events`.
func (c *chainAPI) searchTxs(ctx context.Context, chainID string, conditions []string, limit int) ([]txResult, error) {
	_, ch, err := c.endpoints(chainID)
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("order_by", "ORDER_BY_UNSPECIFIED")
	q.Set("pagination.limit", fmt.Sprint(limit))
	q.Set("limit", fmt.Sprint(limit))
	if sdkAtLeast050(ch.CosmosSdkVersion) {
		q.Set("query", strings.Join(conditions, " AND "))
	} else {
		for _, cnd := range conditions {
			q.Add("events", cnd)
		}
	}
	var resp struct {
		TxResponses []struct {
			Height string           `json:"height"`
			TxHash string           `json:"txhash"`
			Code   int              `json:"code"`
			Events []map[string]any `json:"events"`
		} `json:"tx_responses"`
	}
	if err := c.do(ctx, chainID, http.MethodGet, "/cosmos/tx/v1beta1/txs?"+q.Encode(), nil, &resp); err != nil {
		return nil, err
	}
	out := make([]txResult, 0, len(resp.TxResponses))
	for _, r := range resp.TxResponses {
		out = append(out, txResult{Found: true, Hash: r.TxHash, Height: r.Height, Code: r.Code, Events: r.Events})
	}
	return out, nil
}

// sdkAtLeast050 reports whether an SDK version like "v0.50.9" is >= 0.50. An
// unknown version is assumed modern.
func sdkAtLeast050(v string) bool {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if v == "" {
		return true
	}
	var major, minor int
	if _, err := fmt.Sscanf(v, "%d.%d", &major, &minor); err != nil {
		return true
	}
	return major > 0 || minor >= 50
}

// broadcast submits signed transaction bytes in SYNC mode.
func (c *chainAPI) broadcast(ctx context.Context, chainID string, txBytes []byte) (*txResult, error) {
	body, _ := json.Marshal(map[string]string{
		"tx_bytes": base64.StdEncoding.EncodeToString(txBytes),
		"mode":     "BROADCAST_MODE_SYNC",
	})
	var resp struct {
		TxResponse struct {
			TxHash    string `json:"txhash"`
			Code      int    `json:"code"`
			Codespace string `json:"codespace"`
			RawLog    string `json:"raw_log"`
		} `json:"tx_response"`
	}
	if err := c.do(ctx, chainID, http.MethodPost, "/cosmos/tx/v1beta1/txs", body, &resp); err != nil {
		return nil, err
	}
	r := resp.TxResponse
	return &txResult{Found: true, Hash: r.TxHash, Code: r.Code, Codespace: r.Codespace, RawLog: r.RawLog}, nil
}

func (c *chainAPI) packetCommitted(ctx context.Context, srcChain, port, channel, seq string) (bool, error) {
	var resp struct {
		Commitment string `json:"commitment"`
	}
	p := fmt.Sprintf("/ibc/core/channel/v1/channels/%s/ports/%s/packet_commitments/%s",
		url.PathEscape(channel), url.PathEscape(port), url.PathEscape(seq))
	err := c.do(ctx, srcChain, http.MethodGet, p, nil, &resp)
	if isNotFound(err) {
		return false, nil // no commitment stored: acked or timed out
	}
	if err != nil {
		return false, err
	}
	return resp.Commitment != "", nil
}

func (c *chainAPI) packetReceived(ctx context.Context, dstChain, port, channel, seq string) (bool, error) {
	var resp struct {
		Received bool `json:"received"`
	}
	p := fmt.Sprintf("/ibc/core/channel/v1/channels/%s/ports/%s/packet_receipts/%s",
		url.PathEscape(channel), url.PathEscape(port), url.PathEscape(seq))
	if err := c.do(ctx, dstChain, http.MethodGet, p, nil, &resp); err != nil {
		return false, err
	}
	return resp.Received, nil
}
