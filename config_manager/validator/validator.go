package validator

import (
	"log"
	"time"
)

/*
The validator package is used to make an in depth validation system that will check endpoints.

The old validation functions in the query package did validate partially but it was flawed.
In some cases the networks can contain some nodes that might have different versions, and usually if
this is the case the git commit is different.

There is no need to punish validators that provide this endpoint or to deny the app to use this endpoint just
because they are not following the rules because most of the times this can be fault of the OG developers of
the network.
*/

const mismatchPenalty = 10

// getMostCommonValue returns the key with the highest count in the map
func getMostCommonValue[T comparable](counts map[T]int) T {
	if len(counts) == 0 {
		return *new(T)
	}

	maxCount := 0
	var mostCommon T
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

/*
Validate the block data for the endpoints

Parameters:
- validities - the map of the endpoints and their validity (can be either REST or RPC)

Returns:
- none
*/
func validateBlockData[T any, PT interface {
	EndpointValidity
	*T
}](
	validities *map[string]T,
) {

	// Build tracker for consensus - need to initialize maps for each height
	tracker := make(map[int]blockDataTracker)
	for _, endpoint := range *validities {
		ep := PT(&endpoint)
		if !ep.IsValid() {
			continue
		}
		for height, blockData := range ep.GetBlockData() {
			if blockData == nil {
				continue
			}
			// Initialize maps if not present
			if _, exists := tracker[height]; !exists {
				tracker[height] = blockDataTracker{
					blockHash:          make(map[string]int),
					height:             make(map[int]int),
					timestamp:          make(map[time.Time]int),
					chainId:            make(map[string]int),
					lastCommitHash:     make(map[string]int),
					dataHash:           make(map[string]int),
					validatorsHash:     make(map[string]int),
					nextValidatorsHash: make(map[string]int),
					consensusHash:      make(map[string]int),
					appHash:            make(map[string]int),
					lastResultsHash:    make(map[string]int),
					evidenceHash:       make(map[string]int),
					proposerAddress:    make(map[string]int),
				}
			}

			t := tracker[height]
			t.height[height]++
			t.timestamp[blockData.Block.Header.Time]++
			t.chainId[blockData.Block.Header.ChainID]++
			t.lastCommitHash[blockData.Block.Header.LastCommitHash]++
			t.dataHash[blockData.Block.Header.DataHash]++
			t.validatorsHash[blockData.Block.Header.ValidatorsHash]++
			t.nextValidatorsHash[blockData.Block.Header.NextValidatorsHash]++
			t.consensusHash[blockData.Block.Header.ConsensusHash]++
			t.appHash[blockData.Block.Header.AppHash]++
			t.lastResultsHash[blockData.Block.Header.LastResultsHash]++
			t.evidenceHash[blockData.Block.Header.EvidenceHash]++
			t.proposerAddress[blockData.Block.Header.ProposerAddress]++
			tracker[height] = t
		}
	}

	// Build consensus map from tracker data
	consensusMap := make(map[int]majorityConsensus)
	for height, t := range tracker {
		consensusMap[height] = majorityConsensus{
			blockHash:          getMostCommonValue(t.blockHash),
			height:             getMostCommonValue(t.height),
			timestamp:          getMostCommonValue(t.timestamp),
			chainId:            getMostCommonValue(t.chainId),
			lastCommitHash:     getMostCommonValue(t.lastCommitHash),
			dataHash:           getMostCommonValue(t.dataHash),
			validatorsHash:     getMostCommonValue(t.validatorsHash),
			nextValidatorsHash: getMostCommonValue(t.nextValidatorsHash),
			consensusHash:      getMostCommonValue(t.consensusHash),
			appHash:            getMostCommonValue(t.appHash),
			lastResultsHash:    getMostCommonValue(t.lastResultsHash),
			evidenceHash:       getMostCommonValue(t.evidenceHash),
			proposerAddress:    getMostCommonValue(t.proposerAddress),
		}
	}

	// Validate each endpoint against the consensus
	for url, endpoint := range *validities {
		ep := PT(&endpoint)
		if !ep.IsValid() {
			continue
		}

		for height, blockData := range ep.GetBlockData() {
			if blockData == nil {
				ep.SetPoints(ep.GetPoints() - mismatchPenalty)
				log.Printf("Missing block data for %s at height %d, minus %d points",
					ep.GetURL(), height, mismatchPenalty)
				continue
			}

			consensus, exists := consensusMap[height]
			if !exists {
				continue
			}

			// Build checks slice inside the loop where height and blockData are in scope
			checks := []struct {
				name     string
				expected any
				actual   any
			}{
				{"chainId", consensus.chainId, blockData.Block.Header.ChainID},
				{"lastCommitHash", consensus.lastCommitHash, blockData.Block.Header.LastCommitHash},
				{"dataHash", consensus.dataHash, blockData.Block.Header.DataHash},
				{"validatorsHash", consensus.validatorsHash, blockData.Block.Header.ValidatorsHash},
				{"nextValidatorsHash", consensus.nextValidatorsHash, blockData.Block.Header.NextValidatorsHash},
				{"consensusHash", consensus.consensusHash, blockData.Block.Header.ConsensusHash},
				{"appHash", consensus.appHash, blockData.Block.Header.AppHash},
				{"lastResultsHash", consensus.lastResultsHash, blockData.Block.Header.LastResultsHash},
				{"evidenceHash", consensus.evidenceHash, blockData.Block.Header.EvidenceHash},
				{"proposerAddress", consensus.proposerAddress, blockData.Block.Header.ProposerAddress},
			}

			for _, check := range checks {
				if check.expected != check.actual {
					ep.SetPoints(ep.GetPoints() - mismatchPenalty)
					log.Printf("Mismatch found for %s at height %d (%s): expected %v, got %v, minus %d points",
						ep.GetURL(), height, check.name, check.expected, check.actual, mismatchPenalty)
				}
			}
		}

		// Mark as invalid if points dropped below threshold
		if ep.GetPoints() < 60 {
			ep.SetValid(false)
			log.Printf("Endpoint %s marked invalid due to low points: %d", ep.GetURL(), ep.GetPoints())
		}

		(*validities)[url] = endpoint
	}
}
