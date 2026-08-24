package router

import (
	"fmt"
	"sort"
	"strings"

	"github.com/btcsuite/btcutil/bech32"

	models "github.com/Cogwheel-Validator/spectra-portal/pathfinder/models"
)

// AddressRole identifies which account an address on a chain belongs to.
// The same intermediate chain can need two different addresses: the sender's
// account (e.g. the PFM receiver before the broker hop) and the receiver's
// account (e.g. the PFM receivers after the swap).
type AddressRole int

const (
	// RoleSender - the account identity flows from the source-chain address.
	RoleSender AddressRole = iota
	// RoleReceiver - the account identity flows from the destination-chain address.
	RoleReceiver
)

// AddressNeed declares that a route requires an address on ChainID.
// Required only matters in derive mode (RouteRequest.DeriveMissing): a failed
// derivation for a Required need fails the resolution with a plain error
// (swallowed by the best-effort execution builder, matching v1), while a
// non-required need is simply left absent so call sites keep their legacy
// fallbacks. In strict v2 mode every need must be present in the request map.
type AddressNeed struct {
	ChainID  string
	Role     AddressRole
	Required bool
}

// MissingAddressesError reports the chains a route requires an address for
// but the request's address map did not cover. Only produced in strict v2
// mode (DeriveMissing=false, non-empty address map).
type MissingAddressesError struct {
	ChainIDs []string
}

func (e *MissingAddressesError) Error() string {
	return fmt.Sprintf(
		"route requires addresses for chains [%s]; supply them in the addresses field (send an empty addresses array to discover required chains)",
		strings.Join(e.ChainIDs, ", "),
	)
}

type addrKey struct {
	chainID string
	role    AddressRole
}

// ResolvedAddresses holds the per-(chain, role) addresses a route execution
// needs, resolved from the request's address map.
type ResolvedAddresses struct {
	addrs map[addrKey]string
	// Placeholders is true when the request carried no addresses and every
	// value is a generated zero-address placeholder. Callers must not embed
	// these into execution data or memos.
	Placeholders bool
	// NeededChains is the deduplicated, sorted list of chain IDs from the
	// needs list; it feeds RouteResponse.RequiredChains.
	NeededChains []string
}

// On returns the resolved address for the chain and role, or "" if absent.
func (r *ResolvedAddresses) On(chainID string, role AddressRole) string {
	return r.addrs[addrKey{chainID, role}]
}

// Lookup returns the resolved address for the chain and role.
func (r *ResolvedAddresses) Lookup(chainID string, role AddressRole) (string, bool) {
	addr, ok := r.addrs[addrKey{chainID, role}]
	return addr, ok
}

// placeholderAddress returns a valid bech32 zero address (20 zero bytes) for
// the chain's prefix. It is only used for mock route discovery, where the
// resulting execution data is discarded before the response is built.
func (s *Pathfinder) placeholderAddress(chainID string) (string, error) {
	chain, ok := s.chainsMap[chainID]
	if !ok {
		return "", fmt.Errorf("unknown chain ID: %s", chainID)
	}
	if chain.Bech32Prefix == "" {
		return "", fmt.Errorf("chain %s has no bech32 prefix configured", chainID)
	}
	converted, err := bech32.ConvertBits(make([]byte, 20), 8, 5, true)
	if err != nil {
		return "", fmt.Errorf("failed to convert placeholder payload: %w", err)
	}
	addr, err := bech32.Encode(chain.Bech32Prefix, converted)
	if err != nil {
		return "", fmt.Errorf("failed to encode placeholder address: %w", err)
	}
	return addr, nil
}

// chainsFromNeeds returns the deduplicated, sorted chain IDs of a needs list.
func chainsFromNeeds(needs []AddressNeed) []string {
	seen := make(map[string]bool, len(needs))
	chains := make([]string, 0, len(needs))
	for _, need := range needs {
		if !seen[need.ChainID] {
			seen[need.ChainID] = true
			chains = append(chains, need.ChainID)
		}
	}
	sort.Strings(chains)
	return chains
}

// resolveRouteAddresses resolves every address a route execution needs from
// the request's per-chain address map.
//
// Modes:
//   - Empty map: mock discovery - every need is filled with a placeholder
//     zero address and Placeholders is set.
//   - DeriveMissing (v1): missing entries are derived from the source
//     (RoleSender) or destination (RoleReceiver) address via slip-44-guarded
//     conversion, preserving the legacy behavior including its lenient
//     fallbacks for non-required needs.
//   - Strict (v2beta): every needed chain must be present in the map;
//     otherwise a *MissingAddressesError listing all missing chains is
//     returned.
func (s *Pathfinder) resolveRouteAddresses(req models.RouteRequest, needs []AddressNeed) (*ResolvedAddresses, error) {
	resolved := &ResolvedAddresses{
		addrs: make(map[addrKey]string, len(needs)),
	}

	resolved.NeededChains = chainsFromNeeds(needs)

	if len(req.Addresses) == 0 {
		resolved.Placeholders = true
		for _, need := range needs {
			addr, err := s.placeholderAddress(need.ChainID)
			if err != nil {
				return nil, err
			}
			resolved.addrs[addrKey{need.ChainID, need.Role}] = addr
		}
		return resolved, nil
	}

	var missing []string
	missingSeen := make(map[string]bool)
	for _, need := range needs {
		key := addrKey{need.ChainID, need.Role}
		if addr, ok := req.Addresses[need.ChainID]; ok {
			resolved.addrs[key] = addr
			continue
		}

		if !req.DeriveMissing {
			if !missingSeen[need.ChainID] {
				missingSeen[need.ChainID] = true
				missing = append(missing, need.ChainID)
			}
			continue
		}

		// Legacy v1 derivation: convert from the address the role's identity
		// flows from.
		baseChain := req.ChainFrom
		if need.Role == RoleReceiver {
			baseChain = req.ChainTo
		}
		base, ok := req.Addresses[baseChain]
		if !ok {
			if need.Required {
				return nil, fmt.Errorf("no address provided for chain %s to derive %s address from", baseChain, need.ChainID)
			}
			continue
		}
		addr, err := s.addressConverter.ConvertAddress(base, need.ChainID)
		if err != nil {
			if need.Required {
				return nil, fmt.Errorf("failed to derive address for chain %s: %w", need.ChainID, err)
			}
			// Lenient legacy fallback: leave the entry absent so the call
			// site applies its own default.
			continue
		}
		resolved.addrs[key] = addr
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, &MissingAddressesError{ChainIDs: missing}
	}

	return resolved, nil
}
