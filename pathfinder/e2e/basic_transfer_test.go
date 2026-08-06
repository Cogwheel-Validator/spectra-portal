//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"connectrpc.com/connect"
	v2beta "github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/services/pathfinder/v2beta"
	"github.com/zeebo/assert"
)

// TestBasicRoutes tests that the pathfinder can find a route for every basic IBC transfers.
func TestBasicRoutes(t *testing.T) {
	client := NewFindPathClient(t)

	for _, tc := range PathfinderBasicCases {
		name := fmt.Sprintf("%s->%s %s", tc.FromChainId, tc.ToChainId, tc.FromDenom)
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()

			discover, err := client.FindPath(ctx, connect.NewRequest(&v2beta.FindPathRequest{
				ChainFrom:      tc.FromChainId,
				TokenFromDenom: tc.FromDenom,
				AmountIn:       "10000000", // hard code this since it doesn't make sense to have different values
				ChainTo:        tc.ToChainId,
				TokenToDenom:   "", // make it auto-fill this and assert the response holds the expected one.
				SmartRoute:     true,
			}))
			transfer := discover.Msg.GetDirect()
			if transfer == nil {
				t.Errorf("transfer is nil")
			}
			requiredChains := discover.Msg.GetRequiredChains()
			expectedChains := []string{tc.FromChainId, tc.ToChainId}

			assert.NoError(t, err)
			assert.True(t, discover.Msg.GetSuccess())
			assert.Equal(t, discover.Msg.GetResponseCode(), v2beta.ResponseCode_RESPONSE_CODE_MOCK_ADDRESSES)
			assert.NotNil(t, transfer)
			assert.Equal(t, tc.FromChainId, transfer.Transfer.FromChain)
			assert.Equal(t, tc.ToChainId, transfer.Transfer.ToChain)
			assert.Equal(t, 2, len(requiredChains))
			for _, chain := range requiredChains {
				if !slices.Contains(expectedChains, chain) {
					t.Errorf("chain %s not in the expected chains", chain)
				}
			}
			addresses := addressesFor(t, discover.Msg.GetRequiredChains())

			real, err := client.FindPath(ctx, connect.NewRequest(&v2beta.FindPathRequest{
				ChainFrom:      tc.FromChainId,
				TokenFromDenom: tc.FromDenom,
				AmountIn:       "10000000", // hard code this since it doesn't make sense to have different values
				ChainTo:        tc.ToChainId,
				TokenToDenom:   "", // make it auto-fill this and assert the response holds the expected one.
				Addresses:      addresses,
				SmartRoute:     true,
			}))
			assert.NoError(t, err)
			assert.True(t, real.Msg.GetSuccess())
			assert.Equal(t, real.Msg.GetResponseCode(), v2beta.ResponseCode_RESPONSE_CODE_OK)
			assert.NotNil(t, transfer)
			assert.Equal(t, tc.FromChainId, transfer.Transfer.FromChain)
			assert.Equal(t, tc.ToChainId, transfer.Transfer.ToChain)
		})
	}
}
