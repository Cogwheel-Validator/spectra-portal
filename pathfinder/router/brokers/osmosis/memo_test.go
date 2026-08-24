package osmosis

import (
	"encoding/json"
	"testing"

	ibcmemo "github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/ibc_memo"
	sqsquery "github.com/Cogwheel-Validator/spectra-portal/pathfinder/sqs_query"
	"github.com/zeebo/assert"
)

const testContract = "osmo10a3k4hvk37cc4hnxctw4p95fhscd2z6h2rmx0aukc6rm8u9qqx9smfsh7u"

// twoPoolRouteData returns route data for a swap that hops through two pools.
func twoPoolRouteData() *RouteData {
	return &RouteData{
		Routes: []Route{
			{
				Pools: []Pool{
					{ID: 1, TokenOutDenom: "uion"},
					{ID: 2, TokenOutDenom: "uosmo"},
				},
				InAmount:  "1000000",
				OutAmount: "990000",
			},
		},
	}
}

func swapParams(routeData any) ibcmemo.SwapMemoParams {
	return ibcmemo.SwapMemoParams{
		TokenInDenom:     "ibc/ATOM",
		TokenOutDenom:    "uosmo",
		MinOutputAmount:  "980100",
		RouteData:        routeData,
		TimeoutTimestamp: 1769790211797082680,
		RecoverAddress:   "osmo1recover",
		ReceiverAddress:  "osmo1receiver",
	}
}

func TestGetOperationsWithInput_ChainsDenoms(t *testing.T) {
	ops := twoPoolRouteData().GetOperationsWithInput("ibc/ATOM")

	assert.Equal(t, len(ops), 2)
	assert.Equal(t, ops[0].Pool, "1")
	assert.Equal(t, ops[0].DenomIn, "ibc/ATOM")
	assert.Equal(t, ops[0].DenomOut, "uion")
	// The output of pool 1 feeds pool 2
	assert.Equal(t, ops[1].Pool, "2")
	assert.Equal(t, ops[1].DenomIn, "uion")
	assert.Equal(t, ops[1].DenomOut, "uosmo")
}

func TestGetOperationsWithInput_EmptyRoutes(t *testing.T) {
	empty := &RouteData{}
	assert.Nil(t, empty.GetOperationsWithInput("uatom"))

	noPools := &RouteData{Routes: []Route{{}}}
	assert.Nil(t, noPools.GetOperationsWithInput("uatom"))
}

func TestConvertSqsResponseToRouteData(t *testing.T) {
	sqsResponse := sqsquery.RouteTokenResponse{
		AmountOut:            "990000",
		LiquidityCap:         "123456",
		LiquidityCapOverflow: true,
		Route: []sqsquery.Route{
			{
				Pools: []sqsquery.Pool{
					{ID: 1400, Type: 2, SpreadFactor: "0.002", TokenOutDenom: "uosmo", TakerFee: "0.001", LiquidityCap: "42"},
				},
				HasCwPool: true,
				InAmount:  "1000000",
				OutAmount: "990000",
			},
		},
	}

	routeData := ConvertSqsResponseToRouteData(sqsResponse)

	assert.Equal(t, routeData.LiquidityCap, "123456")
	assert.True(t, routeData.LiquidityCapOverflow)
	assert.Equal(t, len(routeData.Routes), 1)
	assert.True(t, routeData.Routes[0].HasCwPool)
	assert.Equal(t, routeData.Routes[0].InAmount, "1000000")
	assert.Equal(t, len(routeData.Routes[0].Pools), 1)
	pool := routeData.Routes[0].Pools[0]
	assert.Equal(t, pool.ID, int32(1400))
	assert.Equal(t, pool.TokenOutDenom, "uosmo")
	assert.Equal(t, pool.TakerFee, "0.001")
	assert.Equal(t, routeData.GetSwapVenueName(), SwapVenueName)
}

