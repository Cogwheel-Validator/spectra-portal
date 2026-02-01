// Package brokers defines interfaces and common types for DEX broker integrations.
// Each supported broker chain (Osmosis, Neutron, etc.) implements these interfaces.
// For now it only works for Osmosis and it all relies on Skip Go Wasm Smart Contracts.
package brokers

import (
	ibcmemo "github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/router/ibc_memo"
)

// BrokerClient is an interface for querying different DEX protocols on broker chains.
// Each broker (Osmosis, Neutron, etc.) implements this interface with their specific API.
type BrokerClient interface {
	// QuerySwap queries the broker DEX for a swap route and returns standardized swap information.
	// tokenInDenom: the denom of the input token on the broker chain (may be IBC denom)
	// tokenInAmount: the amount of input tokens
	// tokenOutDenom: the denom of the desired output token on the broker chain (may be IBC denom)
	// singleRoute: if true, only return a single route, if false, return all possible routes
	QuerySwap(tokenInDenom, tokenInAmount, tokenOutDenom string, singleRoute *bool) (*SwapResult, error)

	// GetBrokerType returns the type of broker (e.g., "osmosis-sqs", "astroport", etc.)
	GetBrokerType() string

	// GetMemoBuilder returns the memo builder for this broker (for IBC transfers with hooks)
	GetMemoBuilder() ibcmemo.MemoBuilder

	// GetSmartContractBuilder returns the smart contract builder for this broker
	// Used when source is the broker chain (direct contract execution, no IBC needed)
	GetSmartContractBuilder() SmartContractBuilder

	// Close cleans up resources used by the broker client
	Close()
}

// SmartContractBuilder builds data structures for direct smart contract execution.
// This is used when the source chain IS the broker chain, so no IBC memo is needed.
// Each broker implements this with their specific contract interface.
type SmartContractBuilder interface {
	// BuildSwapAndTransfer builds a smart contract call for swap + transfer to receiver on same chain.
	// Used when: Source == Broker == Destination (same-chain swap)
	BuildSwapAndTransfer(params ibcmemo.SwapMemoParams) (*ibcmemo.WasmMemo, error)

	// BuildSwapAndForward builds a smart contract call for swap + IBC forward.
	// Used when: Source == Broker, Destination != Broker (swap then IBC out)
	BuildSwapAndForward(params ibcmemo.SwapAndForwardParams) (*ibcmemo.WasmMemo, error)
}

// SwapResult contains standardized swap information from any broker DEX.
type SwapResult struct {
	// AmountIn is the actual amount in (after any adjustments)
	AmountIn  string
	AmountOut string
	// PriceImpact is the price impact as string (e.g., "0.02" for 2%)
	PriceImpact  string
	EffectiveFee string
	// RouteData is broker-specific route data implementing RouteData interface
	RouteData RouteData
}

// RouteData is an interface for broker-specific routing data.
type RouteData interface {
	// GetOperations returns the swap operations in a format the broker understands
	GetOperations() []ibcmemo.SwapOperation
	// GetSwapVenueName returns the swap venue identifier (e.g., "osmosis-poolmanager")
	GetSwapVenueName() string
}

// SlippageCalculator calculates minimum output with slippage tolerance.
// slippageBps is basis points (e.g., 100 = 1%)
func CalculateMinOutput(expectedOutput string, slippageBps uint32) (string, error) {
	return calculateMinOutputInternal(expectedOutput, slippageBps)
}
