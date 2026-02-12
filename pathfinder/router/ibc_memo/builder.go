package ibcmemo

const (
	// PFMIntermediateReceiver is used as receiver address on intermediate PFM chains.
	// Using an invalid bech32 string ensures:
	// 1. Security: PFM creates a hash of sender+channel for the actual receiver
	// 2. Safety: If sent to a chain without PFM, the transfer fails and refunds properly
	// See: https://github.com/cosmos/ibc-apps/tree/main/middleware/packet-forward-middleware
	PFMIntermediateReceiver = "pfm"
)

// MemoBuilder is the interface for building IBC memos across different broker chains.
// Each broker (Osmosis, Neutron, etc.) implements this interface with their specific logic.
type MemoBuilder interface {
	// BuildSwapMemo creates a wasm memo for swap operations (case 2 from doc.go: Transfer and Swap)
	// This is used when sending tokens to a broker chain to swap and keep on that chain.
	BuildSwapMemo(params SwapMemoParams) (string, error)

	// BuildSwapAndForwardMemo creates a wasm memo for swap + IBC forward (case 5.1 from doc.go)
	// This is used when sending tokens to a broker chain, swapping, and forwarding to destination.
	BuildSwapAndForwardMemo(params SwapAndForwardParams) (string, error)

	// BuildSwapAndMultiHopMemo creates a wasm memo for swap + multi-hop forward (case 5.3 from doc.go)
	// This is used when the outbound path requires multiple IBC hops after the swap.
	BuildSwapAndMultiHopMemo(params SwapAndMultiHopParams) (string, error)

	// BuildForwardSwapMemo creates a forward memo wrapping wasm (case 5.2 from doc.go)
	// This is used when the inbound path requires forwarding through intermediate chain(s) before swap.
	// Supports both single inbound hop and multiple inbound hops (forward -> forward -> wasm).
	BuildForwardSwapMemo(params ForwardSwapParams) (string, error)

	// BuildForwardSwapForwardMemo creates a forward memo wrapping wasm with nested forward (case 5.4)
	// This is the most complex case: multi-hop inbound + swap + multi-hop outbound.
	BuildForwardSwapForwardMemo(params ForwardSwapForwardParams) (string, error)

	// BuildHopAndSwapMemo creates a memo for a hop and swap (case 6.1 from doc.go)
	// This is used when there is transfer from regular chain that needs to be
	// routed through another chain and then swapped on the broker chain
	BuildHopAndSwapMemo(params HopAndSwapParams) (string, error)

	// GetContractAddress returns the ibc-hooks entry point contract address
	GetContractAddress() string
}

// SwapMemoParams contains parameters for building a simple swap memo (case 2).
// Used when: Source -> Broker (swap) -> stays on Broker
type SwapMemoParams struct {
	// TokenInDenom is the denom of the input token on the broker chain (typically IBC denom)
	TokenInDenom string
	// TokenOutDenom is the denom of the output token on the broker chain
	TokenOutDenom string
	// MinOutputAmount is the minimum output amount (for slippage protection)
	MinOutputAmount string
	// RouteData contains broker-specific routing information (pools, etc.)
	// Each broker implementation casts this to their specific type.
	RouteData interface{}
	// TimeoutTimestamp is the timeout in nanoseconds
	TimeoutTimestamp int64
	// RecoverAddress is where funds go if the swap fails (on broker chain)
	RecoverAddress string
	// ReceiverAddress is where tokens go after swap (on broker chain)
	ReceiverAddress string
}

// SwapAndForwardParams contains parameters for swap + single IBC forward (case 5.1).
// Used when: Source -> Broker (swap) -> Destination (single hop)
type SwapAndForwardParams struct {
	SwapMemoParams
	// SourceChannel is the IBC channel from broker to destination
	SourceChannel string
	// ForwardReceiver is the address on the destination chain
	ForwardReceiver string
	// ForwardMemo is an optional memo for the IBC transfer (usually empty)
	ForwardMemo string
}

