package validator

import (
	"time"

	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/input"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/query"
)

// A struct that allows us to track the progress if the url provided regardless if it is RPC or REST is legit
type RestApiValidity struct {
	// Same for RPC and REST
	Endpoint input.APIEndpoint
	// All start with 100 points, anything lower than 60 is invalid
	points int
	// If the points are less than 60, the endpoint is invalid
	valid bool
	// The chain ID of the application
	chainId *string
	// The version of the application
	version *string
	// The git commit of the application
	gitCommit *string
	// The Cosmos SDK version of the application
	cosmosSdkVersion *string
	// The application name
	appName *string
	// The application version
	appVersion *string
	// The binary name of the application
	binaryName *string
	// Block data
	blockData map[int]*query.BlockData
}

type blockDataTracker struct {
	blockHash          map[string]int
	height             map[int]int
	timestamp          map[time.Time]int
	chainId            map[string]int
	lastCommitHash     map[string]int
	dataHash           map[string]int
	validatorsHash     map[string]int
	nextValidatorsHash map[string]int
	consensusHash      map[string]int
	appHash            map[string]int
	lastResultsHash    map[string]int
	evidenceHash       map[string]int
	proposerAddress    map[string]int
}

// Consensue made by getting the most common value from the map
type majorityConsensus struct {
	blockHash          string
	height             int
	timestamp          time.Time
	chainId            string
	lastCommitHash     string
	dataHash           string
	validatorsHash     string
	nextValidatorsHash string
	consensusHash      string
	appHash            string
	lastResultsHash    string
	evidenceHash       string
	proposerAddress    string
}
