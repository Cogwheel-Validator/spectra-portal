package query

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"time"
)

// NewRestClient creates a new RestClient
//
// Params:
//	- baseURLs: the list of API URLs to use
//	- retryAttempts: the number of times to retry the request
//	- retryDelay: the delay between retries
//	- timeout: the timeout for the request
//
// Usage:
// - Used to make a client from which you can make requests to the REST APIs to gather the data
//
// Returns:
//	- *RestClient: the new RestClient
func NewRestClient(
	baseURLs []string, 
	retryAttempts int, 
	retryDelay time.Duration,
	timeout time.Duration,
	chainId string) *RestClient {
	healthyUrls := declareWorkingRestApis(baseURLs, retryAttempts, retryDelay, timeout, chainId)

	if len(healthyUrls) == 0 {
		log.Fatalf("No healthy URLs found for the chain %s", chainId)
	}

	return &RestClient{
		BaseURLs: healthyUrls,
		Client: &http.Client{
			Timeout: timeout,
		},
		RetryAttempts: retryAttempts,
		RetryDelay: retryDelay,
	}
}

// Gather the IBC channel data from the REST API
//
// Usage:
//	- To verify if all of the data match from the chain registry
//
// It currently assumes that all IBC channels are on the same port "transfer" which is
// 99% of the time true.
func (c *RestClient) GetIbcChannelData(channelId string) (*IbcChannelDataResponse, error) {
	fullURL := fmt.Sprintf("%s/ibc/core/channel/v1/channels/%s/ports/tranfer", generateRandomApiUrl(c.BaseURLs), channelId)
	resp, err := c.retryGetRequest(fullURL)
	if err != nil {
		return nil, err
	}
	
	// read the body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	// close the body
	defer func(body io.ReadCloser) {
		err := body.Close()
		if err != nil {
			log.Fatalf("Failed to close response body: %v", err)
		}
	}(resp.Body)

	// unmarshal the body
	var ibcChannelDataResponse IbcChannelDataResponse
	err = json.Unmarshal(body, &ibcChannelDataResponse)
	if err != nil {
		return nil, err
	}

	// return the response
	return &ibcChannelDataResponse, nil
}

func (c *RestClient) GetAllDenomTraces(denom string, nextKey string) (*DenomTracesResponse, error) {
	fullURL := fmt.Sprintf("%s/ibc/apps/transfer/v1/denom_traces?pagination.key=%s", generateRandomApiUrl(c.BaseURLs), nextKey)
	resp, err := c.retryGetRequest(fullURL)
	if err != nil {
		return nil, err
	}
	// read the body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// unmarshal the body
	var denomTracesResponse DenomTracesResponse
	err = json.Unmarshal(body, &denomTracesResponse)
	if err != nil {
		return nil, err
	}

	// close manually because the program needs to check if there are other pages
	err = resp.Body.Close()
	if err != nil {
		log.Fatalf("Failed to close response body: %v", err)
	}

	// get all the pages
	for denomTracesResponse.Pagination.NextKey != "" {
		var newDenomTracesResponse *DenomTracesResponse
		newDenomTracesResponse, err = c.GetAllDenomTraces(denom, denomTracesResponse.Pagination.NextKey)
		if err != nil {
			return nil, err
		}
		denomTracesResponse.DenomTraces = append(
			denomTracesResponse.DenomTraces, newDenomTracesResponse.DenomTraces...)
		denomTracesResponse.Pagination = newDenomTracesResponse.Pagination
	}

	// return the response
	return &denomTracesResponse, nil

}

// Get the denom hash from the REST API
//
// Params:
//	- stringifiedRoute: the stringified route to get the denom hash from, to get this you need to
//	  combine the path, example: "transfer/channel-1" or "transfer/channel-2/transfer/channel-80" for multihops 
//	  and then add original chain denom, example: uatone, uosmo, etc... The function should use url.PathEscape
//	  to make it safe for the URL
//
// Usage:
//	- To get the denom hash from the REST API
//
// Returns:
//	- string: the denom hash
func (c *RestClient) GetDenomHash(stringifiedRoute string) (string, error) {
	urlStringifiedRoute := url.PathEscape(stringifiedRoute)
	fullURL := fmt.Sprintf("%s/ibc/apps/transfer/v1/denom_hashes/%s", generateRandomApiUrl(c.BaseURLs), urlStringifiedRoute)
	resp, err := c.retryGetRequest(fullURL)
	if err != nil {
		return "", err
	}
	// read the body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	
	var response struct {
		Hash string `json:"hash"`
	}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return "", err
	}
	return response.Hash, nil
}

