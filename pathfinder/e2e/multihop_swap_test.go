//go:build e2e

package e2e

import (
	"context"
	"slices"
	"testing"
	"time"

	"connectrpc.com/connect"
	v2beta "github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/services/pathfinder/v2beta"
	"github.com/zeebo/assert"
)

func TestMultiHopSwap(t *testing.T) {
	client := NewFindPathClient(t)

	for _, tc := range SwapCases {
		t.Run(tc.Name, func(t *testing.T) {
			// Do not spam because e2e test runs against real SQS.
			time.Sleep(time.Duration(5 * time.Second))
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(10*time.Second))
			defer cancel()

			discover, err := client.FindPath(ctx, connect.NewRequest(&v2beta.FindPathRequest{
				ChainFrom:      tc.ChainFrom,
				TokenFromDenom: tc.TokenFromDenom,
				AmountIn:       tc.AmountIn,
				ChainTo:        tc.ChainTo,
				TokenToDenom:   tc.TokenToDenom,
				SmartRoute:     true,
			}))
			assert.NoError(t, err)
			assert.True(t, discover.Msg.GetSuccess())
			requiredChains := discover.Msg.GetRequiredChains()
			expectedChains := getRequiredChainsMultiHop(discover.Msg.GetBrokerSwap())
			assert.Equal(t, expectedChains, requiredChains)

			broker := discover.Msg.GetBrokerSwap()
			if broker == nil {
				t.Fatal("expected a broker_swap route (route topology may no longer need a swap for this pair)")
			}
			assert.NotNil(t, broker.GetSwap())

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
			assert.NoError(t, err)
			assert.True(t, real.Msg.GetSuccess())
			assert.Equal(t, real.Msg.GetResponseCode(), v2beta.ResponseCode_RESPONSE_CODE_OK)

			realBroker := real.Msg.GetBrokerSwap()
			if realBroker == nil {
				t.Fatal("expected broker_swap route on the real-address call")
			}
			if len(realBroker.InboundLegs) < 1 && len(realBroker.OutboundLegs) < 1 {
				t.Fatal("expected to have a transaction that would include some kind of bridging")
			}
			assert.NotNil(t, realBroker.Execution)
		})
	}
}

func getRequiredChainsMultiHop(data *v2beta.BrokerSwapRoute) []string {
	chainSet := make(map[string]struct{})
	requiredChains := make([]string, 0, len(data.InboundLegs)+len(data.OutboundLegs)+1)
	for _, leg := range data.InboundLegs {
		// Just in case since this is safer
		chainSet[leg.FromChain] = struct{}{}
		chainSet[leg.ToChain] = struct{}{}
	}
	for _, leg := range data.OutboundLegs {
		chainSet[leg.FromChain] = struct{}{}
		chainSet[leg.ToChain] = struct{}{}
	}
	for chain := range chainSet {
		requiredChains = append(requiredChains, chain)
	}
	slices.Sort(requiredChains)
	return requiredChains
}