func TestBuildSwapMemo(t *testing.T) {
	builder := NewMemoBuilder(testContract)

	memoStr, err := builder.BuildSwapMemo(swapParams(twoPoolRouteData()))
	assert.NoError(t, err)

	var memo ibcmemo.WasmMemo
	assert.NoError(t, json.Unmarshal([]byte(memoStr), &memo))
	assert.NotNil(t, memo.Wasm)
	assert.Equal(t, memo.Wasm.Contract, testContract)

	action := memo.Wasm.Msg.SwapAndAction
	assert.NotNil(t, action)
	assert.Equal(t, action.TimeoutTimestamp, int64(1769790211797082680))

	swap := action.UserSwap.SwapExactAssetIn
	assert.Equal(t, swap.SwapVenueName, SwapVenueName)
	assert.Equal(t, len(swap.Operations), 2)
	assert.Equal(t, swap.Operations[0].DenomIn, "ibc/ATOM")
	assert.Equal(t, swap.Operations[1].DenomOut, "uosmo")

	assert.Equal(t, action.MinAsset.Native.Denom, "uosmo")
	assert.Equal(t, action.MinAsset.Native.Amount, "980100")

	// Post-swap action for a plain swap is a same-chain transfer
	assert.NotNil(t, action.PostSwapAction.Transfer)
	assert.Equal(t, action.PostSwapAction.Transfer.ToAddress, "osmo1receiver")
	assert.Nil(t, action.PostSwapAction.IBCTransfer)
}

func TestBuildSwapMemo_Errors(t *testing.T) {
	// No contract configured
	_, err := NewMemoBuilder("").BuildSwapMemo(swapParams(twoPoolRouteData()))
	assert.Error(t, err)

	builder := NewMemoBuilder(testContract)

	// Wrong RouteData type
	params := swapParams(twoPoolRouteData())
	params.RouteData = "not-route-data"
	_, err = builder.BuildSwapMemo(params)
	assert.Error(t, err)

	// No operations available
	_, err = builder.BuildSwapMemo(swapParams(&RouteData{}))
	assert.Error(t, err)
}

func TestBuildSwapAndForwardMemo(t *testing.T) {
	builder := NewMemoBuilder(testContract)

	params := ibcmemo.SwapAndForwardParams{
		SwapMemoParams:  swapParams(twoPoolRouteData()),
		SourceChannel:   "channel-42",
		ForwardReceiver: "juno1receiver",
		ForwardMemo:     "",
	}

	memoStr, err := builder.BuildSwapAndForwardMemo(params)
	assert.NoError(t, err)

	var memo ibcmemo.WasmMemo
	assert.NoError(t, json.Unmarshal([]byte(memoStr), &memo))

	action := memo.Wasm.Msg.SwapAndAction
	assert.Nil(t, action.PostSwapAction.Transfer)
	assert.NotNil(t, action.PostSwapAction.IBCTransfer)

	info := action.PostSwapAction.IBCTransfer.IBCInfo
	assert.Equal(t, info.SourceChannel, "channel-42")
	assert.Equal(t, info.Receiver, "juno1receiver")
	assert.Equal(t, info.RecoverAddress, "osmo1recover")
	assert.Equal(t, info.Memo, "")
}

func TestBuildSwapAndMultiHopMemo(t *testing.T) {
	builder := NewMemoBuilder(testContract)

	params := ibcmemo.SwapAndMultiHopParams{
		SwapMemoParams: swapParams(twoPoolRouteData()),
		OutboundHops: []ibcmemo.IBCHop{
			{Channel: "channel-750", Port: "transfer", Receiver: "noble1intermediate", Timeout: 100},
			{Channel: "channel-3", Port: "transfer", Receiver: "juno1final", Timeout: 100},
		},
		FinalReceiver: "juno1final",
	}

	memoStr, err := builder.BuildSwapAndMultiHopMemo(params)
	assert.NoError(t, err)

	var memo ibcmemo.WasmMemo
	assert.NoError(t, json.Unmarshal([]byte(memoStr), &memo))

	info := memo.Wasm.Msg.SwapAndAction.PostSwapAction.IBCTransfer.IBCInfo
	// First hop goes to the intermediate chain
	assert.Equal(t, info.SourceChannel, "channel-750")
	assert.Equal(t, info.Receiver, "noble1intermediate")

	// The remaining hop is a nested PFM forward embedded in the memo field
	var nested ibcmemo.ForwardMemo
	assert.NoError(t, json.Unmarshal([]byte(info.Memo), &nested))
	assert.Equal(t, nested.Forward.Channel, "channel-3")
	assert.Equal(t, nested.Forward.Receiver, "juno1final")
	assert.Equal(t, nested.Forward.Port, "transfer")
}

func TestBuildSwapAndMultiHopMemo_NoHops(t *testing.T) {
	builder := NewMemoBuilder(testContract)
	_, err := builder.BuildSwapAndMultiHopMemo(ibcmemo.SwapAndMultiHopParams{
		SwapMemoParams: swapParams(twoPoolRouteData()),
	})
	assert.Error(t, err)
}

