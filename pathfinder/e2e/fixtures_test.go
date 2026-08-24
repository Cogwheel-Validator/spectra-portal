//go:build e2e

package e2e

import (
	"fmt"
	"os"
)

// TestAddresses maps chain IDs to a real, funds-empty wallet address on that
// chain. FindPath never checks balances or broadcasts anything, so these
// only need to be real, well-formed bech32 addresses for the chain's
// prefix - the server validates address-prefix/chain match, so a made-up
// string won't pass. They don't need to be addresses you control funds on.
var TestAddresses map[string]string

// All test cases are loaded from the test_cases.toml file.
var SwapCases, MultiHopCases, MultiHopSwapCases []TestCases

var PathfinderBasicCases []BasicRoutes

func init() {
	var err error

	TestAddresses, err = loadAddresses()
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: failed to load addresses.toml: %v\n", err)
		os.Exit(1)
	}

	SwapCases, MultiHopCases, MultiHopSwapCases, err = loadTestCases()
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: failed to load test_cases.toml: %v\n", err)
		os.Exit(1)
	}

	if BaseURL() == "" {
		// No target server configured. Individual tests skip themselves via
		// RequireBaseURL, so there's nothing to query yet - leave
		// PathfinderBasicCases empty instead of failing here.
		return
	}

	PathfinderBasicCases, err = loadBasicRoutes()
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: failed to load basic routes from %s: %v\n", BaseURL(), err)
		os.Exit(1)
	}
}
