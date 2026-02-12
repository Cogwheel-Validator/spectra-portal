package validator

import (
	"crypto/rand"
	"log"
	"math/big"
	"sync"
	"time"

	"github.com/Cogwheel-Validator/spectra-portal/config_manager/input"
	"github.com/Cogwheel-Validator/spectra-portal/config_manager/query"
)

func ValidateRestEndpoints(
	endpoints []input.APIEndpoint,
	retryAttempts int,
	retryDelay time.Duration,
	timeout time.Duration) map[query.URLProvider]bool {

	healthyEndpoints := make(map[query.URLProvider]bool)
	restValidities := initRestValidity(&endpoints)

	// Step 1: Collect node status from all endpoints
	chainIds, binaryNames, binaryCommits, cosmosSdkVersions, tendermintVersions, appVersions := fetchAllNetworkData(
		&restValidities, retryAttempts, retryDelay, timeout,
	)

	expectedBinaryName := getMostCommonValue(binaryNames)
	expectedTendermintVersion := getMostCommonValue(tendermintVersions)
	expectedCommit := getMostCommonValue(binaryCommits)
	expectedCosmosSdkVersion := getMostCommonValue(cosmosSdkVersions)
	expectedChainId := getMostCommonValue(chainIds)
	expectedAppVersion := getMostCommonValue(appVersions)

	if expectedBinaryName == "" {
		log.Fatalf("No binary name found")
	}
	if expectedTendermintVersion == "" {
		log.Fatalf("No Tendermint version found")
	}
	if expectedCommit == "" {
		log.Fatalf("No git commit found")
	}
	if expectedCosmosSdkVersion == "" {
		log.Fatalf("No Cosmos SDK version found")
	}
	if expectedAppVersion == "" {
		log.Fatalf("No app version found")
	}

	if expectedChainId == "" {
		log.Fatalf("No chain ID found")
	}

	// Step 2: First level validation, check all of the data the program has collected so far
	log.Printf("Validating network endpoints details for chain %s...", expectedChainId)
	validateNetworkDetails(
		expectedBinaryName,
		expectedTendermintVersion,
		expectedCommit,
		expectedCosmosSdkVersion,
		expectedChainId,
		expectedAppVersion,
		&restValidities,
	)

	// Step 3 fetch the block data from 7 randomly selected heights, usually the range should be
	// from the latest block height - 2000
	latestBlockHeights := fetchLatestBlockHeights(&restValidities, retryAttempts, retryDelay, timeout)
	realLatestHeight := getMaxHeight(latestBlockHeights)

	var generatedInts []int
	for i := 0; i < 7; i++ {
		generatedInt, err := rand.Int(rand.Reader, big.NewInt(2000))
		if err != nil {
			log.Fatalf("Failed to generate random integer: %v", err)
		}
		height := realLatestHeight - int(generatedInt.Int64())
		generatedInts = append(generatedInts, height)
	}

	// Step 4: Fetch the block data for the generated heights
	fetchBlockData(&restValidities, retryAttempts, retryDelay, timeout, generatedInts)

	// Step 5: Validate the block data for the endpoints
	validateBlockData(&restValidities)

	// Step 6: Return the healthy endpoints
	for _, endpoint := range restValidities {
		if endpoint.valid {
			healthyEndpoints[query.URLProvider{URL: endpoint.Endpoint.URL, Provider: endpoint.Endpoint.Provider}] = true
		}
	}

	return healthyEndpoints
}