func TestBuildForwardSwapMemo_SingleInboundHop(t *testing.T) {
	builder := NewMemoBuilder(testContract)

	params := ibcmemo.ForwardSwapParams{
		InboundHops: []ibcmemo.IBCHop{
			{Channel: "channel-141", Port: "transfer", Timeout: 100},
		},
		SwapParams: ibcmemo.SwapAndForwardParams{
			SwapMemoParams:  swapParams(twoPoolRouteData()),
			SourceChannel:   "channel-42",
			ForwardReceiver: "juno1receiver",
		},
	}

	memoStr, err := builder.BuildForwardSwapMemo(params)
	assert.NoError(t, err)

	var memo ibcmemo.ForwardMemo
	assert.NoError(t, json.Unmarshal([]byte(memoStr), &memo))
	assert.Equal(t, memo.Forward.Channel, "channel-141")
	// Last (only) hop with no explicit receiver targets the ibc-hooks contract
	assert.Equal(t, memo.Forward.Receiver, testContract)
	assert.NotNil(t, memo.Forward.Next)
	assert.NotNil(t, memo.Forward.Next.Wasm)
	assert.Equal(t, memo.Forward.Next.Wasm.Contract, testContract)
	assert.Nil(t, memo.Forward.Next.Forward)
}

func TestBuildForwardSwapMemo_TwoInboundHops(t *testing.T) {
	builder := NewMemoBuilder(testContract)

	params := ibcmemo.ForwardSwapParams{
		InboundHops: []ibcmemo.IBCHop{
			{Channel: "channel-0", Port: "transfer", Receiver: "cosmos1intermediate", Timeout: 100},
			{Channel: "channel-141", Port: "transfer", Timeout: 100},
		},
		SwapParams: ibcmemo.SwapAndForwardParams{
			SwapMemoParams:  swapParams(twoPoolRouteData()),
			SourceChannel:   "channel-42",
			ForwardReceiver: "juno1receiver",
		},
	}

	memoStr, err := builder.BuildForwardSwapMemo(params)
	assert.NoError(t, err)

	var memo ibcmemo.ForwardMemo
	assert.NoError(t, json.Unmarshal([]byte(memoStr), &memo))
	// Outer forward: first hop with the provided intermediate receiver
	assert.Equal(t, memo.Forward.Channel, "channel-0")
	assert.Equal(t, memo.Forward.Receiver, "cosmos1intermediate")
	// Inner forward: second hop to the broker contract, then wasm
	assert.NotNil(t, memo.Forward.Next.Forward)
	assert.Equal(t, memo.Forward.Next.Forward.Channel, "channel-141")
	assert.Equal(t, memo.Forward.Next.Forward.Receiver, testContract)
	assert.NotNil(t, memo.Forward.Next.Forward.Next)
	assert.NotNil(t, memo.Forward.Next.Forward.Next.Wasm)
}

func TestBuildForwardSwapMemo_NoHops(t *testing.T) {
	builder := NewMemoBuilder(testContract)
	_, err := builder.BuildForwardSwapMemo(ibcmemo.ForwardSwapParams{
		SwapParams: ibcmemo.SwapAndForwardParams{SwapMemoParams: swapParams(twoPoolRouteData())},
	})
	assert.Error(t, err)
}

func TestBuildForwardSwapForwardMemo(t *testing.T) {
	builder := NewMemoBuilder(testContract)

	params := ibcmemo.ForwardSwapForwardParams{
		InboundHops: []ibcmemo.IBCHop{
			{Channel: "channel-0", Port: "transfer", Receiver: "cosmos1intermediate", Timeout: 100},
			{Channel: "channel-141", Port: "transfer", Timeout: 100},
		},
		SwapParams: ibcmemo.SwapAndMultiHopParams{
			SwapMemoParams: swapParams(twoPoolRouteData()),
			OutboundHops: []ibcmemo.IBCHop{
				{Channel: "channel-750", Port: "transfer", Receiver: "noble1intermediate", Timeout: 100},
				{Channel: "channel-3", Port: "transfer", Receiver: "juno1final", Timeout: 100},
			},
			FinalReceiver: "juno1final",
		},
	}

	memoStr, err := builder.BuildForwardSwapForwardMemo(params)
	assert.NoError(t, err)

	var memo ibcmemo.ForwardMemo
	assert.NoError(t, json.Unmarshal([]byte(memoStr), &memo))

	// Inbound: forward -> forward -> wasm
	assert.Equal(t, memo.Forward.Channel, "channel-0")
	assert.NotNil(t, memo.Forward.Next.Forward)
	wasm := memo.Forward.Next.Forward.Next.Wasm
	assert.NotNil(t, wasm)

	// Outbound inside the wasm action: first hop + nested forward for the second
	info := wasm.Msg.SwapAndAction.PostSwapAction.IBCTransfer.IBCInfo
	assert.Equal(t, info.SourceChannel, "channel-750")
	assert.Equal(t, info.Receiver, "noble1intermediate")

	var nested ibcmemo.ForwardMemo
	assert.NoError(t, json.Unmarshal([]byte(info.Memo), &nested))
	assert.Equal(t, nested.Forward.Channel, "channel-3")
	assert.Equal(t, nested.Forward.Receiver, "juno1final")
}

