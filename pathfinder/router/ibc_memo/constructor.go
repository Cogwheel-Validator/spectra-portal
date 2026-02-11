package ibcmemo

import (
	"time"
)

// NewWasmMemo creates a new wasm memo with the given contract and message
func NewWasmMemo(contractAddress string, msg *WasmMsg) *WasmMemo {
	return &WasmMemo{
		Wasm: &WasmData{
			Contract: contractAddress,
			Msg:      msg,
		},
	}
}

// NewWasmMsg creates a new WasmMsg with swap_and_action
func NewWasmMsg(swapAndAction *SwapAndAction) *WasmMsg {
	return &WasmMsg{
		SwapAndAction: swapAndAction,
	}
}

// NewSwapAndAction creates a new SwapAndAction with all required fields
func NewSwapAndAction(
	userSwap *UserSwap,
	minAsset *MinAsset,
	timeoutTimestamp int64,
	postSwapAction *PostSwapAction,
) *SwapAndAction {
	return &SwapAndAction{
		UserSwap:         userSwap,
		MinAsset:         minAsset,
		TimeoutTimestamp: timeoutTimestamp,
		PostSwapAction:   postSwapAction,
		Affiliates:       []interface{}{}, // Always empty array
	}
}

// NewUserSwap creates a new UserSwap with swap_exact_asset_in
func NewUserSwap(swapVenueName string, operations []SwapOperation) *UserSwap {
	return &UserSwap{
		SwapExactAssetIn: &SwapExactAssetIn{
			SwapVenueName: swapVenueName,
			Operations:    operations,
		},
	}
}

// NewSwapOperation creates a single swap operation (pool hop)
func NewSwapOperation(pool, denomIn, denomOut string) SwapOperation {
	return SwapOperation{
		Pool:     pool,
		DenomIn:  denomIn,
		DenomOut: denomOut,
	}
}

// NewSwapOperationWithInterface creates a swap operation with interface (for DEXs like Injective)
func NewSwapOperationWithInterface(pool, denomIn, denomOut string, iface string) SwapOperation {
	return SwapOperation{
		Pool:      pool,
		DenomIn:   denomIn,
		DenomOut:  denomOut,
		Interface: &iface,
	}
}

// NewMinAsset creates a MinAsset for slippage protection
func NewMinAsset(denom, amount string) *MinAsset {
	return &MinAsset{
		Native: &Asset{
			Denom:  denom,
			Amount: amount,
		},
	}
}

// NewTransferAction creates a PostSwapAction for same-chain transfer
func NewTransferAction(toAddress string) *PostSwapAction {
	return &PostSwapAction{
		Transfer: &Transfer{
			ToAddress: toAddress,
		},
	}
}

// NewIBCTransferAction creates a PostSwapAction for IBC transfer
func NewIBCTransferAction(sourceChannel, receiver, memo, recoverAddress string) *PostSwapAction {
	return &PostSwapAction{
		IBCTransfer: &IBCTransfer{
			IBCInfo: &IBCInfo{
				SourceChannel:  sourceChannel,
				Receiver:       receiver,
				Memo:           memo,
				RecoverAddress: recoverAddress,
			},
		},
	}
}

// NewForwardMemo creates a simple forward memo (case 1 from doc.go)
func NewForwardMemo(channel, port, receiver string, retries int, timeout int64) *ForwardMemo {
	return &ForwardMemo{
		Forward: &PFMForward{
			Channel:  channel,
			Port:     port,
			Receiver: receiver,
			Retries:  retries,
			Timeout:  timeout,
		},
	}
}

// NewForwardMemoWithNext creates a forward memo with a next action
func NewForwardMemoWithNext(channel, port, receiver string, retries int, timeout int64, next *PFMNext) *ForwardMemo {
	return &ForwardMemo{
		Forward: &PFMForward{
			Channel:  channel,
			Port:     port,
			Receiver: receiver,
			Retries:  retries,
			Timeout:  timeout,
			Next:     next,
		},
	}
}

// NewPFMNextWithWasm creates a PFMNext that chains to a wasm action
func NewPFMNextWithWasm(wasmMemo *WasmMemo) *PFMNext {
	return &PFMNext{
		Wasm: wasmMemo.Wasm,
	}
}

// NewPFMNextWithForward creates a PFMNext that chains to another forward
func NewPFMNextWithForward(forward *PFMForward) *PFMNext {
	return &PFMNext{
		Forward: forward,
	}
}

// NewNestedForward creates a nested forward structure (for building chains)
func NewNestedForward(channel, port, receiver string, retries int, timeout int64, next *PFMNext) *PFMForward {
	return &PFMForward{
		Channel:  channel,
		Port:     port,
		Receiver: receiver,
		Retries:  retries,
		Timeout:  timeout,
		Next:     next,
	}
}

// DefaultTimeoutTimestamp returns a default timeout (15 minutes from now) in nanoseconds
func DefaultTimeoutTimestamp() int64 {
	return time.Now().Add(15 * time.Minute).UnixNano()
}

// DefaultRetries returns the default number of retries for PFM
func DefaultRetries() int {
	return 2
}

// DefaultPort returns the default IBC port
func DefaultPort() string {
	return "transfer"
}
