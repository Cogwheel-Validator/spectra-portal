package validator

import (
	"crypto/rand"
	"log"
	"math/big"
	"strconv"
	"sync"
	"time"

	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/input"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/query"
)

func ValidateRpcEndpoints(
	endpoints []input.APIEndpoint,
	retryAttempts int,
	retryDelay time.Duration,
	timeout time.Duration,
) map[query.URLProvider]bool {
	healthyEndpoints := make(map[query.URLProvider]bool)
	rpcValidities := initRpcValidity(&endpoints)

	// Step 1: Collect validation data from all endpoints
	heights, versions, abciAppNames, chainIds := collectRpcValidationBasicData(&rpcValidities, retryAttempts, retryDelay, timeout)

	// Step 2: Determine consensus values
	maxHeight := getMaxHeight(heights)
	expectedVersion := getMostCommonValue(versions)
	expectedAbciApp := getMostCommonValue(abciAppNames)
	expectedChainId := getMostCommonValue(chainIds)

	// Step 3: Filter endpoints by consensus
	filterRpcEndpointsByBasicData(&rpcValidities, maxHeight, expectedVersion, expectedAbciApp, expectedChainId)

	// Step 4: Declare 7 blocks that will be used to validate the endpoints
	blocks := make([]int, 7)
	for i := 0; i < 7; i++ {
		randomInt, err := rand.Int(rand.Reader, big.NewInt(2000))
		if err != nil {
			log.Fatalf("Failed to generate random integer: %v", err)
		}
		blocks[i] = maxHeight - int(randomInt.Int64())
	}

	// Step 5: Fetch the block data from the endpoints
	fetchRpcBlockData(&rpcValidities, retryAttempts, retryDelay, timeout, blocks)

	// Step 6: Validate the block data for the endpoints
	validateBlockData(&rpcValidities)

	// Step 7: Return the healthy endpoints
	for _, endpoint := range rpcValidities {
		if endpoint.valid {
			healthyEndpoints[query.URLProvider{URL: endpoint.Endpoint.URL, Provider: endpoint.Endpoint.Provider}] = true
		}
	}

	return healthyEndpoints
}

func initRpcValidity(endpoints *[]input.APIEndpoint) map[string]RpcValidity {
	rpcValidities := make(map[string]RpcValidity, len(*endpoints))
	for _, endpoint := range *endpoints {
		rpcValidities[endpoint.URL] = RpcValidity{
			Endpoint: endpoint,
			points:   100,
			valid:    true,
		}
	}
	return rpcValidities
}

func collectRpcValidationBasicData(
	rpcValidities *map[string]RpcValidity,
	retryAttempts int,
	retryDelay time.Duration,
	timeout time.Duration,
) (
	[]int, // heights
	map[string]int, // versions
	map[string]int, // abciAppNames
	map[string]int, // chainIds
) {
	heights := make([]int, 0, len(*rpcValidities))
	versions := make(map[string]int)
	abciAppNames := make(map[string]int)
	chainIds := make(map[string]int)
	for _, endpoint := range *rpcValidities {
		c := query.NewRpcClient([]string{endpoint.Endpoint.URL}, retryAttempts, retryDelay, timeout)
		status, err := c.QueryStatus(endpoint.Endpoint.URL)
		if err != nil {
			log.Printf("failed to query status for %s: %v", endpoint.Endpoint.URL, err)
			endpoint.valid = false
			(*rpcValidities)[endpoint.Endpoint.URL] = endpoint
			continue
		}
		abciInfo, err := c.QueryAbciInfo(endpoint.Endpoint.URL)
		if err != nil {
			log.Printf("failed to query abci info for %s: %v", endpoint.Endpoint.URL, err)
			endpoint.valid = false
			(*rpcValidities)[endpoint.Endpoint.URL] = endpoint
			continue
		}
		height, err := strconv.Atoi(status.Result.StatusSyncInfo.LatestBlockHeight)
		if err != nil {
			log.Printf("failed to convert latest block height for %s: %v", endpoint.Endpoint.URL, err)
			endpoint.valid = false
			(*rpcValidities)[endpoint.Endpoint.URL] = endpoint
			continue
		}
		endpoint.height = &height
		endpoint.version = &status.Result.NodeInfo.Version
		endpoint.abciAppName = &abciInfo.Result.Response.Data
		endpoint.chainId = &status.Result.NodeInfo.Network
		(*rpcValidities)[endpoint.Endpoint.URL] = endpoint

		// record the consensus values
		heights = append(heights, height)
		versions[status.Result.NodeInfo.Version]++
		abciAppNames[abciInfo.Result.Response.Data]++
		chainIds[status.Result.NodeInfo.Network]++
	}
	return heights, versions, abciAppNames, chainIds
}

