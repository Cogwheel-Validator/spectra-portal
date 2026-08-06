//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	v2beta "github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/services/pathfinder/v2beta"
	"github.com/zeebo/assert"
)

// TestBrokerSwapReachability checks that pairs known to require an Osmosis
// SQS swap leg actually route through the broker and return a live quote.
// This is a liquidity check, not just a topology check: a failure here
// likely means the pair stopped trading on Osmosis, so treat it as a signal
// to update BrokerCases rather than a routing regression by default.
func TestBrokerSwapReachability(t *testing.T) {
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
			if len(realBroker.InboundLegs) == 0 && len(realBroker.OutboundLegs) == 0 {
				t.Fatal("expected to have a transaction that would include some kind of bridging")
			}
			assert.NotNil(t, realBroker.Execution)
		})
	}
}
