package sqsquery

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
)

var log zerolog.Logger

func init() {
	out := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}
	log = zerolog.New(out).With().Timestamp().Str("component", "sqs").Logger()
}

// SqsQueryClient provides access to the Osmosis SQS API with failover support.
// It maintains a primary endpoint and can automatically switch to backup endpoints
// when the primary is unavailable.
type SqsQueryClient struct {
	httpClient     *http.Client
	urls           []string
	healthyURLs    []string
	mu             sync.RWMutex
	healthChecker  *healthChecker
	failoverConfig FailoverConfig
}

// FailoverConfig controls failover behavior
type FailoverConfig struct {
	// MaxRetries is the number of times to retry a failed request on the current endpoint
	MaxRetries int
	// RetryDelay is the initial delay between retries (doubles with each retry)
	RetryDelay time.Duration
	// HealthCheckInterval is how often to check if the primary endpoint is back up
	HealthCheckInterval time.Duration
	// Timeout is the HTTP request timeout
	Timeout time.Duration
}

// DefaultFailoverConfig returns sensible defaults for failover behavior
func DefaultFailoverConfig() FailoverConfig {
	return FailoverConfig{
		MaxRetries:          2,
		RetryDelay:          500 * time.Millisecond,
		HealthCheckInterval: 30 * time.Second,
		Timeout:             10 * time.Second,
	}
}

// healthChecker periodically checks if the endpoints are healthy
type healthChecker struct {
	client    *SqsQueryClient
	stopCh    chan struct{}
	stoppedCh chan struct{}
	isRunning bool
	mu        sync.Mutex
}

// NewSqsQueryClient creates a new SqsQueryClient with a single endpoint (backward compatible)
func NewSqsQueryClient(urls []string) *SqsQueryClient {
	return NewSqsQueryClientWithFailover(urls, DefaultFailoverConfig())
}

// NewSqsQueryClientWithFailover creates a new SqsQueryClient with failover support
func NewSqsQueryClientWithFailover(urls []string, config FailoverConfig) *SqsQueryClient {
	// Validate the primary URL
	for _, u := range urls {
		if _, err := url.Parse(u); err != nil {
			log.Fatal().Err(err).Str("url", u).Msg("Failed to parse API URL")
			return nil
		}
	}

	healthyURLs := make([]string, len(urls))
	copy(healthyURLs, urls)

	client := &SqsQueryClient{
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		urls:           urls,
		healthyURLs:    healthyURLs,
		failoverConfig: config,
	}

	// Start health checker if we have backup URLs
	if len(urls) > 1 {
		client.startHealthChecker()
	}

	log.Info().
		Strs("urls", urls).
		Msg("SQS client initialized")
	return client
}

// startHealthChecker starts the background health checker goroutine
func (c *SqsQueryClient) startHealthChecker() {
	c.healthChecker = &healthChecker{
		client:    c,
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
	}
	c.healthChecker.start()
}

func (h *healthChecker) start() {
	h.mu.Lock()
	if h.isRunning {
		h.mu.Unlock()
		return
	}
	h.isRunning = true
	h.mu.Unlock()

	go func() {
		defer close(h.stoppedCh)
		ticker := time.NewTicker(h.client.failoverConfig.HealthCheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-h.stopCh:
				return
			case <-ticker.C:
				h.checkAndRestore()
			}
		}
	}()
}

func (h *healthChecker) stop() {
	h.mu.Lock()
	if !h.isRunning {
		h.mu.Unlock()
		return
	}
	h.mu.Unlock()

	close(h.stopCh)
	<-h.stoppedCh
}

// checkAndRestore checks if the primary endpoint is healthy and restores it if so
func (h *healthChecker) checkAndRestore() {
	urls := h.client.urls

	for _, url := range urls {
		if h.client.isEndpointHealthy(url) {
			if !slices.Contains(h.client.healthyURLs, url) {
				h.client.healthyURLs = append(h.client.healthyURLs, url)
			}
			log.Info().Str("url", url).Msg("Endpoint is healthy")
			return
		} else {
			h.client.healthyURLs = slices.DeleteFunc(h.client.healthyURLs, func(u string) bool {
				log.Warn().Str("url", u).Msg("Endpoint is unhealthy")
				return u == url
			})
			return
		}
	}
}