/*
Filter the endpoints by the basic data

Parameters:
- rpcValidities - the map of the RPC endpoints and their validity
- maxHeight - the maximum height of the blocks
- expectedVersion - the expected version of the blocks
- expectedAbciAppName - the expected ABCI app name of the blocks
- expectedChainId - the expected chain ID of the blocks
*/
func filterRpcEndpointsByBasicData(
	rpcValidities *map[string]RpcValidity,
	maxHeight int,
	expectedVersion string,
	expectedAbciAppName string,
	expectedChainId string,
) {

	for _, endpoint := range *rpcValidities {
		if !endpoint.valid {
			continue
		}
		checks := []struct {
			name     string
			actual   any
			expected any
		}{
			{"version", *endpoint.version, expectedVersion},
			{"abciAppName", *endpoint.abciAppName, expectedAbciAppName},
			{"chainId", *endpoint.chainId, expectedChainId},
		}
		for _, check := range checks {
			if check.actual != check.expected {
				log.Printf("endpoint %s has different %s: %v (expected: %v) minus 10 points",
					endpoint.Endpoint.URL, check.name, check.actual, check.expected)
				endpoint.points -= mismatchPenalty
				(*rpcValidities)[endpoint.Endpoint.URL] = endpoint

			}
		}

		if *endpoint.height < maxHeight-500 {
			endpoint.points -= mismatchPenalty
			log.Printf("endpoint %s is behind by more than 500 blocks minuse 10 points", endpoint.Endpoint.URL)
			(*rpcValidities)[endpoint.Endpoint.URL] = endpoint
			continue
		}

		// final check of the points, if it has less than 60 points, we should mark the endpoint as invalid
		if endpoint.points < 60 {
			endpoint.valid = false
			log.Printf("Endpoint %s marked invalid due to low points: %d",
				endpoint.Endpoint.URL, endpoint.points)
		}

		// update the rpcValidities map
		(*rpcValidities)[endpoint.Endpoint.URL] = endpoint
	}
}

/*
Fetch the block data from the endpoints

Parameters:
- rpcValidities - the map of the RPC endpoints and their validity
- retryAttempts - the number of retry attempts to perform
- retryDelay - the delay between retry attempts
- timeout - the timeout for the request
- heights - the heights to fetch the block data for
*/
func fetchRpcBlockData(
	rpcValidities *map[string]RpcValidity,
	retryAttempts int,
	retryDelay time.Duration,
	timeout time.Duration,
	heights []int,
) {
	for _, endpoint := range *rpcValidities {
		if !endpoint.valid {
			continue
		}

		wg := sync.WaitGroup{}
		wg.Add(len(heights))
		mu := sync.Mutex{}
		blockData := make(map[int]*query.BlockData)
		for _, height := range heights {
			go func(height int) {
				c := query.NewRpcClient([]string{endpoint.Endpoint.URL}, retryAttempts, retryDelay, timeout)
				block, err := c.QueryBlock(endpoint.Endpoint.URL, height)
				if err != nil {
					log.Printf("failed to get block data for %s at height %d: %v", endpoint.Endpoint.URL, height, err)
					mu.Lock()
					blockData[height] = nil
					mu.Unlock()
					wg.Done()
				}
				data := block.Result
				mu.Lock()
				blockData[height] = &data
				mu.Unlock()
				wg.Done()
			}(height)
		}
		wg.Wait()
		endpoint.blockData = blockData
		(*rpcValidities)[endpoint.Endpoint.URL] = endpoint
	}
}
