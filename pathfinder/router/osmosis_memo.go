package router

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// WasmMemoBuilder builds IBC memo structures for Osmosis swap operations
// using the ibc-hooks entry point contract.
type WasmMemoBuilder struct {
	contractAddress string
}

// NewWasmMemoBuilder creates a new memo builder for the given entry point contract
func NewWasmMemoBuilder(contractAddress string) *WasmMemoBuilder {
	return &WasmMemoBuilder{
		contractAddress: contractAddress,
	}
}

// SwapOperation represents a single pool hop in a swap route
type SwapOperation struct {
	Pool     string `json:"pool"`
	DenomIn  string `json:"denom_in"`
	DenomOut string `json:"denom_out"`
}

// SwapMemoParams contains all parameters needed to build a swap memo
type SwapMemoParams struct {
	// Input token denom on the broker chain
	TokenInDenom string
	// Output token denom on the broker chain
	TokenOutDenom string
	// Minimum output amount (for slippage protection)
	MinOutputAmount string
	// Route data from SQS containing pool information
	RouteData *OsmosisRouteData
	// Timeout timestamp in nanoseconds
	TimeoutTimestamp int64
	// Recovery address on the broker chain (where funds go if something fails)
	RecoverAddress string
	// Post-swap action
	PostSwapAction PostSwapAction
}

// PostSwapAction defines what to do after the swap
type PostSwapAction struct {
	// For swap-only: transfer to this address on the broker chain
	TransferTo string
	// For swap+forward: IBC transfer info
	IBCTransfer *IBCTransferInfo
}

// IBCTransferInfo contains IBC transfer details for post-swap forwarding
type IBCTransferInfo struct {
	SourceChannel string
	Receiver      string
	Memo          string
}

// WasmMemo is the top-level structure for ibc-hooks wasm memo
type WasmMemo struct {
	Wasm *WasmMsg `json:"wasm,omitempty"`
}

// WasmMsg contains the contract call information
type WasmMsg struct {
	Contract string         `json:"contract"`
	Msg      *SwapAndAction `json:"msg"`
}

// SwapAndAction is the entry point contract message
type SwapAndAction struct {
	SwapAndAction *SwapAndActionInner `json:"swap_and_action"`
}

// SwapAndActionInner contains the swap details
type SwapAndActionInner struct {
	UserSwap         *UserSwap          `json:"user_swap"`
	MinAsset         *MinAsset          `json:"min_asset"`
	TimeoutTimestamp int64              `json:"timeout_timestamp"`
	PostSwapAction   *PostSwapActionMsg `json:"post_swap_action"`
	Affiliates       []interface{}      `json:"affiliates"`
}

// UserSwap contains the swap route
type UserSwap struct {
	SwapExactAssetIn *SwapExactAssetIn `json:"swap_exact_asset_in"`
}

// SwapExactAssetIn contains the swap venue and operations
type SwapExactAssetIn struct {
	SwapVenueName string          `json:"swap_venue_name"`
	Operations    []SwapOperation `json:"operations"`
}

// MinAsset specifies the minimum output
type MinAsset struct {
	Native *NativeAsset `json:"native"`
}

// NativeAsset is a native token with denom and amount
type NativeAsset struct {
	Denom  string `json:"denom"`
	Amount string `json:"amount"`
}

// PostSwapActionMsg is the action to take after swapping
type PostSwapActionMsg struct {
	Transfer    *TransferAction    `json:"transfer,omitempty"`
	IBCTransfer *IBCTransferAction `json:"ibc_transfer,omitempty"`
}

// TransferAction sends tokens to an address on the same chain
type TransferAction struct {
	ToAddress string `json:"to_address"`
}

// IBCTransferAction sends tokens via IBC to another chain
type IBCTransferAction struct {
	IBCInfo *IBCInfo `json:"ibc_info"`
}

// IBCInfo contains IBC transfer details
type IBCInfo struct {
	SourceChannel  string `json:"source_channel"`
	Receiver       string `json:"receiver"`
	Memo           string `json:"memo"`
	RecoverAddress string `json:"recover_address"`
}

// PFMMemo is a simple PFM forward memo (no swap)
type PFMMemo struct {
	Forward *PFMForward `json:"forward"`
}

// PFMForward contains PFM forwarding details
type PFMForward struct {
	Channel  string `json:"channel"`
	Port     string `json:"port"`
	Receiver string `json:"receiver"`
	Retries  int    `json:"retries,omitempty"`
	Timeout  int64  `json:"timeout,omitempty"`
}

