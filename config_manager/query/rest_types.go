package query

import (
	"net/http"
	"time"
)

// RestClient is an object for the REST API
type RestClient struct {
	BaseURLs      []string
	Client        *http.Client
	RetryAttempts int
	RetryDelay    time.Duration
	Timeout       time.Duration
}

// IbcChannelDataResponse is the response from the REST API for the IBC channel data
type IbcChannelDataResponse struct {
	Channel struct {
		State          string       `json:"state"`
		Ordering       string       `json:"ordering"`
		Counterparty   Counterparty `json:"counterparty"`
		ConnectionHops []string     `json:"connection_hops"`
		Version        string       `json:"version"`
	} `json:"channel"`
	Proof       any         `json:"proof"`
	ProofHeight ProofHeight `json:"proof_height"`
}

// Counterparty type is part of the IbcChannelDataResponse, it represents
//
//	the counterparty of the IBC channel
type Counterparty struct {
	PortID    string `json:"port_id"`
	ChannelID string `json:"channel_id"`
}

// ProofHeight type is part of the IbcChannelDataResponse, it represents
//
//	the proof height of the IBC channel
type ProofHeight struct {
	RevisionNumber string `json:"revision_number"`
	RevisionHeight string `json:"revision_height"`
}

// DenomTracesResponse type is the response from the REST API for the denom traces
type DenomTracesResponse struct {
	DenomTraces []DenomTrace `json:"denom_traces"`
	Pagination  Pagination   `json:"pagination"`
}

// DenomTrace type is part of the DenomTracesResponse, it represents
//
//	the trace of the denom
type DenomTrace struct {
	Path      string `json:"path"`
	BaseDenom string `json:"base_denom"`
}

// Pagination type is part of the DenomTracesResponse, it represents
//
//	the pagination for every respone that has a array of items
type Pagination struct {
	NextKey string `json:"next_key"`
	Total   string `json:"total"`
}

type NodeStatus struct {
	BaseUrl          string
	Provider         string
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
	DefaultNodeInfo    DefaultNodeInfo `json:"default_node_info"`
	ApplicationVersion struct {
		AppName          string      `json:"app_name"`
		Version          string      `json:"version"`
		GitCommit        string      `json:"git_commit"`
		CosmosSdkVersion string      `json:"cosmos_sdk_version"`
		BuildDeps        []BuildDeps `json:"build_deps"`
	} `json:"application_version"`
}

type BuildDeps struct {
	Path    string `json:"path"`
	Version string `json:"version"`
}

// DefaultNodeInfo type is part of the NodeInfoResponse, it represents
type DefaultNodeInfo struct {
	Network string        `json:"network"`
	Version string        `json:"version"`
	Other   NodeInfoOther `json:"other"`
}

// Partial Data from the Cosmos SDK request for the block data, used for validation
type BlockData struct {
	BlockID struct {
		Hash          string `json:"hash"`
		PartSetHeader struct {
			Total int    `json:"total"`
			Hash  string `json:"hash"`
		} `json:"part_set_header"`
	} `json:"block_id"`
	Block struct {
		Header struct {
			Version struct {
				Block string `json:"block"`
				App   string `json:"app"`
			} `json:"version"`
			ChainID     string    `json:"chain_id"`
			Height      string    `json:"height"`
			Time        time.Time `json:"time"`
			LastBlockID struct {
				Hash          string `json:"hash"`
				PartSetHeader struct {
					Total int    `json:"total"`
					Hash  string `json:"hash"`
				} `json:"part_set_header"`
			} `json:"last_block_id"`
			LastCommitHash     string `json:"last_commit_hash"`
			DataHash           string `json:"data_hash"`
			ValidatorsHash     string `json:"validators_hash"`
			NextValidatorsHash string `json:"next_validators_hash"`
			ConsensusHash      string `json:"consensus_hash"`
			AppHash            string `json:"app_hash"`
			LastResultsHash    string `json:"last_results_hash"`
			EvidenceHash       string `json:"evidence_hash"`
			ProposerAddress    string `json:"proposer_address"`
		} `json:"header"`
		Data struct {
			Txs []string `json:"txs"`
		} `json:"data"`
		LastCommit struct {
			Height  string `json:"height"`
			Round   int    `json:"round"`
			BlockID struct {
				Hash          string `json:"hash"`
				PartSetHeader struct {
					Total int    `json:"total"`
					Hash  string `json:"hash"`
				} `json:"part_set_header"`
			} `json:"block_id"`
			Signatures []struct {
				BlockIDFlag      string    `json:"block_id_flag"`
				ValidatorAddress string    `json:"validator_address"`
				Timestamp        time.Time `json:"timestamp"`
				Signature        string    `json:"signature"`
			} `json:"signatures"`
		} `json:"last_commit"`
	} `json:"block"`
}