// retry the GET request
func (c *RestClient) retryGetRequest(fullURL string) (*http.Response, error) {
	retryAmt := 0
	var resp *http.Response
	var err error
	for retryAmt < c.RetryAttempts {
		resp, err = c.Client.Get(fullURL)
		if err != nil {
			retryAmt++
			// wait before retrying
			time.Sleep(c.RetryDelay)
		} else {
			break
		}
	}
	return resp, err
}

// generate a random API URL from the list of API URLs
func generateRandomApiUrl(baseURLs []string) string {
	return baseURLs[rand.Intn(len(baseURLs))]
}

func getRestStatus(
	baseUrl string, 
	retryAttempts int, 
	retryDelay time.Duration, 
	timeout time.Duration) (NodeStatus, error) {
	client := http.Client{
		Timeout: timeout,
	}
	fullURL := fmt.Sprintf("%s/cosmos/base/tendermint/v1beta1/node_info", baseUrl)
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
	network := response.Network
	version := response.Version
	tx_indexer := response.Other.TxIndexer
	application_version := response.ApplicationVersion
	app_name := application_version.AppName
	app_version := application_version.Version
	git_commit := application_version.GitCommit
	cosmos_sdk_version := application_version.CosmosSdkVersion
	tx_indexer_bool := tx_indexer == "on"
	nodeStatus := NodeStatus{
		BaseUrl: baseUrl,
		Network: network,
		Version: version,
		TxIndexer: tx_indexer_bool,
		AppName: app_name,
		AppVersion: app_version,
		GitCommit: git_commit,
		CosmosSdkVersion: cosmos_sdk_version,
	}

	// return the node status
	return nodeStatus, nil
}

func declareWorkingRestApis(
	baseURLs []string, 
	retryAttempts int, 
	retryDelay time.Duration, 
	timeout time.Duration, 
	chainId string) []string {
	
	// Step 1: Collect node status from all endpoints
	nodeStatuses := make([]NodeStatus, 0, len(baseURLs))
	for _, baseURL := range baseURLs {
		nodeStatus, err := getRestStatus(baseURL, retryAttempts, retryDelay, timeout)
		if err != nil {
			log.Printf("Failed to get REST status for %s: %v", baseURL, err)
			continue
		}
		if nodeStatus.Network != chainId {
			log.Printf("REST API for %s is not for the matching chain %s", baseURL, chainId)
			continue
		}
		if !nodeStatus.TxIndexer {
			log.Printf("REST API for %s does not have tx indexer enabled", baseURL)
			continue
		}
		nodeStatuses = append(nodeStatuses, nodeStatus)
	}
	
	if len(nodeStatuses) == 0 {
		log.Fatalf("No REST APIs are working and are for the matching chain %s", chainId)
	}

	// Step 2: Count occurrences of each attribute
	binaryNames := make(map[string]int)
	binaryCommits := make(map[string]int)
	versions := make(map[string]int)
	
	for _, nodeStatus := range nodeStatuses {
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
	healthyUrls := make([]string, 0, len(nodeStatuses))
	for _, nodeStatus := range nodeStatuses {
		// Note: In case of network upgrades, validators may have different versions.
		// This strict matching ensures consistency but may need to be relaxed
		// for chains with staggered upgrade patterns.
		if nodeStatus.AppName == expectedBinaryName && 
		   nodeStatus.Version == expectedVersion && 
		   nodeStatus.GitCommit == expectedCommit {
			healthyUrls = append(healthyUrls, nodeStatus.BaseUrl)
		} else {
			log.Printf("Filtering out %s due to version mismatch (app: %s, version: %s, commit: %s)",
				nodeStatus.BaseUrl, nodeStatus.AppName, nodeStatus.Version, nodeStatus.GitCommit)
		}
	}
	
	if len(healthyUrls) == 0 {
		log.Printf("Warning: No endpoints match consensus values. Expected - app: %s, version: %s, commit: %s",
			expectedBinaryName, expectedVersion, expectedCommit)
	}
	
	return healthyUrls
}
