package query

import (
	"net/http"
	"time"
)

// RpcClient is an object for the RPC API
type RpcClient struct {
	BaseURLs      []string
	Client        *http.Client
	RetryAttempts int
	RetryDelay    time.Duration
	Timeout       time.Duration
}

// RpcStatusResponse is the response from the RPC status endpoint.
type RpcStatusResponse struct {
	JsonRPC string        `json:"json_rpc"`
	Id      int           `json:"id"`
	Result  StatusResault `json:"result"`
}

// StatusResault holds the node info and sync info from the RPC status endpoint.
type StatusResault struct {
	NodeInfo       StatusNodeInfo `json:"node_info"`
	StatusSyncInfo StatusSyncInfo `json:"sync_info"`
}

// StatusNodeInfo holds the node identity data from the RPC status endpoint.
type StatusNodeInfo struct {
	Network string        `json:"network"`
	Version string        `json:"version"`
	Other   NodeInfoOther `json:"other"`
}

// NodeInfoOther holds additional node info fields, such as the tx indexer setting.
type NodeInfoOther struct {
	TxIndex string `json:"tx_index"`
}

// StatusSyncInfo holds the sync status from the RPC status endpoint.
type StatusSyncInfo struct {
	LatestBlockHeight string    `json:"latest_block_height"`
	LatestBlockTime   time.Time `json:"latest_block_time"`
	CatchingUp        bool      `json:"catching_up"`
}

// AbciInfoResponse is the response from the RPC abci_info endpoint.
type AbciInfoResponse struct {
	Jsonrpc string                 `json:"jsonrpc"`
	ID      int                    `json:"id"`
	Result  AbciInfoResponseResult `json:"result"`
}

// AbciInfoResponseResult holds the response payload of AbciInfoResponse.
type AbciInfoResponseResult struct {
	Response AbciInfoResponseResultResponse `json:"response"`
}

// AbciInfoResponseResultResponse holds the application info returned by abci_info.
type AbciInfoResponseResultResponse struct {
	Data             string `json:"data"`
	Version          string `json:"version"`
	LastBlockHeight  string `json:"last_block_height"`
	LastBlockAppHash string `json:"last_block_app_hash"`
}

// CollectedValidationData aggregates the abci_info and status data collected for an endpoint.
type CollectedValidationData struct {
	AbciInfo AbciInfoResponseResultResponse
	Status   StatusResault
}

// URLProvider identifies an endpoint URL together with its provider name.
type URLProvider struct {
	URL      string
	Provider string
}

// RpcBlockResponse is the response from the RPC block endpoint.
type RpcBlockResponse struct {
	Jsonrpc string    `json:"jsonrpc"`
	Id      int       `json:"id"`
	Result  BlockData `json:"result"`
}
