package query

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/Cogwheel-Validator/spectra-portal/config_manager/input"
)

// getWithContext issues a GET request bounded by timeout, avoiding the
// context-less http.Client.Get so a stalled or malicious endpoint can't hang
// past the configured deadline.
func getWithContext(client *http.Client, url string, timeout time.Duration) (*http.Response, error) {
	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return client.Do(req)
}

// GetRestStatus queries the REST node_info endpoint and returns the node status.
func GetRestStatus(
	endpoint input.APIEndpoint,
	retryAttempts int,
	retryDelay time.Duration,
	timeout time.Duration) (NodeStatus, error) {
	client := http.Client{
		Timeout: timeout,
	}
	fullURL := fmt.Sprintf("%s/cosmos/base/tendermint/v1beta1/node_info", endpoint.URL)
	resp, err := getWithContext(&client, fullURL, timeout)
	if err != nil {
		// retry
		attempt := 0
		for attempt < retryAttempts {
			resp, err = getWithContext(&client, fullURL, timeout)
			if err == nil {
				break
			}
			attempt++
			time.Sleep(retryDelay)
		}
		if err != nil {
			return NodeStatus{}, err
		}
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Failed to close response body: %v", err)
		}
	}()

	// check status code
	if resp.StatusCode != http.StatusOK {
		return NodeStatus{}, fmt.Errorf("endpoint returned status %d", resp.StatusCode)
	}

	// read the body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return NodeStatus{}, err
	}

	// unmarshal the body
	var response NodeInfoResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return NodeStatus{}, err
	}

	// collect only the data the program needs
	network := response.DefaultNodeInfo.Network
	version := response.DefaultNodeInfo.Version
	txIndex := response.DefaultNodeInfo.Other.TxIndex
	applicationVersion := response.ApplicationVersion
	appName := applicationVersion.AppName
	appVersion := applicationVersion.Version
	gitCommit := applicationVersion.GitCommit
	cosmosSdkVersion := applicationVersion.CosmosSdkVersion
	txIndexerBool := txIndex == "on"
	nodeStatus := NodeStatus{
		BaseUrl:          endpoint.URL,
		Provider:         endpoint.Provider,
		Network:          network,
		Version:          version,
		TxIndexer:        txIndexerBool,
		AppName:          appName,
		AppVersion:       appVersion,
		GitCommit:        gitCommit,
		CosmosSdkVersion: cosmosSdkVersion,
	}

	// return the node status
	return nodeStatus, nil
}

// ValidateRestEndpoints validates the REST endpoints and returns a map of healthy endpoints
//
// Parameters:
// - endpoints - the input endpoints to validate
// - retryAttempts - the number of retry attempts to perform
// - retryDelay - the delay between retry attempts
// - timeout - the timeout for the request
//
// # Returns a map of healthy endpoints
//
// Depricated: Use the new validator package instead
func ValidateRestEndpoints(
	endpoints []input.APIEndpoint,
	retryAttempts int,
	retryDelay time.Duration,
	timeout time.Duration,
) map[URLProvider]bool {
	chainIds := make(map[string]int)
	// Step 1: Collect node status from all endpoints
	nodeStatuses := make([]NodeStatus, 0, len(endpoints))
	for _, endpoint := range endpoints {
		nodeStatus, err := GetRestStatus(endpoint, retryAttempts, retryDelay, timeout)
		if err != nil {
			log.Printf("Failed to get REST status for %s: %v", endpoint.URL, err)
			continue
		}
		if !nodeStatus.TxIndexer {
			log.Printf("REST API for %s does not have tx indexer enabled", endpoint.URL)
			continue
		}
		nodeStatuses = append(nodeStatuses, nodeStatus)
		chainIds[nodeStatus.Network]++
	}

	if len(nodeStatuses) == 0 {
		log.Fatalf("No REST APIs are working and are for the matching chain")
	}

	mainChainId := ""
	maxCount := 0
	for chainId, count := range chainIds {
		if count > maxCount {
			maxCount = count
			mainChainId = chainId
		}
	}

	if mainChainId == "" {
		log.Fatalf("No main chain ID found")
	}

	// now that the chainId are collected remove any that are considered secondary
	filteredNodeStatuses := make([]NodeStatus, 0)
	for _, nodeStatus := range nodeStatuses {
		if nodeStatus.Network == mainChainId {
			filteredNodeStatuses = append(filteredNodeStatuses, nodeStatus)
		}
	}

	// Step 2: Count occurrences of each attribute
	binaryNames := make(map[string]int)
	binaryCommits := make(map[string]int)
	versions := make(map[string]int)

	for _, nodeStatus := range filteredNodeStatuses {
		if nodeStatus.AppName != "" {
			binaryNames[nodeStatus.AppName]++
		}
		if nodeStatus.Version != "" {
			versions[nodeStatus.Version]++
		}
		if nodeStatus.GitCommit != "" {
			binaryCommits[nodeStatus.GitCommit]++
		}
	}

	// Step 3: Find consensus values (most common)
	expectedBinaryName := getMostCommonValue(binaryNames)
	expectedVersion := getMostCommonValue(versions)
	expectedCommit := getMostCommonValue(binaryCommits)

	// Step 4: Filter endpoints that match consensus
	healthyEndpoints := make(map[URLProvider]bool)
	for _, nodeStatus := range filteredNodeStatuses {
		// Note: In case of network upgrades, validators may have different versions.
		// This strict matching ensures consistency but may need to be relaxed
		// for chains with staggered upgrade patterns.
		if nodeStatus.AppName == expectedBinaryName &&
			nodeStatus.Version == expectedVersion &&
			nodeStatus.GitCommit == expectedCommit {
			healthyEndpoints[URLProvider{URL: nodeStatus.BaseUrl, Provider: nodeStatus.Provider}] = true
		} else {
			log.Printf("Filtering out %s due to version mismatch (app: %s, version: %s, commit: %s)",
				nodeStatus.BaseUrl, nodeStatus.AppName, nodeStatus.Version, nodeStatus.GitCommit)
		}
	}

	if len(healthyEndpoints) == 0 {
		log.Printf("Warning: No endpoints match consensus values. Expected - app: %s, version: %s, commit: %s",
			expectedBinaryName, expectedVersion, expectedCommit)
	}

	return healthyEndpoints
}

