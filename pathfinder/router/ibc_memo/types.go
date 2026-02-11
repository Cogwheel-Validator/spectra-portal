package ibcmemo

import "encoding/json"

// PostSwapActionEnum defines the type of action after a swap
type PostSwapActionEnum string

const (
	PostSwapActionEnumTransfer    PostSwapActionEnum = "transfer"
	PostSwapActionEnumIBCTransfer PostSwapActionEnum = "ibc_transfer"
)

// ForwardMemo is the top-level structure for PFM forwarding (case 1, 5.2, 5.4 from doc.go)
// This wraps either a simple forward or a forward-with-wasm-next
type ForwardMemo struct {
	Forward *PFMForward `json:"forward"`
}

// WasmMemo is the top-level structure for ibc-hooks wasm memo (case 2, 5.1, 5.3 from doc.go)
type WasmMemo struct {
	Wasm *WasmData `json:"wasm"`
}

// PFMForward contains PFM forwarding details
// Can contain a "next" field for chaining to wasm or another forward
type PFMForward struct {
	Channel  string `json:"channel"`
	Port     string `json:"port"`
	Receiver string `json:"receiver"`
	Retries  int    `json:"retries,omitempty"`
	Timeout  int64  `json:"timeout,omitempty"`
	// Next can contain either a Wasm action or another PFMForward for multi-hop
	Next *PFMNext `json:"next,omitempty"`
}

// PFMNext represents the "next" field in a PFM forward
// It can contain either a wasm action or another forward (union type)
type PFMNext struct {
	Wasm    *WasmData   `json:"wasm,omitempty"`
	Forward *PFMForward `json:"forward,omitempty"`
}

// WasmData contains the contract call information
type WasmData struct {
	Contract string   `json:"contract"`
	Msg      *WasmMsg `json:"msg"`
}

// WasmMsg contains the swap_and_action message
type WasmMsg struct {
	SwapAndAction *SwapAndAction `json:"swap_and_action"`
}

// SwapAndAction is the entry point contract message structure
type SwapAndAction struct {
	UserSwap         *UserSwap       `json:"user_swap"`
	MinAsset         *MinAsset       `json:"min_asset"`
	TimeoutTimestamp int64           `json:"timeout_timestamp"`
	PostSwapAction   *PostSwapAction `json:"post_swap_action"`
	Affiliates       []interface{}   `json:"affiliates"`
}

// UserSwap contains the swap route information
type UserSwap struct {
	SwapExactAssetIn *SwapExactAssetIn `json:"swap_exact_asset_in"`
}

// SwapExactAssetIn contains the swap venue and operations
type SwapExactAssetIn struct {
	SwapVenueName string          `json:"swap_venue_name"`
	Operations    []SwapOperation `json:"operations"`
}

// SwapOperation represents a single pool hop in a swap route
type SwapOperation struct {
	Pool     string `json:"pool"`
	DenomIn  string `json:"denom_in"`
	DenomOut string `json:"denom_out"`
	// Interface is used by some DEXs (like Injective), optional
	Interface *string `json:"interface,omitempty"`
}

// MinAsset specifies the minimum output for slippage protection
type MinAsset struct {
	Native *Asset `json:"native"`
}

// Asset represents a native token with denom and amount
type Asset struct {
	Amount string `json:"amount"`
	Denom  string `json:"denom"`
}

// PostSwapAction is the action to take after swapping (union type)
type PostSwapAction struct {
	IBCTransfer *IBCTransfer `json:"ibc_transfer,omitempty"`
	Transfer    *Transfer    `json:"transfer,omitempty"`
}

// Transfer sends tokens to an address on the same chain (post-swap)
type Transfer struct {
	ToAddress string `json:"to_address"`
}

// IBCTransfer sends tokens via IBC to another chain (post-swap)
type IBCTransfer struct {
	IBCInfo *IBCInfo `json:"ibc_info"`
}

// IBCInfo contains IBC transfer details for post-swap forwarding
type IBCInfo struct {
	Memo           string `json:"memo"`
	Receiver       string `json:"receiver"`
	RecoverAddress string `json:"recover_address"`
	SourceChannel  string `json:"source_channel"`
}

// ToJSON marshals the ForwardMemo to JSON string
func (m *ForwardMemo) ToJSON() (string, error) {
	bytes, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// ToJSON marshals the WasmMemo to JSON string
func (m *WasmMemo) ToJSON() (string, error) {
	bytes, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// ToJSON marshals the PFMForward to JSON string (for nested forwards)
func (f *PFMForward) ToJSON() (string, error) {
	wrapper := &ForwardMemo{Forward: f}
	return wrapper.ToJSON()
}
