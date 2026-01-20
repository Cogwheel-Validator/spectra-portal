package query

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/input"
)

const (
	status      = "status"
	net_info    = "net_info"
	abci_info   = "abci_info"
	block_query = "block"
)

func NewRpcClient(baseURLs []string, retryAttempts int, retryDelay time.Duration, timeout time.Duration) *RpcClient {
	return &RpcClient{
		BaseURLs: baseURLs,
		Client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return fmt.Errorf("redirect not allowed (count=%d) to %s", len(via), req.URL.String())
			},
		},
		RetryAttempts: retryAttempts,
		RetryDelay:    retryDelay,
	}
}

func (c *RpcClient) performRequest(url, method string, params map[string]any, result any) error {
	requestBody, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
		"id":      1,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal request body for method %s: %w", method, err)
	}

	// perform the request
	resp, err := c.Client.Post(url, "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		retryAttempt := 0
		for retryAttempt < c.RetryAttempts {
			resp, err = c.Client.Post(url, "application/json", bytes.NewBuffer(requestBody))
			if err == nil {
				break
			}
			retryAttempt++
			time.Sleep(c.RetryDelay)
		}
		if err != nil {
			return fmt.Errorf("failed to perform request %s method %s: %w", url, method, err)
		}
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("failed to close response body: %v", err)
		}
	}()

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}

func (c *RpcClient) QueryStatus(url string) (RpcStatusResponse, error) {
	var response RpcStatusResponse
	err := c.performRequest(url, status, nil, &response)
	if err != nil {
		return RpcStatusResponse{}, err
	}
	return response, nil
}

func (c *RpcClient) QueryAbciInfo(url string) (AbciInfoResponse, error) {
	var response AbciInfoResponse
	err := c.performRequest(url, abci_info, nil, &response)
	if err != nil {
		return AbciInfoResponse{}, err
	}
	return response, nil
}

// getMostCommonValue returns the key with the highest count in the map
func getMostCommonValue(counts map[string]int) string {
	if len(counts) == 0 {
		return ""
	}

	var mostCommon string
	maxCount := 0
	for key, count := range counts {
		if count > maxCount {
			maxCount = count
			mostCommon = key
		}
	}
	return mostCommon
}

// getMaxHeight returns the maximum value from a slice of integers
func getMaxHeight(heights []int) int {
	if len(heights) == 0 {
		return 0
	}

	max := heights[0]
	for _, h := range heights {
		if h > max {
			max = h
		}
	}
	return max
}

type endpointData struct {
	Height      int
	Version     string
	AbciAppName string
	ChainId     string
	Provider    string
}

// ValidateRpcEndpoints validates the RPC endpoints and returns a map of healthy endpoints
//
// Parameters:
// - endpoints - the input endpoints to validate
// - retryAttempts - the number of retry attempts to perform
// - retryDelay - the delay between retry attempts
// - timeout - the timeout for the request
//
// # Returns a map of healthy endpoints
//
// Depricated: The whole validation should happen within the validation package.
func ValidateRpcEndpoints(
	endpoints []input.APIEndpoint,
	retryAttempts int,
	retryDelay time.Duration,
	timeout time.Duration,
) map[URLProvider]bool {
	// Step 1: Collect validation data from all endpoints
	rawData := make(map[URLProvider]CollectedValidationData)
	for _, endpoint := range endpoints {
		c := NewRpcClient([]string{endpoint.URL}, retryAttempts, retryDelay, timeout)
		status, err := c.QueryStatus(endpoint.URL)
		if err != nil {
			log.Printf("failed to query status for %s: %v", endpoint.URL, err)
			continue
		}
		abciInfo, err := c.QueryAbciInfo(endpoint.URL)
		if err != nil {
			log.Printf("failed to query abci info for %s: %v", endpoint.URL, err)
			continue
		}
		rawData[URLProvider{URL: endpoint.URL, Provider: endpoint.Provider}] = CollectedValidationData{
			AbciInfo: abciInfo.Result.Response,
			Status:   status.Result,
		}
	}

	if len(rawData) == 0 {
		log.Printf("no endpoints returned valid data")
		return map[URLProvider]bool{}
	}

	// Step 2: Parse and filter by tx indexer
	processedData := make(map[URLProvider]endpointData)
	heights := make([]int, 0, len(rawData))
	versionCounts := make(map[string]int)
	abciAppCounts := make(map[string]int)
	chainIdCounts := make(map[string]int)

	for endpoint, data := range rawData {
		// Check tx indexer first
		if data.Status.NodeInfo.Other.TxIndex != "on" {
			log.Printf("tx indexer is not enabled for %s", endpoint.URL)
			continue
		}

		height, err := strconv.Atoi(data.Status.StatusSyncInfo.LatestBlockHeight)
		if err != nil {
			log.Printf("failed to convert latest block height for %s: %v", endpoint.URL, err)
			continue
		}

		epData := endpointData{
			Height:      height,
			Version:     data.AbciInfo.Version,
			AbciAppName: data.AbciInfo.Data,
			ChainId:     data.Status.NodeInfo.Network,
			Provider:    endpoint.Provider,
		}

		processedData[endpoint] = epData
		heights = append(heights, height)

		if epData.Version != "" {
			versionCounts[epData.Version]++
		}
		if epData.AbciAppName != "" {
			abciAppCounts[epData.AbciAppName]++
		}
		if epData.ChainId != "" {
			chainIdCounts[epData.ChainId]++
		}
	}

	if len(processedData) == 0 {
		log.Printf("no endpoints passed tx indexer check")
		return map[URLProvider]bool{}
	}

	// Step 3: Determine consensus values
	maxHeight := getMaxHeight(heights)
	expectedVersion := getMostCommonValue(versionCounts)
	expectedChainId := getMostCommonValue(chainIdCounts)
	expectedAbciApp := getMostCommonValue(abciAppCounts)

	// Step 4: Filter endpoints by consensus
	validEndpoints := make(map[URLProvider]bool)
	for endpoint, data := range processedData {
		// Check height (within 500 blocks of highest)
		if data.Height < maxHeight-500 {
			log.Printf("endpoint %s is behind by more than 500 blocks", endpoint.URL)
			continue
		}

		// Check version match
		if data.Version != expectedVersion {
			log.Printf("endpoint %s has different version: %s (expected: %s)", endpoint.URL, data.Version, expectedVersion)
			continue
		}

		// Check chain ID match
		if data.ChainId != expectedChainId {
			log.Printf("endpoint %s has different chain ID: %s (expected: %s)", endpoint.URL, data.ChainId, expectedChainId)
			continue
		}

		// Check ABCI app match
		if data.AbciAppName != expectedAbciApp {
			log.Printf("endpoint %s has different ABCI app: %s (expected: %s)", endpoint.URL, data.AbciAppName, expectedAbciApp)
			continue
		}

		validEndpoints[URLProvider{URL: endpoint.URL, Provider: data.Provider}] = true
	}

	return validEndpoints
}

func (c *RpcClient) QueryBlock(url string, block int) (RpcBlockResponse, error) {
	var response RpcBlockResponse
	err := c.performRequest(url, block_query, map[string]any{"height": block}, &response)
	if err != nil {
		return RpcBlockResponse{}, err
	}
	return response, nil
}