/*
Fetch all the network data from the REST endpoints

Parameters:

- restValidities - the map of the REST endpoints and their validity
- retryAttempts - the number of retry attempts to perform
- retryDelay - the delay between retry attempts
- timeout - the timeout for the request

Returns:

- chainIds - the map of the chain IDs
- binaryNames - the map of the binary names
- binaryCommits - the map of the binary commits
- cosmosSdkVersions - the map of the Cosmos SDK versions
- tendermintVersions - the map of the Tendermint versions
*/
func fetchAllNetworkData(
	restValidities *map[string]RestApiValidity,
	retryAttempts int,
	retryDelay time.Duration,
	timeout time.Duration,
) (
	map[string]int, // chainIds
	map[string]int, // binaryNames
	map[string]int, // binaryCommits
	map[string]int, // cosmosSdkVersions
	map[string]int, // tendermintVersions
	map[string]int, // appVersions
) {
	chainIds := make(map[string]int)
	binaryNames := make(map[string]int)
	binaryCommits := make(map[string]int)
	cosmosSdkVersions := make(map[string]int)
	tendermintVersions := make(map[string]int)
	appVersions := make(map[string]int)

	for _, endpoint := range *restValidities {
		nodeStatus, err := query.GetRestStatus(endpoint.Endpoint, retryAttempts, retryDelay, timeout)
		if err != nil {
			log.Printf("Failed to get REST status for %s: %v", endpoint.Endpoint.URL, err)
			// Disqualify the endpont by marking it with validity.valid = false
			validity := (*restValidities)[endpoint.Endpoint.URL]
			validity.valid = false
			(*restValidities)[endpoint.Endpoint.URL] = validity
			continue
		}
		if !nodeStatus.TxIndexer {
			log.Printf("REST API for %s does not have tx indexer enabled", endpoint.Endpoint.URL)
			// Disqualify the endpont by marking it with validity.valid = false
			validity := (*restValidities)[endpoint.Endpoint.URL]
			validity.valid = false
			(*restValidities)[endpoint.Endpoint.URL] = validity
			continue
		}

		if nodeStatus.AppName != "" {
			binaryNames[nodeStatus.AppName]++
		}
		if nodeStatus.Version != "" {
			tendermintVersions[nodeStatus.Version]++
		}
		if nodeStatus.GitCommit != "" {
			binaryCommits[nodeStatus.GitCommit]++
		}
		if nodeStatus.CosmosSdkVersion != "" {
			cosmosSdkVersions[nodeStatus.CosmosSdkVersion]++
		}
		if nodeStatus.AppVersion != "" {
			appVersions[nodeStatus.AppVersion]++
		}
		chainIds[nodeStatus.Network]++

		validity := (*restValidities)[endpoint.Endpoint.URL]
		validity.chainId = &nodeStatus.Network
		validity.version = &nodeStatus.Version
		validity.gitCommit = &nodeStatus.GitCommit
		validity.cosmosSdkVersion = &nodeStatus.CosmosSdkVersion
		validity.appName = &nodeStatus.AppName
		validity.appVersion = &nodeStatus.AppVersion
		validity.binaryName = &nodeStatus.AppName

		(*restValidities)[endpoint.Endpoint.URL] = validity
	}

	return chainIds, binaryNames, binaryCommits, cosmosSdkVersions, tendermintVersions, appVersions
}

/*
Fetch the latest block heights from the REST endpoints

Parameters:
- restValidities - the map of the REST endpoints and their validity
- retryAttempts - the number of retry attempts to perform
- retryDelay - the delay between retry attempts
- timeout - the timeout for the request

Returns:
- latestBlockHeights - the map of the latest block heights
*/
func fetchLatestBlockHeights(
	restValidities *map[string]RestApiValidity,
	retryAttempts int,
	retryDelay time.Duration,
	timeout time.Duration,
) []int { // latest block heights

	latestBlockHeights := make([]int, 0)
	for _, endpoint := range *restValidities {
		// Skip if the endpoint is not valid
		if !endpoint.valid {
			continue
		}
		latestBlockHeight, err := query.GetCosmosLatestBlockHeight(endpoint.Endpoint, retryAttempts, retryDelay, timeout)
		if err != nil {
			log.Printf("Failed to get latest block height for %s: %v", endpoint.Endpoint.URL, err)
		}
		latestBlockHeights = append(latestBlockHeights, latestBlockHeight)
	}
	return latestBlockHeights
}