func TestBuildHopAndSwapMemo(t *testing.T) {
	builder := NewMemoBuilder(testContract)

	params := ibcmemo.HopAndSwapParams{
		InboundHops: []ibcmemo.IBCHop{
			{Channel: "channel-1", Port: "transfer", Timeout: 100},
			{Channel: "channel-141", Port: "transfer", Timeout: 100},
		},
		SwapParams: ibcmemo.SwapAndForwardParams{
			SwapMemoParams: swapParams(twoPoolRouteData()),
		},
	}

	memoStr, err := builder.BuildHopAndSwapMemo(params)
	assert.NoError(t, err)

	var memo ibcmemo.ForwardMemo
	assert.NoError(t, json.Unmarshal([]byte(memoStr), &memo))
	// The memo describes only the second hop (intermediate -> broker)
	assert.Equal(t, memo.Forward.Channel, "channel-141")
	assert.Equal(t, memo.Forward.Receiver, testContract)
	assert.NotNil(t, memo.Forward.Next.Wasm)
	// After the swap the tokens stay on the broker with the receiver
	transfer := memo.Forward.Next.Wasm.Msg.SwapAndAction.PostSwapAction.Transfer
	assert.NotNil(t, transfer)
	assert.Equal(t, transfer.ToAddress, "osmo1receiver")
}

func TestBuildHopAndSwapMemo_HopCountValidation(t *testing.T) {
	builder := NewMemoBuilder(testContract)
	base := ibcmemo.SwapAndForwardParams{SwapMemoParams: swapParams(twoPoolRouteData())}
	hop := ibcmemo.IBCHop{Channel: "channel-1", Port: "transfer", Timeout: 100}

	// Zero, one, and three+ hops are all unsupported
	for _, hops := range [][]ibcmemo.IBCHop{
		{},
		{hop},
		{hop, hop, hop},
	} {
		_, err := builder.BuildHopAndSwapMemo(ibcmemo.HopAndSwapParams{InboundHops: hops, SwapParams: base})
		assert.Error(t, err)
	}
}

func TestSmartContractBuilder(t *testing.T) {
	builder := NewSmartContractBuilder(testContract)

	// Swap and transfer on the same chain
	data, err := builder.BuildSwapAndTransfer(swapParams(twoPoolRouteData()))
	assert.NoError(t, err)
	assert.Equal(t, data.Wasm.Contract, testContract)
	assert.NotNil(t, data.Wasm.Msg.SwapAndAction.PostSwapAction.Transfer)
	assert.Equal(t, data.Wasm.Msg.SwapAndAction.PostSwapAction.Transfer.ToAddress, "osmo1receiver")

	// Swap then forward over IBC
	forward, err := builder.BuildSwapAndForward(ibcmemo.SwapAndForwardParams{
		SwapMemoParams:  swapParams(twoPoolRouteData()),
		SourceChannel:   "channel-42",
		ForwardReceiver: "juno1receiver",
	})
	assert.NoError(t, err)
	assert.NotNil(t, forward.Wasm.Msg.SwapAndAction.PostSwapAction.IBCTransfer)
	assert.Equal(t,
		forward.Wasm.Msg.SwapAndAction.PostSwapAction.IBCTransfer.IBCInfo.SourceChannel, "channel-42")

	// Missing contract address errors
	empty := NewSmartContractBuilder("")
	_, err = empty.BuildSwapAndTransfer(swapParams(twoPoolRouteData()))
	assert.Error(t, err)
	_, err = empty.BuildSwapAndForward(ibcmemo.SwapAndForwardParams{SwapMemoParams: swapParams(twoPoolRouteData())})
	assert.Error(t, err)
}
