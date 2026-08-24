// Package common holds the dependencies and helpers shared by every
// versioned RPC handler package.
package common

import (
	"fmt"
	"strings"

	"connectrpc.com/connect"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/denomresolver"
	"github.com/btcsuite/btcutil/bech32"
	"github.com/rs/zerolog"
)

// Deps bundles what a handler needs to serve requests.
type Deps struct {
	Pathfinder    *router.Pathfinder
	DenomResolver *denomresolver.DenomResolver
	Logger        zerolog.Logger
}

// ResolveDenoms resolves the source token denom and infers or resolves the
// destination token denom. An empty tokenTo defaults to the same token on the
// destination chain (bridging without swap). Errors are returned as
// connect.CodeInvalidArgument.
func (d Deps) ResolveDenoms(chainFrom, tokenFrom, chainTo, tokenTo string) (string, string, error) {
	resolvedFromDenom, err := d.DenomResolver.ResolveToChainDenom(chainFrom, tokenFrom)
	if err != nil {
		return "", "", connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("could not resolve source token '%s' on chain '%s': %w",
				tokenFrom, chainFrom, err))
	}

	var resolvedToDenom string
	if tokenTo == "" {
		resolvedToDenom, err = d.DenomResolver.InferTokenToDenom(chainFrom, resolvedFromDenom, chainTo)
		if err != nil {
			return "", "", connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("could not infer destination token: %w", err))
		}
	} else {
		resolvedToDenom, err = d.DenomResolver.ResolveToChainDenom(chainTo, tokenTo)
		if err != nil {
			return "", "", connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("could not resolve destination token '%s' on chain '%s': %w",
					tokenTo, chainTo, err))
		}
	}

	return resolvedFromDenom, resolvedToDenom, nil
}

// ValidateBech32Address validates that an address is a valid bech32 address
// Returns the prefix if valid, or an error if invalid
func ValidateBech32Address(address string) (string, error) {
	if address == "" {
		return "", fmt.Errorf("address is empty")
	}

	// Check minimum length (prefix + "1" + data + checksum)
	if len(address) < 10 {
		return "", fmt.Errorf("address too short (minimum 10 characters)")
	}

	// Check for separator
	sepIdx := strings.LastIndex(address, "1")
	if sepIdx < 1 {
		return "", fmt.Errorf("missing bech32 separator '1'")
	}

	// Validate the prefix (human-readable part)
	prefix := address[:sepIdx]
	if prefix == "" {
		return "", fmt.Errorf("empty bech32 prefix")
	}

	// Try to decode as bech32 - this validates the checksum
	decodedPrefix, data, err := bech32.Decode(address)
	if err != nil {
		return "", fmt.Errorf("invalid bech32 address (checksum failed): %w", err)
	}

	// Verify decoded prefix matches
	if decodedPrefix != prefix {
		return "", fmt.Errorf("bech32 prefix mismatch")
	}

	// Verify data is not empty (should be 20 or 32 bytes for cosmos addresses)
	if len(data) == 0 {
		return "", fmt.Errorf("empty address data")
	}

	return prefix, nil
}