// Initialize a map containing the data which will be used to track the progress of the endpoints
func initRestValidity(endpoints *[]input.APIEndpoint) map[string]RestApiValidity {
	restValidities := make(map[string]RestApiValidity, len(*endpoints))
	for _, endpoint := range *endpoints {
		restValidities[endpoint.URL] = RestApiValidity{
			Endpoint: endpoint,
			points:   100,
			valid:    true,
		}
	}
	return restValidities
}

/*
Validate the network details of the endpoints

Parameters:
- expectedBinaryName - the expected binary name
- expectedTendermintVersion - the expected Tendermint version
- expectedCommit - the expected commit
- expectedCosmosSdkVersion - the expected Cosmos SDK version
- expectedChainId - the expected chain ID
- restValidities - the map of the REST endpoints and their validity

Returns:
- none
*/
func validateNetworkDetails(
	expectedBinaryName string,
	expectedTendermintVersion string,
	expectedCommit string,
	expectedCosmosSdkVersion string,
	expectedChainId string,
	expectedAppVersion string,
	restValidities *map[string]RestApiValidity,
) {

	for url, endpoint := range *restValidities {
		if !endpoint.valid {
			continue
		}

		checks := []struct {
			name     string
			actual   *string
			expected string
		}{
			{"chainId", endpoint.chainId, expectedChainId},
			{"version", endpoint.version, expectedTendermintVersion},
			{"gitCommit", endpoint.gitCommit, expectedCommit},
			{"cosmosSdkVersion", endpoint.cosmosSdkVersion, expectedCosmosSdkVersion},
			{"appVersion", endpoint.appVersion, expectedAppVersion},
			{"binaryName", endpoint.binaryName, expectedBinaryName},
		}

		for _, check := range checks {
			// If it is missing for some reason we should also count it as a penalty
			if check.actual == nil || *check.actual != check.expected {
				endpoint.points -= mismatchPenalty
				log.Printf("Mismatch found for %s: on check %s, expected %v, got %v, minus 10 points",
					url,
					check.name,
					check.expected,
					*check.actual,
				)
			}
		}

		// final check of the points, if it has less than 60 points, we should mark the endpoint as invalid
		if endpoint.points < 60 {
			endpoint.valid = false
		}

		// update the restValidities map
		(*restValidities)[url] = endpoint
	}
}

/*
Fetch the block data for the generated heights

Parameters:
- restValidities - the map of the REST endpoints and their validity
- retryAttempts - the number of retry attempts to perform
- retryDelay - the delay between retry attempts
- timeout - the timeout for the request
- heights - the heights to fetch the block data for

Returns:
- none
*/
func fetchBlockData(
	restValidities *map[string]RestApiValidity,
	retryAttempts int,
	retryDelay time.Duration,
	timeout time.Duration,
	heights []int,
) {

	for _, endpoint := range *restValidities {
		if !endpoint.valid {
			continue
		}
		// for inserting into the map
		mu := sync.Mutex{}
		// for fetching block data for each height
		wg := sync.WaitGroup{}
		wg.Add(len(heights))

		blockData := make(map[int]*query.BlockData)
		for _, height := range heights {
			go func(height int) {
				defer wg.Done()
				response, err := query.GetCosmosBlockHeights(
					endpoint.Endpoint,
					retryAttempts,
					retryDelay,
					timeout,
					height,
				)
				if err != nil {
					log.Printf("Failed to get block data for %s: %v", endpoint.Endpoint.URL, err)
					mu.Lock()
					blockData[height] = nil
					mu.Unlock()
					return
				}
				mu.Lock()
				blockData[height] = &response
				mu.Unlock()
			}(height)
		}
		wg.Wait()
		endpoint.blockData = blockData
		(*restValidities)[endpoint.Endpoint.URL] = endpoint
	}
}
