package ibcmemo

import "encoding/json"

type NextEnum string

const (
	NextEnumWasm       NextEnum = "wasm"
	NextEnumPFMForward NextEnum = "pfm_forward"
)

type PostSwapActionEnum string

const (
	PostSwapActionEnumTransfer    PostSwapActionEnum = "transfer"
	PostSwapActionEnumIBCTransfer PostSwapActionEnum = "ibc_transfer"
)

var SwapVenueNames = []string{
	"osmosis-poolmanager",
}

type PFMForward struct {
	Channel  string `json:"channel"`
	Next     *Next  `json:"next,omitempty"`
	Port     string `json:"port"`
	Receiver string `json:"receiver"`
	Retries  int    `json:"retries"`
	Timeout  int64  `json:"timeout"`
}

func (p *PFMForward) ToString() string {
	json, err := json.Marshal(p)
	if err != nil {
		return ""
	}
	return string(json)
}

type Wasm struct {
	WasmData WasmData `json:"wasm"`
}

type WasmData struct {
	Contract string `json:"contract"`
	Msg      any    `json:"msg"`
}

func (w *Wasm) ToString() string {
	json, err := json.Marshal(w)
	if err != nil {
		return ""
	}
	return string(json)
}

type WasmMsg struct {
	SwapAndAction *SwapAndAction `json:"swap_and_action,omitempty"`
}

type SwapAndAction struct {
	Affiliates       []any           `json:"affiliates"` // show even when empty!
	MinAsset         *MinAsset       `json:"min_asset,omitempty"`
	PostSwapAction   *PostSwapAction `json:"post_swap_action,omitempty"`
	TimeoutTimestamp int64           `json:"timeout_timestamp,omitempty"`
	UserSwap         any             `json:"user_swap,omitempty"`
}

type MinAsset struct {
	Native Asset `json:"native"`
}

type Asset struct {
	Amount string `json:"amount"`
	Denom  string `json:"denom"`
}

type PostSwapAction struct {
	PostSwapActionEnum PostSwapActionEnum `json:"-"` // not using json reflection
	IBCTransfer        *IBCTransfer       `json:"ibc_transfer,omitempty"`
	Transfer           *Transfer          `json:"transfer,omitempty"`
}

type IBCTransfer struct {
	IBCInfo IBCInfo `json:"ibc_info"`
}

type IBCInfo struct {
	Memo           string `json:"memo"`
	Receiver       string `json:"receiver"`
	RecoverAddress string `json:"recover_address"`
	SourceChannel  string `json:"source_channel"`
}

type Transfer struct {
	ToAddress string `json:"to_address"`
}

type UserSwap struct {
	SwapExactAssetIn *SwapExactAssetIn `json:"swap_exact_asset_in"`
}

type SwapExactAssetIn struct {
	SwapVenueName string          `json:"swap_venue_name"`
	Operations    []SwapOperation `json:"operations"`
}

type SwapOperation struct {
	Pool     string `json:"pool"`
	DenomIn  string `json:"denom_in"`
	DenomOut string `json:"denom_out"`
	// This is tied to some other DEXs, like Injective has, not all of them
	// have this but just keep it for now.
	Interface *string `json:"interface,omitempty"`
}

type Next struct {
	NextEnum   NextEnum    `json:"-"` // not using json reflection
	Wasm       *Wasm       `json:"wasm,omitempty"`
	PFMForward *PFMForward `json:"pfm_forward,omitempty"`
}
