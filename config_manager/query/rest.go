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

func getRestStatus(
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
	tx_index := response.DefaultNodeInfo.Other.TxIndex
	application_version := response.ApplicationVersion
	app_name := application_version.AppName
	app_version := application_version.Version
	git_commit := application_version.GitCommit
	cosmos_sdk_version := application_version.CosmosSdkVersion
	tx_indexer_bool := tx_index == "on"
	nodeStatus := NodeStatus{
		BaseUrl:          endpoint.URL,
		Provider:         endpoint.Provider,
		Network:          network,
		Version:          version,
		TxIndexer:        tx_indexer_bool,
		AppName:          app_name,
		AppVersion:       app_version,
		GitCommit:        git_commit,
		CosmosSdkVersion: cosmos_sdk_version,
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
// Returns a map of healthy endpoints
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
		nodeStatus, err := getRestStatus(endpoint, retryAttempts, retryDelay, timeout)
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
