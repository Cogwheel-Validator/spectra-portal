package query

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DenomQuerier provides methods to query and resolve IBC denoms on a chain.
// It uses a simple HTTP client without the full REST client validation overhead.
type DenomQuerier struct {
	baseURL       string
	client        *http.Client
	retryAttempts int
	retryDelay    time.Duration
}

// NewDenomQuerier creates a querier for a single REST endpoint.
func NewDenomQuerier(baseURL string, timeout time.Duration, retryAttempts int, retryDelay time.Duration) *DenomQuerier {
	return &DenomQuerier{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		client: &http.Client{
			Timeout: timeout,
		},
		retryAttempts: retryAttempts,
		retryDelay:    retryDelay,
	}
}

// DenomTraceInfo represents a single denom trace from a chain.
type DenomTraceInfo struct {
	Path      string // e.g., "transfer/channel-2"
	BaseDenom string // e.g., "uatone"
}

// ParsedDenomTrace contains parsed information from a denom trace.
type ParsedDenomTrace struct {
	Path      string   // Full path, e.g., "transfer/channel-2/transfer/channel-75"
	BaseDenom string   // Original denom, e.g., "uatone"
	IBCDenom  string   // Full IBC denom hash, e.g., "ibc/ABC123..."
	Channels  []string // Parsed channel IDs in order
	Ports     []string // Parsed port IDs in order
	HopCount  int      // Number of IBC hops
	FirstHop  string   // First channel in path (direct connection to this chain)
}

// QueryAllDenomTraces fetches all denom traces from the chain.
func (q *DenomQuerier) QueryAllDenomTraces() ([]DenomTraceInfo, error) {
	traces := make([]DenomTraceInfo, 0)
	nextKey := ""

	for {
		url := fmt.Sprintf("%s/ibc/apps/transfer/v1/denom_traces", q.baseURL)
		if nextKey != "" {
			url = fmt.Sprintf("%s?pagination.key=%s", url, nextKey)
		}

		body, err := q.doGetWithRetry(url)
		if err != nil {
			return nil, fmt.Errorf("failed to query denom traces: %w", err)
		}

		var response struct {
			DenomTraces []struct {
				Path      string `json:"path"`
				BaseDenom string `json:"base_denom"`
			} `json:"denom_traces"`
			Pagination struct {
				NextKey string `json:"next_key"`
			} `json:"pagination"`
		}

		if err := json.Unmarshal(body, &response); err != nil {
			return nil, fmt.Errorf("failed to parse denom traces response: %w", err)
		}

		for _, t := range response.DenomTraces {
			traces = append(traces, DenomTraceInfo{
				Path:      t.Path,
				BaseDenom: t.BaseDenom,
			})
		}

		if response.Pagination.NextKey == "" {
			break
		}
		nextKey = response.Pagination.NextKey
	}

	return traces, nil
}

// QueryDenomHash queries the chain for the IBC denom hash of a given trace.
// trace should be in format "transfer/channel-X/denom" or "transfer/channel-X/transfer/channel-Y/denom"
func (q *DenomQuerier) QueryDenomHash(trace string) (string, error) {
	escapedTrace := url.PathEscape(trace)
	url := fmt.Sprintf("%s/ibc/apps/transfer/v1/denom_hashes/%s", q.baseURL, escapedTrace)

	body, err := q.doGetWithRetry(url)
	if err != nil {
		return "", fmt.Errorf("failed to query denom hash: %w", err)
	}

	var response struct {
		Hash string `json:"hash"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("failed to parse denom hash response: %w", err)
	}

	return fmt.Sprintf("ibc/%s", response.Hash), nil
}

// ComputeDenomHash computes the IBC denom hash locally without querying the chain.
// This is useful when we don't have a healthy endpoint but know the path.
// trace should be in format "transfer/channel-X/denom"
func ComputeDenomHash(trace string) string {
	hash := sha256.Sum256([]byte(trace))
	return fmt.Sprintf("ibc/%s", strings.ToUpper(hex.EncodeToString(hash[:])))
}

// ParseDenomTrace parses a denom trace into structured information.
func ParseDenomTrace(path, baseDenom string) *ParsedDenomTrace {
	parsed := &ParsedDenomTrace{
		Path:      path,
		BaseDenom: baseDenom,
		Channels:  make([]string, 0),
		Ports:     make([]string, 0),
	}

	// Parse path segments: "transfer/channel-2/transfer/channel-75"
	// Each hop is "port/channel"
	segments := strings.Split(path, "/")

	for i := 0; i < len(segments)-1; i += 2 {
		if i+1 < len(segments) {
			parsed.Ports = append(parsed.Ports, segments[i])
			parsed.Channels = append(parsed.Channels, segments[i+1])
		}
	}

	parsed.HopCount = len(parsed.Channels)
	if parsed.HopCount > 0 {
		parsed.FirstHop = parsed.Channels[0]
	}

	// Compute IBC denom hash
	fullTrace := fmt.Sprintf("%s/%s", path, baseDenom)
	parsed.IBCDenom = ComputeDenomHash(fullTrace)

	return parsed
}

// FilterTracesByChannel filters denom traces to only those that came through a specific channel.
func FilterTracesByChannel(traces []DenomTraceInfo, channelID string) []ParsedDenomTrace {
	filtered := make([]ParsedDenomTrace, 0)

	for _, trace := range traces {
		parsed := ParseDenomTrace(trace.Path, trace.BaseDenom)
		// Check if this trace's first hop matches our channel
		if parsed.FirstHop == channelID {
			filtered = append(filtered, *parsed)
		}
	}

	return filtered
}

// FilterDirectTraces filters for traces that came directly through a channel (single hop).
func FilterDirectTraces(traces []DenomTraceInfo, channelID string) []ParsedDenomTrace {
	filtered := make([]ParsedDenomTrace, 0)

	for _, trace := range traces {
		parsed := ParseDenomTrace(trace.Path, trace.BaseDenom)
		// Only single-hop traces through our channel
		if parsed.HopCount == 1 && parsed.FirstHop == channelID {
			filtered = append(filtered, *parsed)
		}
	}

	return filtered
}

func (q *DenomQuerier) doGetWithRetry(url string) ([]byte, error) {
	var lastErr error

	for attempt := 0; attempt <= q.retryAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(q.retryDelay)
		}

		resp, err := q.client.Get(url)
		if err != nil {
			lastErr = err
			continue
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				log.Printf("failed to close response body: %v", err)
			}
		}()

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = err
			continue
		}

		return body, nil
	}

	return nil, fmt.Errorf("request failed after %d attempts: %w", q.retryAttempts+1, lastErr)
}

// IsHealthy checks if the REST endpoint is healthy.
func (q *DenomQuerier) IsHealthy() bool {
	url := fmt.Sprintf("%s/cosmos/base/tendermint/v1beta1/node_info", q.baseURL)
	resp, err := q.client.Get(url)
	if err != nil {
		return false
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("failed to close response body: %v", err)
		}
	}()
	return resp.StatusCode == http.StatusOK
}
