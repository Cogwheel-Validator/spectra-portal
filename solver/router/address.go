package router

import (
	"fmt"

	"github.com/btcsuite/btcutil/bech32"
)

// AddressConverter handles bech32 address conversions between different chains
type AddressConverter struct {
	// chainPrefixes maps chain IDs to their bech32 prefixes
	chainPrefixes map[string]string
}

// NewAddressConverter creates a new address converter with the given chain prefix mappings
func NewAddressConverter(chains []SolverChain) *AddressConverter {
	prefixes := make(map[string]string)
	for _, chain := range chains {
		if chain.Bech32Prefix != "" {
			prefixes[chain.Id] = chain.Bech32Prefix
		}
	}
	return &AddressConverter{chainPrefixes: prefixes}
}

// ConvertAddress converts an address from one bech32 prefix to another
// This is useful for deriving the same account's address on different chains
func (c *AddressConverter) ConvertAddress(address string, targetChainID string) (string, error) {
	targetPrefix, ok := c.chainPrefixes[targetChainID]
	if !ok {
		return "", fmt.Errorf("unknown chain ID: %s", targetChainID)
	}

	return ConvertBech32Address(address, targetPrefix)
}

// ConvertBech32Address converts a bech32 address to a new prefix
func ConvertBech32Address(address string, targetPrefix string) (string, error) {
	// Decode the original address
	_, data, err := bech32.Decode(address)
	if err != nil {
		return "", fmt.Errorf("failed to decode address: %w", err)
	}

	// Encode with the new prefix
	converted, err := bech32.Encode(targetPrefix, data)
	if err != nil {
		return "", fmt.Errorf("failed to encode address: %w", err)
	}

	return converted, nil
}

// GetPrefix returns the bech32 prefix for a chain
func (c *AddressConverter) GetPrefix(chainID string) (string, bool) {
	prefix, ok := c.chainPrefixes[chainID]
	return prefix, ok
}

// SetPrefix sets or updates the bech32 prefix for a chain
func (c *AddressConverter) SetPrefix(chainID, prefix string) {
	c.chainPrefixes[chainID] = prefix
}

// DeriveAddressesForRoute derives all necessary addresses for a route
// Given a sender address, it returns addresses for each chain in the route path
type RouteAddresses struct {
	// SourceAddress on the source chain (same as input)
	SourceAddress string
	// BrokerAddress on the broker chain (derived from source)
	BrokerAddress string
	// DestinationAddress on the destination chain (provided by user)
	DestinationAddress string
}

// DeriveRouteAddresses derives addresses needed for a broker swap route
func (c *AddressConverter) DeriveRouteAddresses(
	senderAddress string,
	brokerChainID string,
	receiverAddress string,
) (*RouteAddresses, error) {
	// Derive the sender's address on the broker chain
	brokerAddress, err := c.ConvertAddress(senderAddress, brokerChainID)
	if err != nil {
		return nil, fmt.Errorf("failed to derive broker address: %w", err)
	}

	return &RouteAddresses{
		SourceAddress:      senderAddress,
		BrokerAddress:      brokerAddress,
		DestinationAddress: receiverAddress,
	}, nil
}