// GetAdditionalNodeInfo gets the additional node info from the REST endpoint
//
// Parameters:
//
// - healthyRestEndpoint - the healthy REST endpoint to get the additional node info from
//
// Returns:
// - the additional node info
// - error if the request fails
//
// Only used within the client config generation for now
func GetAdditionalNodeInfo(healthyRestEndpoint string) (NodeInfoResponse, error) {
	timeout := 10 * time.Second
	client := http.Client{
		Timeout: timeout,
	}
	resp, err := getWithContext(&client, healthyRestEndpoint+"/cosmos/base/tendermint/v1beta1/node_info", timeout)
	if err != nil {
		return NodeInfoResponse{}, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Fatalf("Failed to close response body: %v", err)
		}
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return NodeInfoResponse{}, err
	}
	var response NodeInfoResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return NodeInfoResponse{}, err
	}
	return response, nil
}

// GetCosmosBlockHeights gets the block data from the REST API for a given block
//
// Parameters:
// - endpoint - the endpoint to get the block data from
// - block - the integer of the blocks to get the data from
//
// Returns:
// - the block data
// - map of the block data with the block number as the key
// - error if the request fails
func GetCosmosBlockHeights(
	endpoint input.APIEndpoint,
	retryAttempts int,
	retryDelay time.Duration,
	timeout time.Duration,
	block int) (BlockData, error) {
	client := http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// allow only 1 redirect
			if len(via) > 2 {
				return fmt.Errorf("redirect limit exceeded (count=%d) to %s", len(via), req.URL.String())
			}
			return nil
		},
	}

	fullURL := fmt.Sprintf(
		"%s/cosmos/base/tendermint/v1beta1/blocks/%d",
		endpoint.URL,
		block,
	)

	resp, err := getWithContext(&client, fullURL, timeout)
	if err != nil {
		attempt := 0
		for attempt < retryAttempts {
			resp, err = getWithContext(&client, fullURL, timeout)
			if err == nil {
				break
			}
			attempt++
			time.Sleep(retryDelay)
		}
		if err != nil {
			return BlockData{}, err
		}
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Failed to close response body: %v", err)
		}
	}()

	// check status code
	if resp.StatusCode != http.StatusOK {
		return BlockData{}, fmt.Errorf("endpoint returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return BlockData{}, err
	}

	var blockDataValue BlockData
	err = json.Unmarshal(body, &blockDataValue)
	if err != nil {
		return BlockData{}, err
	}
	return blockDataValue, nil
}

// GetCosmosLatestBlockHeight gets the latest block height from the REST API
//
// Parameters:
// - endpoint - the endpoint to get the latest block height from
// - retryAttempts - the number of retry attempts to perform
// - retryDelay - the delay between retry attempts
// - timeout - the timeout for the request
//
// Returns:
// - the latest block height
// - error if the request fails
func GetCosmosLatestBlockHeight(
	endpoint input.APIEndpoint,
	retryAttempts int,
	retryDelay time.Duration,
	timeout time.Duration) (int, error) {
	client := http.Client{
		Timeout: timeout,
	}
	fullURL := fmt.Sprintf("%s/cosmos/base/tendermint/v1beta1/blocks/latest", endpoint.URL)
	resp, err := getWithContext(&client, fullURL, timeout)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Fatalf("Failed to close response body: %v", err)
		}
	}()

	// check status code
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("endpoint returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	var blockDataValue BlockData
	err = json.Unmarshal(body, &blockDataValue)
	if err != nil {
		return 0, err
	}
	height, err := strconv.Atoi(blockDataValue.Block.Header.Height)
	if err != nil {
		return 0, err
	}
	return height, nil
}
