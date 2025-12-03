package query

import (
	"net/http"
	"time"
)

// rpcClient is an object for the RPC API
type RpcClient struct {
	BaseURLs      []string
	Client        *http.Client
	RetryAttempts int
	RetryDelay    time.Duration
	Timeout       time.Duration
}

type RpcStatusResponse struct {
	JsonRPC string        `json:"json_rpc"`
	Id      int           `json:"id"`
	Result  StatusResault `json:"result"`
}

type StatusResault struct {
	NodeInfo       StatusNodeInfo `json:"node_info"`
	StatusSyncInfo StatusSyncInfo `json:"sync_info"`
}
type StatusNodeInfo struct {
	Network string        `json:"network"`
	Version string        `json:"version"`
	Other   NodeInfoOther `json:"other"`
}

type NodeInfoOther struct {
	TxIndex string `json:"tx_index"`
}

type StatusSyncInfo struct {
	LatestBlockHeight string    `json:"latest_block_height"`
	LatestBlockTime   time.Time `json:"latest_block_time"`
	CatchingUp        bool      `json:"catching_up"`
}

type AbciInfoResponse struct {
	Jsonrpc string                 `json:"jsonrpc"`
	ID      int                    `json:"id"`
	Result  AbciInfoResponseResult `json:"result"`
}

type AbciInfoResponseResult struct {
	Response AbciInfoResponseResultResponse `json:"response"`
}

type AbciInfoResponseResultResponse struct {
	Data             string `json:"data"`
	Version          string `json:"version"`
	LastBlockHeight  string `json:"last_block_height"`
	LastBlockAppHash string `json:"last_block_app_hash"`
}

type CollectedValidationData struct {
	AbciInfo AbciInfoResponseResultResponse
	Status   StatusResault
}

type URLProvider struct {
	URL      string
	Provider string
}
