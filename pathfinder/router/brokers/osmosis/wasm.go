package osmosis

import (
	"fmt"

	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/brokers"
	ibcmemo "github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/ibc_memo"
)

// Ensure SmartContractBuilder implements brokers.SmartContractBuilder
var _ brokers.SmartContractBuilder = (*SmartContractBuilder)(nil)

type SmartContractBuilder struct {
	contractAddress string
}

func NewSmartContractBuilder(contractAddress string) *SmartContractBuilder {
	return &SmartContractBuilder{
		contractAddress: contractAddress,
	}
}

// BuildSwapAndTransfer builds constructs a data structure that can be used to sign smart contract
// ( check ibc_memo doc.go case 3 ), the difference here is that the program will be sending the data
// like it is, not stringified.
func (b *SmartContractBuilder) BuildSwapAndTransfer(params ibcmemo.SwapMemoParams) (*ibcmemo.WasmMemo, error) {
	if b.contractAddress == "" {
		return nil, fmt.Errorf("ibc-hooks contract address not configured")
	}

	routeData, ok := params.RouteData.(*RouteData)
	if !ok {
		return nil, fmt.Errorf("route data is not Osmosis RouteData type")
	}

	operations := routeData.GetOperationsWithInput(params.TokenInDenom)
	if len(operations) == 0 {
		return nil, fmt.Errorf("no swap operations available")
	}

	data := ibcmemo.NewWasmMemo(
		b.contractAddress,
		ibcmemo.NewWasmMsg(
			ibcmemo.NewSwapAndAction(
				ibcmemo.NewUserSwap(SwapVenueName, operations),
				ibcmemo.NewMinAsset(params.TokenOutDenom, params.MinOutputAmount),
				params.TimeoutTimestamp,
				ibcmemo.NewTransferAction(params.ReceiverAddress),
			),
		),
	)

	return data, nil
}

// BuildSwapAndForward builds constructs a data structure that can be used to sign smart contract
// ( check ibc_memo doc.go case 4 ), the difference here is that the program will be sending the data
// like it is, not stringified.
func (b *SmartContractBuilder) BuildSwapAndForward(params ibcmemo.SwapAndForwardParams) (*ibcmemo.WasmMemo, error) {
	if b.contractAddress == "" {
		return nil, fmt.Errorf("ibc-hooks contract address not configured")
	}

	routeData, ok := params.RouteData.(*RouteData)
	if !ok {
		return nil, fmt.Errorf("route data is not Osmosis RouteData type")
	}

	operations := routeData.GetOperationsWithInput(params.TokenInDenom)
	if len(operations) == 0 {
		return nil, fmt.Errorf("no swap operations available")
	}

	data := ibcmemo.NewWasmMemo(
		b.contractAddress,
		ibcmemo.NewWasmMsg(
			ibcmemo.NewSwapAndAction(
				ibcmemo.NewUserSwap(SwapVenueName, operations),
				ibcmemo.NewMinAsset(params.TokenOutDenom, params.MinOutputAmount),
				params.TimeoutTimestamp,
				ibcmemo.NewIBCTransferAction(params.SourceChannel, params.ForwardReceiver, params.ForwardMemo, params.RecoverAddress),
			),
		),
	)

	return data, nil
}
