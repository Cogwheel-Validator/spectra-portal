package validator

import (
	"time"

	"github.com/Cogwheel-Validator/spectra-portal/config_manager/input"
	"github.com/Cogwheel-Validator/spectra-portal/config_manager/query"
)

// EndpointValidity is the interface that both RestApiValidity and RpcValidity must implement
type EndpointValidity interface {
	GetEndpoint() input.APIEndpoint
	GetPoints() int
	SetPoints(int)
	IsValid() bool
	SetValid(bool)
	GetBlockData() map[int]*query.BlockData
	GetURL() string
}

// RestApiValidity allows us to track the progress if the url provided regardless if it is RPC or REST is legit
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

// GetEndpoint implements the EndpointValidity interface for RestApiValidity
func (r RestApiValidity) GetEndpoint() input.APIEndpoint {
	return r.Endpoint
}

// GetPoints returns the current validity points for the endpoint.
func (r RestApiValidity) GetPoints() int {
	return r.points
}

// SetPoints sets the validity points for the endpoint.
func (r *RestApiValidity) SetPoints(p int) {
	r.points = p
}

// IsValid returns whether the endpoint is currently considered valid.
func (r RestApiValidity) IsValid() bool {
	return r.valid
}

// SetValid sets whether the endpoint is considered valid.
func (r *RestApiValidity) SetValid(v bool) {
	r.valid = v
}

// GetBlockData returns the collected block data for the endpoint.
func (r RestApiValidity) GetBlockData() map[int]*query.BlockData {
	return r.blockData
}

// GetURL returns the endpoint's URL.
func (r RestApiValidity) GetURL() string {
	return r.Endpoint.URL
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

// majorityConsensus is the consensus made by getting the most common value from the map
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

// RpcValidity allows us to track the progress of validating an RPC endpoint.
type RpcValidity struct {
	Endpoint    input.APIEndpoint
	points      int
	valid       bool
	version     *string
	abciAppName *string
	height      *int
	// marked as network in the status
	chainId   *string
	blockData map[int]*query.BlockData
}

// GetEndpoint implements the EndpointValidity interface for RpcValidity
func (r RpcValidity) GetEndpoint() input.APIEndpoint {
	return r.Endpoint
}

// GetPoints returns the current validity points for the endpoint.
func (r RpcValidity) GetPoints() int {
	return r.points
}

// SetPoints sets the validity points for the endpoint.
func (r *RpcValidity) SetPoints(p int) {
	r.points = p
}

// IsValid returns whether the endpoint is currently considered valid.
func (r RpcValidity) IsValid() bool {
	return r.valid
}

// SetValid sets whether the endpoint is considered valid.
func (r *RpcValidity) SetValid(v bool) {
	r.valid = v
}

// GetBlockData returns the collected block data for the endpoint.
func (r RpcValidity) GetBlockData() map[int]*query.BlockData {
	return r.blockData
}

// GetURL returns the endpoint's URL.
func (r RpcValidity) GetURL() string {
	return r.Endpoint.URL
}

// RpcValidationBasicData aggregates the abci_info and status data collected for an RPC endpoint.
type RpcValidationBasicData struct {
	AbciInfo query.AbciInfoResponseResultResponse
	Status   query.StatusResault
}
