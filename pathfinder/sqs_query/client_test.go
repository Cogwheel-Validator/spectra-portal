package sqsquery

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zeebo/assert"
)

// fastFailoverConfig keeps retries fast so failure paths don't slow the suite down.
func fastFailoverConfig() FailoverConfig {
	return FailoverConfig{
		MaxRetries:          2,
		RetryDelay:          time.Millisecond,
		HealthCheckInterval: time.Hour, // never fires during a test
		Timeout:             2 * time.Second,
	}
}

// sampleQuoteResponse is a realistic (trimmed) SQS /router/quote response.
func sampleQuoteResponse(amountIn, tokenOutDenom string) RouteTokenResponse {
	return RouteTokenResponse{
		AmountIn: struct {
			Denom  string `json:"denom"`
			Amount string `json:"amount"`
		}{Denom: "ibc/ATOM", Amount: amountIn},
		AmountOut: "2500000",
		Route: []Route{
			{
				Pools: []Pool{
					{ID: 1400, Type: 2, SpreadFactor: "0.002", TokenOutDenom: tokenOutDenom, TakerFee: "0.001"},
				},
				OutAmount: "2500000",
				InAmount:  amountIn,
			},
		},
		EffectiveFee: "0.003",
		PriceImpact:  "0.011",
	}
}

func TestGetRoute_ExactIn(t *testing.T) {
	var gotQuery atomic.Value

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, r.URL.Path, "/router/quote")
		gotQuery.Store(r.URL.Query())

		tokenOut := r.URL.Query().Get("tokenOutDenom")
		resp := sampleQuoteResponse("1000000", tokenOut)
		w.Header().Set("Content-Type", "application/json")
		assert.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	client := NewSqsQueryClientWithFailover([]string{server.URL}, fastFailoverConfig())
	defer client.Close()

	tokenIn := &TokenRequest{Denom: "ibc/ATOM", Amount: "1000000"}
	tokenOutDenom := "uosmo"

	response, err := client.GetRoute(tokenIn, nil, nil, &tokenOutDenom, true)
	assert.NoError(t, err)

	// Verify the query the mock SQS actually received
	query := gotQuery.Load().(url.Values)
	assert.Equal(t, query.Get("tokenIn"), "1000000ibc/ATOM")
	assert.Equal(t, query.Get("tokenOutDenom"), "uosmo")
	assert.Equal(t, query.Get("singleRoute"), "true")
	assert.Equal(t, query.Get("humanDenoms"), "false")
	assert.Equal(t, query.Get("applyExponents"), "false")

	// Verify the response is parsed into the typed structure
	assert.Equal(t, response.AmountOut, "2500000")
	assert.Equal(t, response.AmountIn.Amount, "1000000")
	assert.Equal(t, response.EffectiveFee, "0.003")
	assert.Equal(t, len(response.Route), 1)
	assert.Equal(t, response.Route[0].Pools[0].ID, int32(1400))
	assert.Equal(t, response.Route[0].Pools[0].TokenOutDenom, "uosmo")
}

func TestGetRoute_ExactOut(t *testing.T) {
	var gotQuery atomic.Value

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery.Store(r.URL.Query())
		resp := sampleQuoteResponse("1010000", "uosmo")
		w.Header().Set("Content-Type", "application/json")
		assert.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	client := NewSqsQueryClientWithFailover([]string{server.URL}, fastFailoverConfig())
	defer client.Close()

	tokenOut := &TokenRequest{Denom: "uosmo", Amount: "2500000"}
	tokenInDenom := "ibc/ATOM"

	_, err := client.GetRoute(nil, tokenOut, &tokenInDenom, nil, false)
	assert.NoError(t, err)

	query := gotQuery.Load().(url.Values)
	assert.Equal(t, query.Get("tokenOut"), "2500000uosmo")
	assert.Equal(t, query.Get("tokenInDenom"), "ibc/ATOM")
	assert.Equal(t, query.Get("singleRoute"), "false")
}

func TestGetRoute_ParameterValidation(t *testing.T) {
	// No server needed - validation happens before any request
	client := NewSqsQueryClientWithFailover([]string{"http://127.0.0.1:1"}, fastFailoverConfig())
	defer client.Close()

	tokenIn := &TokenRequest{Denom: "uatom", Amount: "1"}
	tokenOut := &TokenRequest{Denom: "uosmo", Amount: "1"}
	denom := "uosmo"

	// Both token structs nil
	_, err := client.GetRoute(nil, nil, &denom, nil, false)
	assert.Error(t, err)

	// Both token structs set
	_, err = client.GetRoute(tokenIn, tokenOut, nil, &denom, false)
	assert.Error(t, err)

	// No denom at all
	_, err = client.GetRoute(tokenIn, nil, nil, nil, false)
	assert.Error(t, err)

	// Both denoms set
	_, err = client.GetRoute(tokenIn, nil, &denom, &denom, false)
	assert.Error(t, err)

	// Mismatched combination: tokenIn requires tokenOutDenom, not tokenInDenom
	_, err = client.GetRoute(tokenIn, nil, &denom, nil, false)
	assert.Error(t, err)
}

