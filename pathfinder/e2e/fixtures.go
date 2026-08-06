//go:build e2e

package e2e

import "testing"

// TestAddresses maps chain IDs to a real, funds-empty wallet address on that
// chain. FindPath never checks balances or broadcasts anything, so these
// only need to be real, well-formed bech32 addresses for the chain's
// prefix — the server validates address-prefix/chain match, so a made-up
// string won't pass. They don't need to be addresses you control funds on.
var TestAddresses = loadAddresses(&testing.T{})

// All test cases are loaded from the test_cases.toml file.
var SwapCases, MultiHopCases, MultiHopSwapCases = loadTestCases(&testing.T{})
var PathfinderBasicCases = loadBasicRoutes(&testing.T{})
