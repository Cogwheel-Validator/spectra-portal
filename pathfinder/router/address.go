package router

import (
	"fmt"

	"github.com/btcsuite/btcutil/bech32"
)

// AddressConverter handles bech32 address conversions between different chains
type AddressConverter struct {
	// chainPrefixes maps chain IDs to their bech32 prefixes
	chainPrefixes map[string]string
	// chainSlip44 maps chain IDs to their SLIP-44 coin type
	chainSlip44 map[string]int
	// prefixSlip44 maps a bech32 prefix to its SLIP-44 coin type. It is used to
	// determine the SLIP-44 of a source address from its prefix alone, since the
	// conversion methods only receive the address and the target chain.
	prefixSlip44 map[string]int
}

// NewAddressConverter creates a new address converter with the given chain prefix mappings
func NewAddressConverter(chains []PathfinderChain) *AddressConverter {
	prefixes := make(map[string]string)
	chainSlip44 := make(map[string]int)
	prefixSlip44 := make(map[string]int)
	for _, chain := range chains {
		if chain.Bech32Prefix == "" {
			continue
		}
		prefixes[chain.Id] = chain.Bech32Prefix
		chainSlip44[chain.Id] = chain.Slip44
		prefixSlip44[chain.Bech32Prefix] = chain.Slip44
	}
	return &AddressConverter{
		chainPrefixes: prefixes,
		chainSlip44:   chainSlip44,
		prefixSlip44:  prefixSlip44,
	}
}

// ConvertAddress converts an address from one bech32 prefix to another.
// This is useful for deriving the same account's address on different chains.
//
// Converting an address by swapping its bech32 prefix only yields the correct
// account on the target chain when both chains share the same SLIP-44 coin type.
// Chains with different SLIP-44 types derive keys differently, so the re-encoded
// bytes would point at a different (wrong) account. When the source and target
// chains use different SLIP-44 coin types, this returns an error instead of a
// silently incorrect address.
//
// This is a know weakness of this implementation. This will only be used for v1
// FindPath methods. In the version 2 the pathfinder will require strict requirement
// to insert addresses directly so there will be no need for this conversion.
func (c *AddressConverter) ConvertAddress(address string, targetChainID string) (string, error) {
	targetPrefix, ok := c.chainPrefixes[targetChainID]
	if !ok {
		return "", fmt.Errorf("unknown chain ID: %s", targetChainID)
	}

	// Decode the source address up front so we can inspect its prefix and reuse
	// the decoded data for the re-encode.
	sourcePrefix, data, err := bech32.Decode(address)
	if err != nil {
		return "", fmt.Errorf("failed to decode address: %w", err)
	}

	if err := c.ensureSlip44Match(sourcePrefix, targetChainID); err != nil {
		return "", err
	}

	converted, err := bech32.Encode(targetPrefix, data)
	if err != nil {
		return "", fmt.Errorf("failed to encode address: %w", err)
	}

	return converted, nil
}

// ensureSlip44Match guards against converting an address between chains that use
// different SLIP-44 coin types, which would produce an address for the wrong
// account. It returns an error when either SLIP-44 type is unknown or when they
// differ.
func (c *AddressConverter) ensureSlip44Match(sourcePrefix, targetChainID string) error {
	sourceSlip44, ok := c.prefixSlip44[sourcePrefix]
	if !ok {
		return fmt.Errorf(
			"cannot determine SLIP-44 coin type for source address prefix %q; refusing conversion to avoid deriving a wrong address",
			sourcePrefix,
		)
	}

	targetSlip44, ok := c.chainSlip44[targetChainID]
	if !ok {
		return fmt.Errorf(
			"cannot determine SLIP-44 coin type for target chain %q; refusing conversion to avoid deriving a wrong address",
			targetChainID,
		)
	}

	if sourceSlip44 != targetSlip44 {
		return fmt.Errorf(
			"cannot convert address: source prefix %q (SLIP-44 %d) and target chain %q (SLIP-44 %d) use different coin types; the converted address would belong to a different account",
			sourcePrefix, sourceSlip44, targetChainID, targetSlip44,
		)
	}

	return nil
}

// ConvertBech32Address converts a bech32 address to a new prefix.
//
// NOTE: this is a low-level helper that performs no SLIP-44 compatibility check.
// Prefer AddressConverter.ConvertAddress, which guards against converting between
// chains with different SLIP-44 coin types. Use this only when the prefixes are
// known to belong to chains sharing the same SLIP-44 type.
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

// SetPrefix sets or updates the bech32 prefix and SLIP-44 coin type for a chain.
func (c *AddressConverter) SetPrefix(chainID, prefix string, slip44 int) {
	c.chainPrefixes[chainID] = prefix
	c.chainSlip44[chainID] = slip44
	c.prefixSlip44[prefix] = slip44
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
