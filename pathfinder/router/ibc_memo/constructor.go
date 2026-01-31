package ibcmemo

import (
	"fmt"
	"slices"
)

func NewWasm(contractAddress string, msg *WasmMsg) *Wasm {
	wasmData := newWasmData(contractAddress, msg)
	return &Wasm{
		WasmData: *wasmData,
	}
}

func newWasmData(contractAddress string, msg *WasmMsg) *WasmData {
	return &WasmData{
		Contract: contractAddress,
		Msg:      msg,
	}
}

func NewSwapOperation(
	pool string,
	denomIn string,
	denomOut string,
	interfaceValue *string) *SwapOperation {
	return &SwapOperation{
		Pool:      pool,
		DenomIn:   denomIn,
		DenomOut:  denomOut,
		Interface: interfaceValue,
	}
}

func NewSwapExactAssetIn(
	swapVenueName string,
	operations []SwapOperation) (*SwapExactAssetIn, error) {
	if !slices.Contains(SwapVenueNames, swapVenueName) {
		return nil, fmt.Errorf("invalid swap venue name: %s", swapVenueName)
	}
	return &SwapExactAssetIn{
		SwapVenueName: swapVenueName,
		Operations:    operations,
	}, nil
}

// Union type constructors with validation
func NewNextWithWasm(wasm *Wasm) *Next {
	return &Next{
		NextEnum: NextEnumWasm,
		Wasm:     wasm,
	}
}

func NewNextWithPFMForward(forward *PFMForward) *Next {
	return &Next{
		NextEnum:   NextEnumPFMForward,
		PFMForward: forward,
	}
}

func NewPostSwapActionWithTransfer(transfer *Transfer) *PostSwapAction {
	return &PostSwapAction{
		PostSwapActionEnum: PostSwapActionEnumTransfer,
		Transfer:           transfer,
	}
}

func NewPostSwapActionWithIBCTransfer(ibc *IBCTransfer) *PostSwapAction {
	return &PostSwapAction{
		PostSwapActionEnum: PostSwapActionEnumIBCTransfer,
		IBCTransfer:        ibc,
	}
}

func NewPFMForward(
	channel,
	port,
	receiver string,
	retries int,
	timeout int64,
	next *Next,
) (*PFMForward, error) {
	if retries < 0 {
		return nil, fmt.Errorf("retries must be greater than 0")
	}
	if timeout < 0 {
		return nil, fmt.Errorf("timeout must be greater than 0")
	}
	return &PFMForward{
		Channel:  channel,
		Port:     port,
		Receiver: receiver,
		Retries:  retries,
		Timeout:  timeout,
		Next:     next,
	}, nil
}

func NewIBCTransfer(sourceChannel, receiver, recoverAddress, memo string) *IBCTransfer {
	return &IBCTransfer{
		IBCInfo: IBCInfo{
			SourceChannel:  sourceChannel,
			Receiver:       receiver,
			RecoverAddress: recoverAddress,
			Memo:           memo,
		},
	}
}

func NewMinAsset(amount, denom string) *MinAsset {
	return &MinAsset{
		Native: Asset{
			Amount: amount,
			Denom:  denom,
		},
	}
}
