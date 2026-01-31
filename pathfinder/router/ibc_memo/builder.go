package ibcmemo

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
	// This is used when the inbound path requires forwarding through an intermediate chain before swap.
	BuildForwardSwapMemo(params ForwardSwapParams) (string, error)

	// BuildForwardSwapForwardMemo creates a forward memo wrapping wasm with nested forward (case 5.4)
	// This is the most complex case: multi-hop inbound + swap + multi-hop outbound.
	BuildForwardSwapForwardMemo(params ForwardSwapForwardParams) (string, error)

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
	RouteData RouteData
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
// Used when: Source -> Intermediate (forward) -> Broker (swap) -> Destination
type ForwardSwapParams struct {
	// InboundHop is the forwarding hop to the broker
	InboundHop IBCHop
	// SwapParams contains the swap parameters (executed after forwarding to broker)
	SwapParams SwapAndForwardParams
}

// ForwardSwapForwardParams contains parameters for forward + swap + forward (case 5.4).
// Used when: Source -> Intermediate (forward) -> Broker (swap) -> Intermediate -> Destination
type ForwardSwapForwardParams struct {
	// InboundHop is the forwarding hop to the broker
	InboundHop IBCHop
	// SwapParams contains the swap parameters
	SwapParams SwapAndMultiHopParams
}

// IBCHop represents a single IBC transfer hop
type IBCHop struct {
	// Channel is the IBC channel ID
	Channel string
	// Port is the IBC port (typically "transfer")
	Port string
	// Receiver is the address on the destination of this hop
	Receiver string
	// Timeout in nanoseconds
	Timeout int64
}

// RouteData is an interface for broker-specific routing data.
// Each broker implements this to provide their pool/route information.
type RouteData interface {
	// GetOperations returns the swap operations in a format the broker understands
	GetOperations() []SwapOperation
	// GetSwapVenueName returns the swap venue identifier (e.g., "osmosis-poolmanager")
	GetSwapVenueName() string
}
