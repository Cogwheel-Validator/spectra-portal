package queryrest

import (
	"net/http"
	"time"
)

// RestClient is an object for the REST API
type RestClient struct {
	BaseURLs []string
	Client *http.Client
	RetryAttempts int
	RetryDelay time.Duration
	Timeout time.Duration
}

// IbcChannelDataResponse is the response from the REST API for the IBC channel data
type IbcChannelDataResponse struct {
	Channel struct {
		State        string `json:"state"`
		Ordering     string `json:"ordering"`
		Counterparty Counterparty `json:"counterparty"`
		ConnectionHops []string `json:"connection_hops"`
		Version        string   `json:"version"`
	} `json:"channel"`
	Proof       any `json:"proof"`
	ProofHeight ProofHeight `json:"proof_height"`
}

// Counterparty type is part of the IbcChannelDataResponse, it represents
//  the counterparty of the IBC channel
type Counterparty struct {
	PortID    string `json:"port_id"`
	ChannelID string `json:"channel_id"`
}

// ProofHeight type is part of the IbcChannelDataResponse, it represents
//  the proof height of the IBC channel
type ProofHeight struct {
	RevisionNumber string `json:"revision_number"`
	RevisionHeight string `json:"revision_height"`
}

// DenomTracesResponse type is the response from the REST API for the denom traces
type DenomTracesResponse struct {
	DenomTraces []DenomTrace `json:"denom_traces"`
	Pagination Pagination `json:"pagination"`
}

// DenomTrace type is part of the DenomTracesResponse, it represents
//  the trace of the denom
type DenomTrace struct {
	Path      string `json:"path"`
	BaseDenom string `json:"base_denom"`
}

// Pagination type is part of the DenomTracesResponse, it represents
//  the pagination for every respone that has a array of items
type Pagination struct {
	NextKey string `json:"next_key"`
	Total   string `json:"total"`
}

type NodeStatus struct {
	BaseUrl          string
	Network          string
	Version          string
	TxIndexer        bool
	AppName          string
	AppVersion       string
	GitCommit        string
	CosmosSdkVersion string
}

// nodeInfoResponse represents the structure of the REST API response
type NodeInfoResponse struct {
	Network string `json:"network"`
	Version string `json:"version"`
	Other   struct {
		TxIndexer string `json:"tx_indexer"`
	} `json:"other"`
	ApplicationVersion struct {
		AppName          string `json:"app_name"`
		Version          string `json:"version"`
		GitCommit        string `json:"git_commit"`
		CosmosSdkVersion string `json:"cosmos_sdk_version"`
	} `json:"application_version"`
}