// isEndpointHealthy checks if an endpoint is responding
func (c *SqsQueryClient) isEndpointHealthy(endpoint string) bool {
	// Try a simple health check on the endpoint's swagger page
	healthURL := fmt.Sprintf("%s/swagger/index.html", endpoint)
	resp, err := c.httpClient.Get(healthURL)
	if err != nil {
		log.Debug().Err(err).Str("url", healthURL).Msg("Health check failed")
		return false
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	log.Debug().Str("url", healthURL).Int("status", resp.StatusCode).Msg("Health check response")
	return resp.StatusCode == http.StatusOK
}

// getCurrentURL returns the current active endpoint
func (c *SqsQueryClient) getRandomHealthyURL() string {
	if len(c.healthyURLs) == 0 {
		return ""
	}
	return c.healthyURLs[rand.Intn(len(c.healthyURLs))]
}

// Close stops the health checker and cleans up resources
func (c *SqsQueryClient) Close() {
	if c.healthChecker != nil {
		c.healthChecker.stop()
	}
}

// doRequestWithFailover performs an HTTP GET request with retry and failover logic
func (c *SqsQueryClient) doRequestWithFailover(path string) ([]byte, error) {
	var lastErr error
	retryDelay := c.failoverConfig.RetryDelay

	// Try on current endpoint with retries
	for attempt := 0; attempt <= c.failoverConfig.MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(retryDelay)
			retryDelay *= 2
		}

		fullURL := c.getRandomHealthyURL() + path
		resp, err := c.httpClient.Get(fullURL)
		if err != nil {
			lastErr = err
			continue
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
			continue
		}

		return body, nil
	}

	// Current endpoint failed, try failover
	if len(c.healthyURLs) > 0 && c.getRandomHealthyURL() != "" {
		// Retry once on the new endpoint
		fullURL := c.getRandomHealthyURL() + path
		resp, err := c.httpClient.Get(fullURL)
		if err != nil {
			return nil, fmt.Errorf("failover request failed: %w (original: %w)", err, lastErr)
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if err != nil {
			return nil, fmt.Errorf("failover read failed: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failover HTTP %d: %s", resp.StatusCode, string(body))
		}

		return body, nil
	}

	return nil, fmt.Errorf("request failed after %d retries: %w", c.failoverConfig.MaxRetries+1, lastErr)
}

