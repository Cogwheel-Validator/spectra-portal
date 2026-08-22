package router_test

import (
	"testing"

	models "github.com/Cogwheel-Validator/spectra-portal/pathfinder/models"
	router "github.com/Cogwheel-Validator/spectra-portal/pathfinder/router"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/brokers"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/routeindex"
	"github.com/zeebo/assert"
)

// multiHopChains models a topology where the source chain (neutron-1) has no direct
// channel to the broker (osmosis-1) and must route through cosmoshub-4:
//
//	neutron-1 -> cosmoshub-4 -> osmosis-1 (swap) [-> juno-1]
var multiHopChains = []routeindex.PathfinderChain{
	{
		Name: "Osmosis", Id: "osmosis-1", Broker: true, BrokerId: "osmosis-sqs",
		HasPFM: true, Bech32Prefix: "osmo",
		Routes: []routeindex.BasicRoute{
			{
				ToChain: "juno", ToChainId: "juno-1", ChannelId: "channel-42", PortId: "transfer",
				AllowedTokens: map[string]routeindex.TokenInfo{
					"ibc/ujuno-osmo": {
						ChainDenom: "ibc/ujuno-osmo", IbcDenom: "ujuno",
						BaseDenom: "ujuno", OriginChain: "juno-1", Decimals: 6,
					},
					"uosmo": {
						ChainDenom: "uosmo", IbcDenom: "ibc/uosmo-juno",
						BaseDenom: "uosmo", OriginChain: "osmosis-1", Decimals: 6,
					},
				},
			},
		},
	},
	{
		Name: "Cosmos Hub", Id: "cosmoshub-4", HasPFM: true, Bech32Prefix: "cosmos",
		Routes: []routeindex.BasicRoute{
			{
				ToChain: "osmosis", ToChainId: "osmosis-1", ChannelId: "channel-141", PortId: "transfer",
				AllowedTokens: map[string]routeindex.TokenInfo{
					"ibc/untrn-hub": {
						ChainDenom: "ibc/untrn-hub", IbcDenom: "ibc/untrn-osmo",
						BaseDenom: "untrn", OriginChain: "neutron-1", Decimals: 6,
					},
				},
			},
		},
	},
	{
		Name: "Neutron", Id: "neutron-1", HasPFM: true, Bech32Prefix: "neutron",
		Routes: []routeindex.BasicRoute{
			{
				ToChain: "cosmoshub", ToChainId: "cosmoshub-4", ChannelId: "channel-1", PortId: "transfer",
				AllowedTokens: map[string]routeindex.TokenInfo{
					"untrn": {
						ChainDenom: "untrn", IbcDenom: "ibc/untrn-hub",
						BaseDenom: "untrn", OriginChain: "neutron-1", Decimals: 6,
					},
				},
			},
		},
	},
	{
		Name: "Juno", Id: "juno-1", HasPFM: true, Bech32Prefix: "juno",
		Routes: []routeindex.BasicRoute{
			{
				ToChain: "osmosis", ToChainId: "osmosis-1", ChannelId: "channel-0", PortId: "transfer",
				AllowedTokens: map[string]routeindex.TokenInfo{
					"ujuno": {
						ChainDenom: "ujuno", IbcDenom: "ibc/ujuno-osmo",
						BaseDenom: "ujuno", OriginChain: "juno-1", Decimals: 6,
					},
				},
			},
		},
	},
}

// setupMultiHopPathfinder wires the multi-hop topology with a mock broker that
// records the denoms it was queried with.
func setupMultiHopPathfinder(t *testing.T) (*router.Pathfinder, *[]string) {
	t.Helper()

	routeIndex := routeindex.NewRouteIndex()
	assert.NoError(t, routeIndex.BuildIndex(multiHopChains))

	queriedTokensIn := []string{}
	brokerClients := map[string]brokers.BrokerClient{
		"osmosis-sqs": &MockBrokerClient{
			brokerType:      "osmosis-sqs",
			contractAddress: "osmo10a3k4hvk37cc4hnxctw4p95fhscd2z6h2rmx0aukc6rm8u9qqx9smfsh7u",
			swapFunc: func(tokenIn, amountIn, tokenOut string, singleRoute *bool) (*brokers.SwapResult, error) {
				queriedTokensIn = append(queriedTokensIn, tokenIn)
				return (&MockBrokerClient{}).QuerySwap(tokenIn, amountIn, tokenOut, singleRoute)
			},
		},
	}

	return router.NewPathfinder(multiHopChains, routeIndex, brokerClients), &queriedTokensIn
}