func TestGetRoute_RetriesUntilSuccess(t *testing.T) {
	var requests atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) <= 2 {
			http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		resp := sampleQuoteResponse("1000000", "uosmo")
		assert.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	client := NewSqsQueryClientWithFailover([]string{server.URL}, fastFailoverConfig())
	defer client.Close()

	tokenIn := &TokenRequest{Denom: "ibc/ATOM", Amount: "1000000"}
	tokenOutDenom := "uosmo"

	response, err := client.GetRoute(tokenIn, nil, nil, &tokenOutDenom, true)
	assert.NoError(t, err)
	assert.Equal(t, response.AmountOut, "2500000")
	// Two failures then a success on the third attempt
	assert.Equal(t, requests.Load(), int32(3))
}

func TestGetRoute_ExhaustsRetries(t *testing.T) {
	var requests atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewSqsQueryClientWithFailover([]string{server.URL}, fastFailoverConfig())
	defer client.Close()

	tokenIn := &TokenRequest{Denom: "ibc/ATOM", Amount: "1000000"}
	tokenOutDenom := "uosmo"

	_, err := client.GetRoute(tokenIn, nil, nil, &tokenOutDenom, true)
	assert.Error(t, err)
	// MaxRetries+1 attempts on the primary loop plus one final failover attempt
	assert.Equal(t, requests.Load(), int32(4))
}

func TestGetRoute_MalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("this is not json"))
	}))
	defer server.Close()

	client := NewSqsQueryClientWithFailover([]string{server.URL}, fastFailoverConfig())
	defer client.Close()

	tokenIn := &TokenRequest{Denom: "ibc/ATOM", Amount: "1000000"}
	tokenOutDenom := "uosmo"

	_, err := client.GetRoute(tokenIn, nil, nil, &tokenOutDenom, true)
	assert.Error(t, err)
}

func TestGetTokenPrice(t *testing.T) {
	const denom = "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, r.URL.Path, "/token-price")
		assert.Equal(t, r.URL.Query().Get("tokenDenom"), denom)

		// SQS returns the price keyed twice: outer key is the queried denom,
		// inner key is the quote stablecoin denom.
		payload := map[string]map[string]string{
			denom: {"ibc/SOMESTABLECOIN": "2.001"},
		}
		assert.NoError(t, json.NewEncoder(w).Encode(payload))
	}))
	defer server.Close()

	client := NewSqsQueryClientWithFailover([]string{server.URL}, fastFailoverConfig())
	defer client.Close()

	price, err := client.GetTokenPrice(denom)
	assert.NoError(t, err)
	assert.Equal(t, price.String(), "2.001")
}

func TestGetTokenPrice_TokenMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Response for some other denom than the one requested
		payload := map[string]map[string]string{
			"ibc/OTHER": {"ibc/STABLE": "1.0"},
		}
		assert.NoError(t, json.NewEncoder(w).Encode(payload))
	}))
	defer server.Close()

	client := NewSqsQueryClientWithFailover([]string{server.URL}, fastFailoverConfig())
	defer client.Close()

	_, err := client.GetTokenPrice("ibc/REQUESTED")
	assert.Error(t, err)
}

func TestGetAllPossibleRoutes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, r.URL.Path, "/router/routes")
		assert.Equal(t, r.URL.Query().Get("tokenInDenom"), "ibc/ATOM")
		assert.Equal(t, r.URL.Query().Get("tokenOutDenom"), "uosmo")

		resp := AllPossibleRoutesResponse{
			Routes: []AllRoutes{
				{
					Pools: []AllPools{
						{ID: 1, TokenInDenom: "ibc/ATOM", TokenOutDenom: "uosmo"},
					},
				},
				{
					Pools: []AllPools{
						{ID: 2, TokenInDenom: "ibc/ATOM", TokenOutDenom: "uion"},
						{ID: 3, TokenInDenom: "uion", TokenOutDenom: "uosmo"},
					},
				},
			},
		}
		assert.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	client := NewSqsQueryClientWithFailover([]string{server.URL}, fastFailoverConfig())
	defer client.Close()

	response, err := client.GetAllPossibleRoutes("ibc/ATOM", "uosmo")
	assert.NoError(t, err)
	assert.Equal(t, len(response.Routes), 2)
	assert.Equal(t, len(response.Routes[1].Pools), 2)
	assert.Equal(t, response.Routes[1].Pools[1].TokenOutDenom, "uosmo")
}

func TestGetRoute_FailoverToSecondEndpoint(t *testing.T) {
	// Primary endpoint refuses connections (closed immediately), backup works.
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	dead.Close()

	var backupHits atomic.Int32
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backupHits.Add(1)
		resp := sampleQuoteResponse("1000000", "uosmo")
		assert.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer backup.Close()

	client := NewSqsQueryClientWithFailover([]string{dead.URL, backup.URL}, fastFailoverConfig())
	defer client.Close()

	tokenIn := &TokenRequest{Denom: "ibc/ATOM", Amount: "1000000"}
	tokenOutDenom := "uosmo"

	// Endpoint selection is random per attempt; with 3 retries + 1 failover try
	// against [dead, backup], run a few times to make hitting the backup certain.
	succeeded := false
	for range 5 {
		response, err := client.GetRoute(tokenIn, nil, nil, &tokenOutDenom, true)
		if err == nil && response.AmountOut == "2500000" {
			succeeded = true
			break
		}
	}
	assert.True(t, succeeded)
	assert.True(t, backupHits.Load() >= 1)
}
