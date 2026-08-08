//go:build e2e

package e2e

import (
	"testing"

	v2beta "github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/services/pathfinder/v2beta"
)

// addressesFor resolves TestAddresses for every chain ID, skipping the
// calling test when one isn't configured yet.
func addressesFor(t *testing.T, chainIDs []string) []*v2beta.ChainAddress {
	t.Helper()
	out := make([]*v2beta.ChainAddress, 0, len(chainIDs))
	for _, id := range chainIDs {
		addr, ok := TestAddresses[id]
		if !ok || addr == "" {
			t.Skipf("no test address configured for chain %q; fill in TestAddresses in fixtures.go", id)
		}
		out = append(out, &v2beta.ChainAddress{ChainId: id, Address: addr})
	}
	return out
}