// BuildSwapMemo creates the wasm memo JSON for a swap operation
func (b *WasmMemoBuilder) BuildSwapMemo(params SwapMemoParams) (string, error) {
	if b.contractAddress == "" {
		return "", fmt.Errorf("ibc-hooks contract address not configured")
	}

	// Build swap operations from route data
	operations, err := b.buildSwapOperations(params.RouteData, params.TokenInDenom)
	if err != nil {
		return "", fmt.Errorf("failed to build swap operations: %w", err)
	}

	// Build post-swap action
	var postSwapAction *PostSwapActionMsg
	if params.PostSwapAction.TransferTo != "" {
		// Swap-only: transfer to address on broker chain
		postSwapAction = &PostSwapActionMsg{
			Transfer: &TransferAction{
				ToAddress: params.PostSwapAction.TransferTo,
			},
		}
	} else if params.PostSwapAction.IBCTransfer != nil {
		// Swap + forward: IBC transfer to destination chain
		postSwapAction = &PostSwapActionMsg{
			IBCTransfer: &IBCTransferAction{
				IBCInfo: &IBCInfo{
					SourceChannel:  params.PostSwapAction.IBCTransfer.SourceChannel,
					Receiver:       params.PostSwapAction.IBCTransfer.Receiver,
					Memo:           params.PostSwapAction.IBCTransfer.Memo,
					RecoverAddress: params.RecoverAddress,
				},
			},
		}
	} else {
		return "", fmt.Errorf("post-swap action not specified")
	}

	// Build the full memo structure
	memo := &WasmMemo{
		Wasm: &WasmMsg{
			Contract: b.contractAddress,
			Msg: &SwapAndAction{
				SwapAndAction: &SwapAndActionInner{
					UserSwap: &UserSwap{
						SwapExactAssetIn: &SwapExactAssetIn{
							SwapVenueName: "osmosis-poolmanager",
							Operations:    operations,
						},
					},
					MinAsset: &MinAsset{
						Native: &NativeAsset{
							Denom:  params.TokenOutDenom,
							Amount: params.MinOutputAmount,
						},
					},
					TimeoutTimestamp: params.TimeoutTimestamp,
					PostSwapAction:   postSwapAction,
					Affiliates:       []interface{}{},
				},
			},
		},
	}

	// Marshal to JSON
	memoBytes, err := json.Marshal(memo)
	if err != nil {
		return "", fmt.Errorf("failed to marshal memo: %w", err)
	}

	return string(memoBytes), nil
}

// buildSwapOperations converts SQS route data to swap operations
func (b *WasmMemoBuilder) buildSwapOperations(routeData *OsmosisRouteData, tokenInDenom string) ([]SwapOperation, error) {
	if routeData == nil || len(routeData.Routes) == 0 {
		return nil, fmt.Errorf("no route data available")
	}

	// Use the first route (best route from SQS)
	route := routeData.Routes[0]
	if len(route.Pools) == 0 {
		return nil, fmt.Errorf("no pools in route")
	}

	operations := make([]SwapOperation, len(route.Pools))
	currentDenomIn := tokenInDenom

	for i, pool := range route.Pools {
		operations[i] = SwapOperation{
			Pool:     strconv.Itoa(int(pool.ID)),
			DenomIn:  currentDenomIn,
			DenomOut: pool.TokenOutDenom,
		}
		// The output of this pool is the input of the next
		currentDenomIn = pool.TokenOutDenom
	}

	return operations, nil
}

// BuildPFMMemo creates a simple PFM forward memo (no swap)
func BuildPFMMemo(channel, port, receiver string, timeoutNanos int64) (string, error) {
	memo := &PFMMemo{
		Forward: &PFMForward{
			Channel:  channel,
			Port:     port,
			Receiver: receiver,
			Retries:  2,
			Timeout:  timeoutNanos,
		},
	}

	memoBytes, err := json.Marshal(memo)
	if err != nil {
		return "", fmt.Errorf("failed to marshal PFM memo: %w", err)
	}

	return string(memoBytes), nil
}

// DefaultTimeoutTimestamp returns a default timeout (10 minutes from now) in nanoseconds
func DefaultTimeoutTimestamp() int64 {
	return time.Now().Add(10 * time.Minute).UnixNano()
}

// CalculateMinOutput calculates minimum output with slippage tolerance
// slippageBps is basis points (e.g., 100 = 1%)
func CalculateMinOutput(expectedOutput string, slippageBps uint32) (string, error) {
	// Parse the expected output
	expected, err := strconv.ParseInt(expectedOutput, 10, 64)
	if err != nil {
		return "", fmt.Errorf("failed to parse expected output: %w", err)
	}

	// Calculate minimum with slippage
	// minOutput = expected * (10000 - slippageBps) / 10000
	minOutput := expected * int64(10000-slippageBps) / 10000

	return strconv.FormatInt(minOutput, 10), nil
}