/*
GetRoute returns the best quote it can compute for the exact in or exact out token swap method.

For exact amount in swap method, the tokenIn and tokenOutDenom are required.
For exact amount out swap method, the tokenOut and tokenInDenom are required.
Mixing swap method parameters in other way than specified will result in an error.

When singleRoute parameter is set to true, it gives the best single quote while excluding splits.

No 2 methods can be used together.
So when using this query, you can only use one of the following parameters:
- tokenIn and tokenOutDenom
- tokenOut and tokenInDenom
*/
func (c *SqsQueryClient) GetRoute(
	tokenIn, tokenOut *TokenRequest,
	tokenInDenom, tokenOutDenom *string,
	singleRoute bool) (RouteTokenResponse, error) {
	// check if the tokenIn and tokenOut are not nil
	// there must be at least one of them not nil
	if tokenIn == nil && tokenOut == nil {
		return RouteTokenResponse{}, errors.New("tokenIn and tokenOut cannot be nil")
	}
	if tokenIn != nil && tokenOut != nil {
		return RouteTokenResponse{}, errors.New("tokenIn and tokenOut cannot be used together")
	}

	if tokenInDenom == nil && tokenOutDenom == nil {
		return RouteTokenResponse{}, errors.New("tokenInDenom or tokenOutDenom is required")
	}
	if tokenInDenom != nil && tokenOutDenom != nil {
		return RouteTokenResponse{}, errors.New("tokenInDenom and tokenOutDenom cannot be used together")
	}

	var path string
	if tokenIn != nil && tokenOutDenom != nil {
		tokenInParam := url.QueryEscape(tokenIn.Amount + tokenIn.Denom)
		tokenOutDenomParam := url.QueryEscape(*tokenOutDenom)
		path = fmt.Sprintf(
			"/router/quote?tokenIn=%s&tokenOutDenom=%s&singleRoute=%t&humanDenoms=false&applyExponents=false&appendBaseFee=true",
			tokenInParam, tokenOutDenomParam, singleRoute,
		)
	} else if tokenOut != nil && tokenInDenom != nil {
		tokenOutParam := url.QueryEscape(tokenOut.Amount + tokenOut.Denom)
		tokenInDenomParam := url.QueryEscape(*tokenInDenom)
		path = fmt.Sprintf(
			"/router/quote?tokenOut=%s&tokenInDenom=%s&singleRoute=%t&humanDenoms=false&applyExponents=false&appendBaseFee=true",
			tokenOutParam, tokenInDenomParam, singleRoute,
		)
	} else {
		return RouteTokenResponse{}, errors.New("invalid parameters")
	}

	body, err := c.doRequestWithFailover(path)
	if err != nil {
		return RouteTokenResponse{}, err
	}
	var routeTokenResponse RouteTokenResponse
	if err := json.Unmarshal(body, &routeTokenResponse); err != nil {
		return RouteTokenResponse{}, fmt.Errorf("failed to parse route response: %w", err)
	}
	return routeTokenResponse, nil
}

// GetTokenPrice fetches the price of a token in USD terms
func (c *SqsQueryClient) GetTokenPrice(tokenDenom string) (decimal.Decimal, error) {
	path := fmt.Sprintf("/token-price?tokenDenom=%s", url.QueryEscape(tokenDenom))

	body, err := c.doRequestWithFailover(path)
	if err != nil {
		return decimal.Decimal{}, err
	}

	/*The json is in very strange format from which I can't create a type
	it usually contains:
	{
		"ibc/denom_inserted": {
			"ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4": "2.001"
		}
	}
	The denom is kinda the key here, the second value is probably USDC, however I haven't tested
	this more, it could probably be even something like USDT on some pairs and there are some other
	variations of ibc denoms for stablecoins.
	So just collect the string and turn it into a decimal.Decimal
	*/
	var tokenPriceResponse map[string]any
	if err := json.Unmarshal(body, &tokenPriceResponse); err != nil {
		return decimal.Decimal{}, fmt.Errorf("failed to parse price response: %w", err)
	}

	tokenPrice, ok := tokenPriceResponse[tokenDenom].(map[string]any)
	if !ok {
		return decimal.Decimal{}, fmt.Errorf("token price error when unmarshalling json, %s", tokenPriceResponse)
	}
	for _, price := range tokenPrice {
		priceStr, ok := price.(string)
		if !ok {
			continue
		}
		priceDecimal, err := decimal.NewFromString(priceStr)
		if err != nil {
			return decimal.Decimal{}, err
		}
		return priceDecimal, nil
	}

	return decimal.Decimal{}, errors.New("token price not found")
}

// GetAllPossibleRoutes returns all possible routes between two tokens
func (c *SqsQueryClient) GetAllPossibleRoutes(tokenInDenom, tokenOutDenom string) (AllPossibleRoutesResponse, error) {
	path := fmt.Sprintf(
		"/router/routes?tokenInDenom=%s&tokenOutDenom=%s",
		url.QueryEscape(tokenInDenom), url.QueryEscape(tokenOutDenom),
	)

	body, err := c.doRequestWithFailover(path)
	if err != nil {
		return AllPossibleRoutesResponse{}, err
	}

	var allPossibleRoutesResponse AllPossibleRoutesResponse
	if err := json.Unmarshal(body, &allPossibleRoutesResponse); err != nil {
		return AllPossibleRoutesResponse{}, fmt.Errorf("failed to parse routes response: %w", err)
	}
	return allPossibleRoutesResponse, nil
}
