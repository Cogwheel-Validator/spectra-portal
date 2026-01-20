package query

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/input"
)

func GetRestStatus(
	endpoint input.APIEndpoint,
	retryAttempts int,
	retryDelay time.Duration,
	timeout time.Duration) (NodeStatus, error) {
	client := http.Client{
		Timeout: timeout,
	}
	fullURL := fmt.Sprintf("%s/cosmos/base/tendermint/v1beta1/node_info", endpoint.URL)
	resp, err := client.Get(fullURL)
	if err != nil {
		//retry
		attempt := 0
		for attempt < retryAttempts {
			resp, err = client.Get(fullURL)
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

/*
Get the Cosmos SDK version from the REST endpoint

Parameters:

- healthyRestEndpoint - the healthy REST endpoint to get the Cosmos SDK version from

Returns:
- the Cosmos SDK version
- error if the request fails

Only used within the client config generation for now
*/
func GetCosmosSdkVersion(healthyRestEndpoint string) (string, error) {
	client := http.Client{
		Timeout: 10 * time.Second,
	}
	resp, err := client.Get(healthyRestEndpoint + "/cosmos/base/tendermint/v1beta1/node_info")
	if err != nil {
		return "", err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Fatalf("Failed to close response body: %v", err)
		}
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var response NodeInfoResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return "", err
	}
	return response.ApplicationVersion.CosmosSdkVersion, nil
}

/*
Get the block data from the REST API for a given block

Parameters:
- endpoint - the endpoint to get the block data from
- block - the integer of the blocks to get the data from

Returns:
- the block data
- map of the block data with the block number as the key
- error if the request fails
*/
func GetCosmosBlockHeights(endpoint input.APIEndpoint, block int) (BlockData, error) {
	client := http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Do not allow ANY redirects! Will work with this more...
			return fmt.Errorf("redirect not allowed (count=%d) to %s", len(via), req.URL.String())
		},
	}

	resp, err := client.Get(
		fmt.Sprintf(
			"%s/cosmos/base/tendermint/v1beta1/blocks/%d",
			endpoint.URL,
			block,
		),
	)
	if err != nil {
		log.Printf("Failed to get block %d: %v", block, err)
		return BlockData{}, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return BlockData{}, err
	}

	var blockDataValue BlockData
	err = json.Unmarshal(body, &blockDataValue)
	if err != nil {
		log.Printf("Failed to unmarshal block %d: %v", block, err)
		return BlockData{}, err
	}
	return blockDataValue, nil
}