// SwapAndMultiHopParams contains parameters for swap + multi-hop forward (case 5.3).
// Used when: Source -> Broker (swap) -> Intermediate -> Destination
type SwapAndMultiHopParams struct {
	SwapMemoParams
	// OutboundHops contains the IBC hops after the swap
	OutboundHops []IBCHop
	// FinalReceiver is the address on the final destination chain
	FinalReceiver string
}

// ForwardSwapParams contains parameters for forward-then-swap (case 5.2).
// Used when: Source -> Intermediate(s) (forward) -> Broker (swap) -> Destination
// Supports multiple inbound hops: Source -> Int1 -> Int2 -> Broker (swap) -> Dest
type ForwardSwapParams struct {
	// InboundHops contains the forwarding hops to reach the broker.
	// For single hop: [hop_to_broker]
	// For multi-hop: [hop_to_int1, hop_to_broker]
	InboundHops []IBCHop
	// SwapParams contains the swap parameters (executed after forwarding to broker)
	SwapParams SwapAndForwardParams
}

// ForwardSwapForwardParams contains parameters for forward + swap + forward (case 5.4).
// Used when: Source -> Intermediate(s) (forward) -> Broker (swap) -> Intermediate(s) -> Destination
type ForwardSwapForwardParams struct {
	// InboundHops contains the forwarding hops to reach the broker.
	InboundHops []IBCHop
	// SwapParams contains the swap parameters
	SwapParams SwapAndMultiHopParams
}

// HopAndSwapParams contains parameters for a hop and swap.
// Used when: Source -> Intermediate(s) (forward) -> Broker (swap) -> Destination(broker chain)
type HopAndSwapParams struct {
	// InboundHops contains the forwarding hops to reach the broker.
	// For single hop: [hop_to_broker]
	// For multi-hop: [hop_to_int1, hop_to_broker]
	InboundHops []IBCHop
	// SwapParams contains the swap parameters (executed after forwarding to broker)
	SwapParams SwapAndForwardParams
}

// IBCHop represents a single IBC transfer hop
type IBCHop struct {
	// Channel is the IBC channel ID
	Channel string
	// Port is the IBC port (typically "transfer")
	Port string
	// Receiver is the address on the destination chain for this hop. Use the address
	// converter to derive the correct bech32 address for intermediate chains; the
	// final hop uses the user's destination address.
	Receiver string
	// Timeout in nanoseconds
	Timeout int64
}

// buildNestedForwardMemo builds a nested PFM forward structure for multi-hop forwarding.
// The hops slice should contain all hops except the first one (which is handled separately).
// Each hop's Receiver should be the address on that hop's destination chain (callers
// typically set this via the address converter); finalReceiver is used as fallback for
// the last hop if Receiver is empty.
func BuildNestedForwardMemo(hops []IBCHop, finalReceiver string) *PFMForward {
	if len(hops) == 0 {
		return nil
	}

	// Build from the last hop backwards
	var current *PFMForward

	for i := len(hops) - 1; i >= 0; i-- {
		hop := hops[i]
		// Use the receiver for this hop. Callers (e.g. pathfinder) set hop.Receiver
		// using the address converter so each hop has the correct bech32 address
		// for the destination chain; last hop has finalReceiver, intermediates
		// have the derived address for that chain.
		receiver := hop.Receiver
		if receiver == "" && i == len(hops)-1 {
			receiver = finalReceiver
		}

		if current == nil {
			// Last hop - no next
			current = NewNestedForward(
				hop.Channel,
				hop.Port,
				receiver,
				DefaultRetries(),
				hop.Timeout,
				nil,
			)
		} else {
			// Intermediate hop - chain to previous
			current = NewNestedForward(
				hop.Channel,
				hop.Port,
				receiver,
				DefaultRetries(),
				hop.Timeout,
				NewPFMNextWithForward(current),
			)
		}
	}

	return current
}

// BuildSimpleForwardMemo creates a simple PFM forward memo (case 1 from doc.go).
// This is a convenience method for non-swap forwarding.
func BuildSimpleForwardMemo(channel, port, receiver string, timeout int64) (string, error) {
	memo := NewForwardMemo(channel, port, receiver, DefaultRetries(), timeout)
	return memo.ToJSON()
}