// Full broker route with a 2-hop inbound path:
// neutron-1 -> cosmoshub-4 -> osmosis-1 (swap) -> juno-1
func TestPathfinder_MultiHopInboundFullRoute(t *testing.T) {
	pathfinder, queriedTokensIn := setupMultiHopPathfinder(t)

	req := models.RouteRequest{
		ChainFrom:      "neutron-1",
		ChainTo:        "juno-1",
		TokenFromDenom: "untrn",
		TokenToDenom:   "ujuno",
		AmountIn:       "1000000",
		Addresses: map[string]string{
			"neutron-1": testAddr(t, "neutron"),
			"juno-1":    testAddr(t, "juno"),
		},
		DeriveMissing: true,
		SmartRoute:    boolPtr(true),
		SlippageBps:   uint32Ptr(100),
	}

	response := pathfinder.FindPath(req)

	assert.True(t, response.Success)
	assert.Equal(t, response.RouteType, "broker_swap")
	brokerRoute := response.BrokerSwap

	// Path includes every chain: source, intermediate, broker, destination
	assert.Equal(t, len(brokerRoute.Path), 4)
	assert.Equal(t, brokerRoute.Path[0], "neutron-1")
	assert.Equal(t, brokerRoute.Path[1], "cosmoshub-4")
	assert.Equal(t, brokerRoute.Path[2], "osmosis-1")
	assert.Equal(t, brokerRoute.Path[3], "juno-1")

	// Two inbound legs with the token transforming along the way
	assert.Equal(t, len(brokerRoute.InboundLegs), 2)
	leg0 := brokerRoute.InboundLegs[0]
	assert.Equal(t, leg0.FromChain, "neutron-1")
	assert.Equal(t, leg0.ToChain, "cosmoshub-4")
	assert.Equal(t, leg0.Channel, "channel-1")
	assert.Equal(t, leg0.Token.ChainDenom, "untrn")
	assert.True(t, leg0.Token.IsNative)

	leg1 := brokerRoute.InboundLegs[1]
	assert.Equal(t, leg1.FromChain, "cosmoshub-4")
	assert.Equal(t, leg1.ToChain, "osmosis-1")
	assert.Equal(t, leg1.Channel, "channel-141")
	assert.Equal(t, leg1.Token.ChainDenom, "ibc/untrn-hub")
	assert.False(t, leg1.Token.IsNative)

	// The broker must be queried with the denom the token has ON the broker chain,
	// i.e. after both IBC hops - not the denom on the intermediate chain.
	assert.Equal(t, len(*queriedTokensIn), 1)
	assert.Equal(t, (*queriedTokensIn)[0], "ibc/untrn-osmo")
	assert.Equal(t, brokerRoute.Swap.TokenIn.ChainDenom, "ibc/untrn-osmo")

	// Single outbound leg to juno
	assert.Equal(t, len(brokerRoute.OutboundLegs), 1)
	assert.Equal(t, brokerRoute.OutboundLegs[0].ToChain, "juno-1")
	assert.Equal(t, brokerRoute.OutboundLegs[0].Channel, "channel-42")

	// Multi-hop inbound with SmartRoute produces a forward-wrapped memo
	assert.NotNil(t, brokerRoute.Execution)
	assert.NotNil(t, brokerRoute.Execution.Memo)
}

// Swap-only route with a 2-hop inbound path (destination is the broker):
// neutron-1 -> cosmoshub-4 -> osmosis-1 (swap, stay on osmosis)
func TestPathfinder_MultiHopInboundSwapOnly(t *testing.T) {
	pathfinder, queriedTokensIn := setupMultiHopPathfinder(t)

	req := models.RouteRequest{
		ChainFrom:      "neutron-1",
		ChainTo:        "osmosis-1",
		TokenFromDenom: "untrn",
		TokenToDenom:   "uosmo",
		AmountIn:       "1000000",
		Addresses: map[string]string{
			"neutron-1": testAddr(t, "neutron"),
			"osmosis-1": testAddr(t, "osmo"),
		},
		DeriveMissing: true,
		SmartRoute:    boolPtr(true),
		SlippageBps:   uint32Ptr(100),
	}

	response := pathfinder.FindPath(req)

	assert.True(t, response.Success)
	assert.Equal(t, response.RouteType, "broker_swap")
	brokerRoute := response.BrokerSwap

	assert.Equal(t, len(brokerRoute.Path), 3)
	assert.Equal(t, brokerRoute.Path[2], "osmosis-1")
	assert.Equal(t, len(brokerRoute.InboundLegs), 2)
	assert.Equal(t, len(brokerRoute.OutboundLegs), 0)

	assert.Equal(t, (*queriedTokensIn)[0], "ibc/untrn-osmo")
	assert.Equal(t, brokerRoute.Swap.TokenOut.ChainDenom, "uosmo")

	// For a 2-hop inbound (hop-and-swap), the first IBC transfer is addressed to the
	// sender's account on the intermediate chain (PFM takes over from there),
	// not to the ibc-hooks contract on the broker.
	assert.NotNil(t, brokerRoute.Execution)
	assert.NotNil(t, brokerRoute.Execution.Memo)
	assert.NotNil(t, brokerRoute.Execution.IBCReceiver)
	assert.Equal(t, *brokerRoute.Execution.IBCReceiver, testAddr(t, "cosmos"))
}
