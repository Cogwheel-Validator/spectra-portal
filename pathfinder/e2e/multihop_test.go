//go:build e2e

package e2e

import (
	"context"
	"sort"
	"testing"

	"connectrpc.com/connect"
	v2beta "github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/services/pathfinder/v2beta"
	"github.com/zeebo/assert"
)

// TestMultiHop verifies that the pathfinder correctly handles multihop routes,
// including PFM memo generation when supported.
func TestMultiHop(t *testing.T) {
	client := NewFindPathClient(t)

	for _, tc := range MultiHopCases {
		name := tc.Name
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()

			discover, err := client.FindPath(ctx, connect.NewRequest(&v2beta.FindPathRequest{
				ChainFrom:      tc.ChainFrom,
				TokenFromDenom: tc.TokenFromDenom,
				AmountIn:       tc.AmountIn,
				ChainTo:        tc.ChainTo,
				TokenToDenom:   tc.TokenToDenom,
				SmartRoute:     true,
			}))
			transfer := discover.Msg.GetIndirect()
			if transfer == nil {
				t.Fatalf("transfer is nil; expected an indirect route but got direct=%v brokerSwap=%v",
					discover.Msg.GetDirect(), discover.Msg.GetBrokerSwap())
			}
			requiredChains := discover.Msg.GetRequiredChains()
			expectedChains := getReqChains(transfer.Legs)
			assert.Equal(t, requiredChains, expectedChains)

			assert.NoError(t, err)
			assert.True(t, discover.Msg.GetSuccess())
			assert.Equal(t, discover.Msg.GetResponseCode(), v2beta.ResponseCode_RESPONSE_CODE_MOCK_ADDRESSES)
			assert.NotNil(t, transfer)
			assert.Equal(t, transfer.PfmMemo, "")

			addresses := addressesFor(t, discover.Msg.GetRequiredChains())

			real, err := client.FindPath(ctx, connect.NewRequest(&v2beta.FindPathRequest{
				ChainFrom:      tc.ChainFrom,
				TokenFromDenom: tc.TokenFromDenom,
				AmountIn:       tc.AmountIn,
				ChainTo:        tc.ChainTo,
				TokenToDenom:   tc.TokenToDenom,
				Addresses:      addresses,
				SmartRoute:     true,
			}))

			indirect := real.Msg.GetIndirect()
			if indirect == nil {
				t.Fatalf("indirect is nil; expected an indirect route but got direct=%v brokerSwap=%v",
					real.Msg.GetDirect(), real.Msg.GetBrokerSwap())
			}

			assert.NoError(t, err)
			assert.True(t, real.Msg.GetSuccess())
			assert.Equal(t, real.Msg.GetResponseCode(), v2beta.ResponseCode_RESPONSE_CODE_OK)
			assert.NotNil(t, indirect)
			assert.Equal(t, len(indirect.Legs), len(requiredChains)-1)
			if indirect.SupportsPfm {
				assert.NotEqual(t, indirect.PfmMemo, "")
			}
		})
	}
}

// getReqChains derives the set of chains a route's legs touch, matching how
// the server's chainsFromNeeds builds RequiredChains.
func getReqChains(legs []*v2beta.IBCLeg) []string {
	seen := make(map[string]bool)
	chains := make([]string, 0, len(legs)+1)
	add := func(chain string) {
		if !seen[chain] {
			seen[chain] = true
			chains = append(chains, chain)
		}
	}
	for _, leg := range legs {
		add(leg.FromChain)
		add(leg.ToChain)
	}
	sort.Strings(chains)
	return chains
}
