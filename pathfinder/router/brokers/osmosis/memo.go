package osmosis

import (
	"fmt"

	ibcmemo "github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/router/ibc_memo"
)

// MemoBuilder builds IBC memo structures for Osmosis swap operations
// using the Skip Go ibc-hooks entry point contract.
// Implements ibcmemo.MemoBuilder interface.
type MemoBuilder struct {
	contractAddress string
}

// NewMemoBuilder creates a new Osmosis memo builder for the given entry point contract
func NewMemoBuilder(contractAddress string) *MemoBuilder {
	return &MemoBuilder{
		contractAddress: contractAddress,
	}
}

// GetContractAddress returns the ibc-hooks contract address
func (b *MemoBuilder) GetContractAddress() string {
	return b.contractAddress
}

// BuildSwapMemo creates a wasm memo for swap operations (case 2 from doc.go: Transfer and Swap).
// Used when: Source -> Broker (swap) -> stays on Broker
//
// Example output:
//
//	{
//	  "wasm": {
//	    "contract": "osmo10a3k4...",
//	    "msg": {
//	      "swap_and_action": {
//	        "user_swap": { "swap_exact_asset_in": { ... } },
//	        "min_asset": { "native": { "denom": "...", "amount": "..." } },
//	        "timeout_timestamp": 1769790211797082680,
//	        "post_swap_action": { "transfer": { "to_address": "osmo1..." } },
//	        "affiliates": []
//	      }
//	    }
//	  }
//	}
func (b *MemoBuilder) BuildSwapMemo(params ibcmemo.SwapMemoParams) (string, error) {
	if b.contractAddress == "" {
		return "", fmt.Errorf("ibc-hooks contract address not configured")
	}

	routeData, ok := params.RouteData.(*RouteData)
	if !ok {
		return "", fmt.Errorf("route data is not Osmosis RouteData type")
	}

	operations := routeData.GetOperationsWithInput(params.TokenInDenom)
	if len(operations) == 0 {
		return "", fmt.Errorf("no swap operations available")
	}

	memo := ibcmemo.NewWasmMemo(
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

	return memo.ToJSON()
}

// BuildSwapAndForwardMemo creates a wasm memo for swap + IBC forward (case 5.1 from doc.go).
// Used when: Source -> Broker (swap) -> Destination (single hop)
//
// Example output:
//
//	{
//	  "wasm": {
//	    "contract": "osmo10a3k4...",
//	    "msg": {
//	      "swap_and_action": {
//	        "user_swap": { ... },
//	        "min_asset": { ... },
//	        "timeout_timestamp": ...,
//	        "post_swap_action": {
//	          "ibc_transfer": {
//	            "ibc_info": {
//	              "source_channel": "channel-253",
//	              "receiver": "noble1...",
//	              "memo": "",
//	              "recover_address": "osmo1..."
//	            }
//	          }
//	        },
//	        "affiliates": []
//	      }
//	    }
//	  }
//	}
func (b *MemoBuilder) BuildSwapAndForwardMemo(params ibcmemo.SwapAndForwardParams) (string, error) {
	if b.contractAddress == "" {
		return "", fmt.Errorf("ibc-hooks contract address not configured")
	}

	routeData, ok := params.RouteData.(*RouteData)
	if !ok {
		return "", fmt.Errorf("route data is not Osmosis RouteData type")
	}

	operations := routeData.GetOperationsWithInput(params.TokenInDenom)
	if len(operations) == 0 {
		return "", fmt.Errorf("no swap operations available")
	}

	memo := ibcmemo.NewWasmMemo(
		b.contractAddress,
		ibcmemo.NewWasmMsg(
			ibcmemo.NewSwapAndAction(
				ibcmemo.NewUserSwap(SwapVenueName, operations),
				ibcmemo.NewMinAsset(params.TokenOutDenom, params.MinOutputAmount),
				params.TimeoutTimestamp,
				ibcmemo.NewIBCTransferAction(
					params.SourceChannel,
					params.ForwardReceiver,
					params.ForwardMemo,
					params.RecoverAddress,
				),
			),
		),
	)

	return memo.ToJSON()
}

// BuildSwapAndMultiHopMemo creates a wasm memo for swap + multi-hop forward (case 5.3 from doc.go).
// Used when: Source -> Broker (swap) -> Intermediate -> Destination
//
// For intermediate PFM hops, uses "pfm" as receiver (security feature).
// The nested forward is embedded in the ibc_transfer memo field.
// Example output:
//
//	{
//	  "wasm": {
//	    "contract": "osmo10a3k4...",
//	    "msg": {
//	      "swap_and_action": {
//	        ...
//	        "post_swap_action": {
//	          "ibc_transfer": {
//	            "ibc_info": {
//	              "source_channel": "channel-750",
//	              "receiver": "pfm",  // invalid bech32 for intermediate chain
//	              "memo": "{\"forward\":{\"channel\":\"channel-3\",\"port\":\"transfer\",\"receiver\":\"juno1...\",\"retries\":2,\"timeout\":...}}",
//	              "recover_address": "osmo1..."
//	            }
//	          }
//	        },
//	        "affiliates": []
//	      }
//	    }
//	  }
//	}
func (b *MemoBuilder) BuildSwapAndMultiHopMemo(params ibcmemo.SwapAndMultiHopParams) (string, error) {
	if b.contractAddress == "" {
		return "", fmt.Errorf("ibc-hooks contract address not configured")
	}

	if len(params.OutboundHops) == 0 {
		return "", fmt.Errorf("no outbound hops provided")
	}

	routeData, ok := params.RouteData.(*RouteData)
	if !ok {
		return "", fmt.Errorf("route data is not Osmosis RouteData type")
	}

	operations := routeData.GetOperationsWithInput(params.TokenInDenom)
	if len(operations) == 0 {
		return "", fmt.Errorf("no swap operations available")
	}

	// Build nested forward memo for hops after the first one
	forwardMemo := ""
	if len(params.OutboundHops) > 1 {
		nestedForward := ibcmemo.BuildNestedForwardMemo(params.OutboundHops[1:], params.FinalReceiver)
		var err error
		forwardMemo, err = nestedForward.ToJSON()
		if err != nil {
			return "", fmt.Errorf("failed to build nested forward memo: %w", err)
		}
	}

	// First hop receiver:
	// - If single hop: final receiver
	// - If multi-hop: use "pfm" for intermediate chain security
	firstHopReceiver := params.FinalReceiver
	if len(params.OutboundHops) > 1 {
		firstHopReceiver = ibcmemo.PFMIntermediateReceiver
	}

	memo := ibcmemo.NewWasmMemo(
		b.contractAddress,
		ibcmemo.NewWasmMsg(
			ibcmemo.NewSwapAndAction(
				ibcmemo.NewUserSwap(SwapVenueName, operations),
				ibcmemo.NewMinAsset(params.TokenOutDenom, params.MinOutputAmount),
				params.TimeoutTimestamp,
				ibcmemo.NewIBCTransferAction(
					params.OutboundHops[0].Channel,
					firstHopReceiver,
					forwardMemo,
					params.RecoverAddress,
				),
			),
		),
	)

	return memo.ToJSON()
}

// BuildForwardSwapMemo creates a forward memo wrapping wasm (case 5.2 from doc.go).
// Used when: Source -> Intermediate(s) (forward) -> Broker (swap) -> Destination
// Supports multiple inbound hops: Source -> Int1 -> Int2 -> Broker (swap) -> Dest
//
// For intermediate PFM hops, uses "pfm" as receiver (security feature).
// Example with single inbound hop:
//
//	{
//	  "forward": {
//	    "channel": "channel-141",
//	    "port": "transfer",
//	    "receiver": "osmo10a3k4...",  // contract address
//	    "retries": 2,
//	    "timeout": ...,
//	    "next": {
//	      "wasm": { ... }
//	    }
//	  }
//	}
//
// Example with two inbound hops:
//
//	{
//	  "forward": {
//	    "channel": "channel-0",
//	    "port": "transfer",
//	    "receiver": "pfm",  // invalid bech32 for intermediate chain
//	    "retries": 2,
//	    "timeout": ...,
//	    "next": {
//	      "forward": {
//	        "channel": "channel-141",
//	        "receiver": "osmo10a3k4...",
//	        "next": { "wasm": { ... } }
//	      }
//	    }
//	  }
//	}
func (b *MemoBuilder) BuildForwardSwapMemo(params ibcmemo.ForwardSwapParams) (string, error) {
	if b.contractAddress == "" {
		return "", fmt.Errorf("ibc-hooks contract address not configured")
	}

	if len(params.InboundHops) == 0 {
		return "", fmt.Errorf("no inbound hops provided")
	}

	routeData, ok := params.SwapParams.RouteData.(*RouteData)
	if !ok {
		return "", fmt.Errorf("route data is not Osmosis RouteData type")
	}

	operations := routeData.GetOperationsWithInput(params.SwapParams.TokenInDenom)
	if len(operations) == 0 {
		return "", fmt.Errorf("no swap operations available")
	}

	// Build the inner wasm memo (executed on broker chain after all forwards)
	wasmMemo := ibcmemo.NewWasmMemo(
		b.contractAddress,
		ibcmemo.NewWasmMsg(
			ibcmemo.NewSwapAndAction(
				ibcmemo.NewUserSwap(SwapVenueName, operations),
				ibcmemo.NewMinAsset(params.SwapParams.TokenOutDenom, params.SwapParams.MinOutputAmount),
				params.SwapParams.TimeoutTimestamp,
				ibcmemo.NewIBCTransferAction(
					params.SwapParams.SourceChannel,
					params.SwapParams.ForwardReceiver,
					params.SwapParams.ForwardMemo,
					params.SwapParams.RecoverAddress,
				),
			),
		),
	)

	// Build the nested forward structure from innermost (broker) to outermost (source)
	// Start with the wasm memo as the innermost "next"
	var currentNext *ibcmemo.PFMNext = ibcmemo.NewPFMNextWithWasm(wasmMemo)

	// Build forwards from last hop (closest to broker) to first hop (closest to source)
	for i := len(params.InboundHops) - 1; i >= 0; i-- {
		hop := params.InboundHops[i]

		// Determine receiver for this hop:
		// - Last hop (i == len-1): receiver is the contract address
		// - Other hops: use "pfm" for intermediate chain security
		receiver := ibcmemo.PFMIntermediateReceiver
		if i == len(params.InboundHops)-1 {
			receiver = b.contractAddress
		}

		if i == 0 {
			// First hop - this becomes the top-level forward memo
			forwardMemo := ibcmemo.NewForwardMemoWithNext(
				hop.Channel,
				hop.Port,
				receiver,
				ibcmemo.DefaultRetries(),
				hop.Timeout,
				currentNext,
			)
			return forwardMemo.ToJSON()
		}

		// Intermediate hop - wrap in another forward
		nestedForward := ibcmemo.NewNestedForward(
			hop.Channel,
			hop.Port,
			receiver,
			ibcmemo.DefaultRetries(),
			hop.Timeout,
			currentNext,
		)
		currentNext = ibcmemo.NewPFMNextWithForward(nestedForward)
	}

	// Should not reach here if InboundHops is not empty
	return "", fmt.Errorf("unexpected error building forward memo")
}

// BuildForwardSwapForwardMemo creates a forward memo wrapping wasm with nested forward (case 5.4).
// Used when: Source -> Intermediate(s) (forward) -> Broker (swap) -> Intermediate(s) -> Destination
//
// This is the most complex case combining all patterns.
// For all intermediate PFM hops (both inbound and outbound), uses "pfm" as receiver.
func (b *MemoBuilder) BuildForwardSwapForwardMemo(params ibcmemo.ForwardSwapForwardParams) (string, error) {
	if b.contractAddress == "" {
		return "", fmt.Errorf("ibc-hooks contract address not configured")
	}

	if len(params.InboundHops) == 0 {
		return "", fmt.Errorf("no inbound hops provided")
	}

	if len(params.SwapParams.OutboundHops) == 0 {
		return "", fmt.Errorf("no outbound hops provided")
	}

	routeData, ok := params.SwapParams.RouteData.(*RouteData)
	if !ok {
		return "", fmt.Errorf("route data is not Osmosis RouteData type")
	}

	operations := routeData.GetOperationsWithInput(params.SwapParams.TokenInDenom)
	if len(operations) == 0 {
		return "", fmt.Errorf("no swap operations available")
	}

	// Build nested forward memo for outbound hops after the first one
	outboundForwardMemo := ""
	if len(params.SwapParams.OutboundHops) > 1 {
		nestedForward := ibcmemo.BuildNestedForwardMemo(params.SwapParams.OutboundHops[1:], params.SwapParams.FinalReceiver)
		var err error
		outboundForwardMemo, err = nestedForward.ToJSON()
		if err != nil {
			return "", fmt.Errorf("failed to build outbound forward memo: %w", err)
		}
	}

	// First outbound hop receiver:
	// - If single outbound hop: final receiver
	// - If multi-hop outbound: use "pfm" for intermediate chain security
	firstOutboundReceiver := params.SwapParams.FinalReceiver
	if len(params.SwapParams.OutboundHops) > 1 {
		firstOutboundReceiver = ibcmemo.PFMIntermediateReceiver
	}

	// Build the inner wasm memo with IBC transfer
	wasmMemo := ibcmemo.NewWasmMemo(
		b.contractAddress,
		ibcmemo.NewWasmMsg(
			ibcmemo.NewSwapAndAction(
				ibcmemo.NewUserSwap(SwapVenueName, operations),
				ibcmemo.NewMinAsset(params.SwapParams.TokenOutDenom, params.SwapParams.MinOutputAmount),
				params.SwapParams.TimeoutTimestamp,
				ibcmemo.NewIBCTransferAction(
					params.SwapParams.OutboundHops[0].Channel,
					firstOutboundReceiver,
					outboundForwardMemo,
					params.SwapParams.RecoverAddress,
				),
			),
		),
	)

	// Build the nested forward structure from innermost (broker) to outermost (source)
	var currentNext *ibcmemo.PFMNext = ibcmemo.NewPFMNextWithWasm(wasmMemo)

	// Build forwards from last hop (closest to broker) to first hop (closest to source)
	for i := len(params.InboundHops) - 1; i >= 0; i-- {
		hop := params.InboundHops[i]

		// Determine receiver for this hop:
		// - Last hop (i == len-1): receiver is the contract address
		// - Other hops: use "pfm" for intermediate chain security
		receiver := ibcmemo.PFMIntermediateReceiver
		if i == len(params.InboundHops)-1 {
			receiver = b.contractAddress
		}

		if i == 0 {
			// First hop - this becomes the top-level forward memo
			forwardMemo := ibcmemo.NewForwardMemoWithNext(
				hop.Channel,
				hop.Port,
				receiver,
				ibcmemo.DefaultRetries(),
				hop.Timeout,
				currentNext,
			)
			return forwardMemo.ToJSON()
		}

		// Intermediate hop - wrap in another forward
		nestedForward := ibcmemo.NewNestedForward(
			hop.Channel,
			hop.Port,
			receiver,
			ibcmemo.DefaultRetries(),
			hop.Timeout,
			currentNext,
		)
		currentNext = ibcmemo.NewPFMNextWithForward(nestedForward)
	}

	// Should not reach here if InboundHops is not empty
	return "", fmt.Errorf("unexpected error building forward memo")
}
