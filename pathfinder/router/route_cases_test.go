package router_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/btcsuite/btcutil/bech32"
	models "github.com/Cogwheel-Validator/spectra-portal/pathfinder/models"
	router "github.com/Cogwheel-Validator/spectra-portal/pathfinder/router"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/brokers"
	"github.com/zeebo/assert"
)

// testAddr returns a valid bech32 address with the given prefix.
// The data part is fixed so the same account is derived on every chain,
// which mirrors how the address converter re-encodes addresses.
func testAddr(t testing.TB, prefix string) string {
	t.Helper()
	addr, err := bech32.Encode(prefix, make([]byte, 32))
	if err != nil {
		t.Fatalf("failed to encode bech32 address: %v", err)
	}
	return addr
}

func boolPtr(b bool) *bool       { return &b }
func uint32Ptr(v uint32) *uint32 { return &v }

// Case 1 from route_multihop.go: source == broker == destination, just a swap.
func TestPathfinder_SameChainSwap(t *testing.T) {
	pathfinder, _ := setupTestPathfinder()

	req := models.RouteRequest{
		ChainFrom:       "osmosis-1",
		ChainTo:         "osmosis-1",
		TokenFromDenom:  "uosmo",
		TokenToDenom:    "ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2",
		AmountIn:        "1000000",
		SenderAddress:   testAddr(t, "osmo"),
		ReceiverAddress: testAddr(t, "osmo"),
		SmartRoute:      boolPtr(true),
		SlippageBps:     uint32Ptr(100),
	}

	response := pathfinder.FindPath(req)

	assert.True(t, response.Success)
	assert.Equal(t, response.RouteType, "broker_swap")
	assert.NotNil(t, response.BrokerSwap)

	brokerRoute := response.BrokerSwap
	assert.Equal(t, len(brokerRoute.Path), 1)
	assert.Equal(t, brokerRoute.Path[0], "osmosis-1")
	assert.Equal(t, len(brokerRoute.InboundLegs), 0)
	assert.Equal(t, len(brokerRoute.OutboundLegs), 0)

	assert.NotNil(t, brokerRoute.Swap)
	assert.Equal(t, brokerRoute.Swap.TokenIn.ChainDenom, "uosmo")
	assert.True(t, brokerRoute.Swap.TokenIn.IsNative)
	assert.Equal(t, brokerRoute.Swap.TokenOut.OriginChain, "cosmoshub-4")
	assert.False(t, brokerRoute.Swap.TokenOut.IsNative)

	// Same-chain swap with SmartRoute must return smart contract data, not an IBC memo
	assert.NotNil(t, brokerRoute.Execution)
	assert.Nil(t, brokerRoute.Execution.Memo)
	assert.NotNil(t, brokerRoute.Execution.SmartContractData)
	assert.True(t, brokerRoute.Execution.UsesWasm)
	// 980000 * (10000 - 100) / 10000
	assert.Equal(t, brokerRoute.Execution.MinOutputAmount, "970200")
}

// Case 2: destination is the broker chain (inbound IBC + swap, no outbound leg).
func TestPathfinder_SwapOnlyRoute(t *testing.T) {
	pathfinder, _ := setupTestPathfinder()

	req := models.RouteRequest{
		ChainFrom:       "cosmoshub-4",
		ChainTo:         "osmosis-1",
		TokenFromDenom:  "uatom",
		TokenToDenom:    "uosmo",
		AmountIn:        "1000000",
		SenderAddress:   testAddr(t, "cosmos"),
		ReceiverAddress: testAddr(t, "osmo"),
		SmartRoute:      boolPtr(true),
		SlippageBps:     uint32Ptr(100),
	}

	response := pathfinder.FindPath(req)

	assert.True(t, response.Success)
	assert.Equal(t, response.RouteType, "broker_swap")
	brokerRoute := response.BrokerSwap

	assert.Equal(t, len(brokerRoute.Path), 2)
	assert.Equal(t, brokerRoute.Path[0], "cosmoshub-4")
	assert.Equal(t, brokerRoute.Path[1], "osmosis-1")

	assert.Equal(t, len(brokerRoute.InboundLegs), 1)
	assert.Equal(t, brokerRoute.InboundLegs[0].Channel, "channel-0")
	assert.Equal(t, len(brokerRoute.OutboundLegs), 0)

	// The swap input on the broker must be the IBC denom of ATOM on Osmosis
	assert.Equal(t, brokerRoute.Swap.TokenIn.ChainDenom,
		"ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2")
	assert.Equal(t, brokerRoute.Swap.TokenOut.ChainDenom, "uosmo")

	// SmartRoute execution: IBC memo targeting the ibc-hooks contract
	assert.NotNil(t, brokerRoute.Execution)
	assert.NotNil(t, brokerRoute.Execution.Memo)
	assert.NotNil(t, brokerRoute.Execution.IBCReceiver)
	assert.Equal(t, *brokerRoute.Execution.IBCReceiver,
		"osmo10a3k4hvk37cc4hnxctw4p95fhscd2z6h2rmx0aukc6rm8u9qqx9smfsh7u")
	assert.NotNil(t, brokerRoute.Execution.RecoverAddress)
	// Recover address is the sender re-derived on the broker chain
	assert.Equal(t, *brokerRoute.Execution.RecoverAddress, testAddr(t, "osmo"))
	assert.Equal(t, brokerRoute.Execution.MinOutputAmount, "970200")
}

// Case 3: source is the broker chain (swap + outbound IBC).
func TestPathfinder_BrokerAsSourceRoute(t *testing.T) {
	pathfinder, _ := setupTestPathfinder()

	req := models.RouteRequest{
		ChainFrom:       "osmosis-1",
		ChainTo:         "cosmoshub-4",
		TokenFromDenom:  "uosmo",
		TokenToDenom:    "uatom",
		AmountIn:        "1000000",
		SenderAddress:   testAddr(t, "osmo"),
		ReceiverAddress: testAddr(t, "cosmos"),
		SmartRoute:      boolPtr(true),
		SlippageBps:     uint32Ptr(100),
	}

	response := pathfinder.FindPath(req)

	assert.True(t, response.Success)
	assert.Equal(t, response.RouteType, "broker_swap")
	brokerRoute := response.BrokerSwap

	assert.Equal(t, len(brokerRoute.Path), 2)
	assert.Equal(t, brokerRoute.Path[0], "osmosis-1")
	assert.Equal(t, brokerRoute.Path[1], "cosmoshub-4")
	assert.Equal(t, len(brokerRoute.InboundLegs), 0)

	assert.Equal(t, len(brokerRoute.OutboundLegs), 1)
	assert.Equal(t, brokerRoute.OutboundLegs[0].FromChain, "osmosis-1")
	assert.Equal(t, brokerRoute.OutboundLegs[0].ToChain, "cosmoshub-4")
	assert.Equal(t, brokerRoute.OutboundLegs[0].Amount, "980000")

	// The swap output on the broker is ATOM's IBC denom on Osmosis (unwinds to uatom on the hub)
	assert.Equal(t, brokerRoute.Swap.TokenOut.ChainDenom,
		"ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2")

	// SmartRoute execution from the broker chain: smart contract data, no memo
	assert.NotNil(t, brokerRoute.Execution)
	assert.Nil(t, brokerRoute.Execution.Memo)
	assert.NotNil(t, brokerRoute.Execution.SmartContractData)
}

// Case 4 with unwinding: the swap output originates from a chain that is neither the
// broker nor the destination, so the outbound path is broker -> origin -> destination.
func TestPathfinder_FourChainOutboundRoute(t *testing.T) {
	pathfinder, _ := setupTestPathfinder()

	// ATOM on Cosmos Hub -> USDC on Juno. USDC is native to Noble, so the swap
	// output must unwind through Noble: hub -> osmosis (swap) -> noble -> juno.
	req := models.RouteRequest{
		ChainFrom:       "cosmoshub-4",
		ChainTo:         "juno-1",
		TokenFromDenom:  "uatom",
		TokenToDenom:    "ibc/EAC38D55372F38F1AFD68DF7FE9EF762DCF69F26520643CF3F9D292A738D8034",
		AmountIn:        "1000000",
		SenderAddress:   testAddr(t, "cosmos"),
		ReceiverAddress: testAddr(t, "juno"),
		SmartRoute:      boolPtr(true),
		SlippageBps:     uint32Ptr(100),
	}

	response := pathfinder.FindPath(req)

	assert.True(t, response.Success)
	assert.Equal(t, response.RouteType, "broker_swap")
	brokerRoute := response.BrokerSwap

	assert.Equal(t, len(brokerRoute.OutboundLegs), 2)
	leg0 := brokerRoute.OutboundLegs[0]
	assert.Equal(t, leg0.FromChain, "osmosis-1")
	assert.Equal(t, leg0.ToChain, "noble-1")
	assert.Equal(t, leg0.Channel, "channel-3")
	// The token leaving the broker is USDC's IBC denom on Osmosis
	assert.Equal(t, leg0.Token.ChainDenom,
		"ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4")

	leg1 := brokerRoute.OutboundLegs[1]
	assert.Equal(t, leg1.FromChain, "noble-1")
	assert.Equal(t, leg1.ToChain, "juno-1")
	assert.Equal(t, leg1.Channel, "channel-1")
	// On Noble the token unwinds to native uusdc
	assert.Equal(t, leg1.Token.ChainDenom, "uusdc")
	assert.True(t, leg1.Token.IsNative)

	// Amount is carried through the pure IBC transfers unchanged after the swap
	assert.Equal(t, leg0.Amount, "980000")
	assert.Equal(t, leg1.Amount, "980000")

	// Noble (the only intermediate outbound chain) supports PFM
	assert.True(t, brokerRoute.OutboundSupportsPFM)

	// Multi-hop outbound with SmartRoute produces an IBC memo
	assert.NotNil(t, brokerRoute.Execution)
	assert.NotNil(t, brokerRoute.Execution.Memo)
	assert.True(t, brokerRoute.Execution.UsesWasm)
}

// SmartRoute=false (manual route) must not generate execution data.
func TestPathfinder_ManualRouteSkipsExecutionData(t *testing.T) {
	pathfinder, _ := setupTestPathfinder()

	req := models.RouteRequest{
		ChainFrom:       "cosmoshub-4",
		ChainTo:         "juno-1",
		TokenFromDenom:  "uatom",
		TokenToDenom:    "ujuno",
		AmountIn:        "1000000",
		SenderAddress:   testAddr(t, "cosmos"),
		ReceiverAddress: testAddr(t, "juno"),
		SmartRoute:      boolPtr(false),
		SlippageBps:     uint32Ptr(100),
	}

	response := pathfinder.FindPath(req)

	assert.True(t, response.Success)
	assert.Equal(t, response.RouteType, "broker_swap")
	assert.Nil(t, response.BrokerSwap.Execution)
}

// Slippage above 100% (10000 bps) must be rejected and the route reported impossible.
func TestPathfinder_InvalidSlippageRejected(t *testing.T) {
	pathfinder, _ := setupTestPathfinder()

	req := models.RouteRequest{
		ChainFrom:       "cosmoshub-4",
		ChainTo:         "juno-1",
		TokenFromDenom:  "uatom",
		TokenToDenom:    "ujuno",
		AmountIn:        "1000000",
		SenderAddress:   testAddr(t, "cosmos"),
		ReceiverAddress: testAddr(t, "juno"),
		SlippageBps:     uint32Ptr(10001),
	}

	response := pathfinder.FindPath(req)

	assert.False(t, response.Success)
	assert.Equal(t, response.RouteType, "impossible")
	if !strings.Contains(response.ErrorMessage, "slippage") {
		t.Errorf("expected slippage error, got: %s", response.ErrorMessage)
	}
}

// A broker route candidate without a configured broker client must fail cleanly.
func TestPathfinder_MissingBrokerClient(t *testing.T) {
	routeIndex := router.NewRouteIndex()
	err := routeIndex.BuildIndex(chains)
	assert.NoError(t, err)

	// No broker clients configured at all
	pathfinder := router.NewPathfinder(chains, routeIndex, map[string]brokers.BrokerClient{})

	req := models.RouteRequest{
		ChainFrom:       "cosmoshub-4",
		ChainTo:         "juno-1",
		TokenFromDenom:  "uatom",
		TokenToDenom:    "ujuno",
		AmountIn:        "1000000",
		SenderAddress:   testAddr(t, "cosmos"),
		ReceiverAddress: testAddr(t, "juno"),
	}

	response := pathfinder.FindPath(req)

	assert.False(t, response.Success)
	assert.Equal(t, response.RouteType, "impossible")
	if !strings.Contains(response.ErrorMessage, "no client configured for broker osmosis-sqs") {
		t.Errorf("expected missing-client error, got: %s", response.ErrorMessage)
	}
}

// Direct route must be rejected when the requested output denom does not match
// what the token becomes on the destination chain.
func TestPathfinder_DirectRouteDenomMismatch(t *testing.T) {
	_, routeIndex := setupTestPathfinder()

	req := models.RouteRequest{
		ChainFrom:      "cosmoshub-4",
		ChainTo:        "osmosis-1",
		TokenFromDenom: "uatom",
		// Wrong output denom: uatom does not become uosmo on Osmosis
		TokenToDenom: "uosmo",
		AmountIn:     "1000000",
	}

	assert.Nil(t, routeIndex.FindDirectRoute(req))
}

// The indirect BFS must not bridge two different tokens even if both exist on both chains.
func TestPathfinder_IndirectRouteRejectsDifferentTokens(t *testing.T) {
	_, routeIndex := setupTestPathfinder()

	req := models.RouteRequest{
		ChainFrom:      "cosmoshub-4",
		ChainTo:        "osmosis-1",
		TokenFromDenom: "uatom",
		TokenToDenom:   "uosmo",
	}

	assert.Nil(t, routeIndex.FindIndirectRoute(req))
}

// Multi-hop route without PFM support on the intermediate chain: the route is still
// returned, but SupportsPFM must be false and no PFM memo may be generated.
func TestPathfinder_IndirectRouteWithoutPFM(t *testing.T) {
	// Small dedicated topology: token native to chain-a travels a -> b -> c,
	// but chain-b (the forwarding chain) has no PFM.
	noPfmChains := []router.PathfinderChain{
		{
			Name: "Chain A", Id: "chain-a", HasPFM: true, Bech32Prefix: "aaa",
			Routes: []router.BasicRoute{
				{
					ToChain: "chainb", ToChainId: "chain-b", ChannelId: "channel-1", PortId: "transfer",
					AllowedTokens: map[string]router.TokenInfo{
						"utok": {ChainDenom: "utok", IbcDenom: "ibc/utok-on-b", BaseDenom: "utok", OriginChain: "chain-a"},
					},
				},
			},
		},
		{
			Name: "Chain B", Id: "chain-b", HasPFM: false, Bech32Prefix: "bbb",
			Routes: []router.BasicRoute{
				{
					ToChain: "chainc", ToChainId: "chain-c", ChannelId: "channel-2", PortId: "transfer",
					AllowedTokens: map[string]router.TokenInfo{
						"ibc/utok-on-b": {ChainDenom: "ibc/utok-on-b", IbcDenom: "ibc/utok-on-c", BaseDenom: "utok", OriginChain: "chain-a"},
					},
				},
			},
		},
		{
			Name: "Chain C", Id: "chain-c", HasPFM: true, Bech32Prefix: "ccc",
			Routes: []router.BasicRoute{
				{
					ToChain: "chainb", ToChainId: "chain-b", ChannelId: "channel-2", PortId: "transfer",
					AllowedTokens: map[string]router.TokenInfo{
						"ibc/utok-on-c": {ChainDenom: "ibc/utok-on-c", IbcDenom: "ibc/utok-on-b", BaseDenom: "utok", OriginChain: "chain-a"},
					},
				},
			},
		},
	}

	routeIndex := router.NewRouteIndex()
	assert.NoError(t, routeIndex.BuildIndex(noPfmChains))
	pathfinder := router.NewPathfinder(noPfmChains, routeIndex, map[string]brokers.BrokerClient{})

	req := models.RouteRequest{
		ChainFrom:       "chain-a",
		ChainTo:         "chain-c",
		TokenFromDenom:  "utok",
		TokenToDenom:    "ibc/utok-on-c",
		AmountIn:        "1000000",
		SenderAddress:   testAddr(t, "aaa"),
		ReceiverAddress: testAddr(t, "ccc"),
	}

	response := pathfinder.FindPath(req)

	assert.True(t, response.Success)
	assert.Equal(t, response.RouteType, "indirect")
	assert.Equal(t, len(response.Indirect.Legs), 2)
	assert.False(t, response.Indirect.SupportsPFM)
	assert.Equal(t, response.Indirect.PFMMemo, "")
}

// The PFM memo produced for indirect routes must be valid JSON with the expected
// forward structure for the second leg.
func TestPathfinder_IndirectRoutePFMMemoStructure(t *testing.T) {
	pathfinder, _ := setupTestPathfinder()

	receiver := testAddr(t, "osmo")
	req := models.RouteRequest{
		ChainFrom:       "juno-1",
		ChainTo:         "osmosis-1",
		TokenFromDenom:  "ibc/EAC38D55372F38F1AFD68DF7FE9EF762DCF69F26520643CF3F9D292A738D8034",
		TokenToDenom:    "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4",
		AmountIn:        "5000000",
		SenderAddress:   testAddr(t, "juno"),
		ReceiverAddress: receiver,
	}

	response := pathfinder.FindPath(req)

	assert.True(t, response.Success)
	assert.Equal(t, response.RouteType, "indirect")
	assert.True(t, response.Indirect.SupportsPFM)

	var memo struct {
		Forward struct {
			Receiver string `json:"receiver"`
			Port     string `json:"port"`
			Channel  string `json:"channel"`
		} `json:"forward"`
	}
	assert.NoError(t, json.Unmarshal([]byte(response.Indirect.PFMMemo), &memo))
	assert.Equal(t, memo.Forward.Receiver, receiver)
	assert.Equal(t, memo.Forward.Port, "transfer")
	// Second leg: Noble -> Osmosis uses channel-0 on Noble
	assert.Equal(t, memo.Forward.Channel, "channel-0")
}

// Requests where the input token simply does not exist anywhere must be impossible.
func TestPathfinder_UnknownTokenIsImpossible(t *testing.T) {
	pathfinder, _ := setupTestPathfinder()

	req := models.RouteRequest{
		ChainFrom:       "cosmoshub-4",
		ChainTo:         "juno-1",
		TokenFromDenom:  "uatom",
		TokenToDenom:    "ufake",
		AmountIn:        "1000000",
		SenderAddress:   testAddr(t, "cosmos"),
		ReceiverAddress: testAddr(t, "juno"),
	}

	response := pathfinder.FindPath(req)

	assert.False(t, response.Success)
	assert.Equal(t, response.RouteType, "impossible")
}

// Destination chain without PFM is still reachable via a broker (Atom One in the fixture).
func TestPathfinder_BrokerRouteToNonPFMChain(t *testing.T) {
	pathfinder, _ := setupTestPathfinder()

	req := models.RouteRequest{
		ChainFrom:       "cosmoshub-4",
		ChainTo:         "atomone-1",
		TokenFromDenom:  "uatom",
		TokenToDenom:    "uphoton",
		AmountIn:        "1000000",
		SenderAddress:   testAddr(t, "cosmos"),
		ReceiverAddress: testAddr(t, "atone"),
	}

	response := pathfinder.FindPath(req)

	assert.True(t, response.Success)
	assert.Equal(t, response.RouteType, "broker_swap")
	brokerRoute := response.BrokerSwap
	assert.Equal(t, len(brokerRoute.OutboundLegs), 1)
	assert.Equal(t, brokerRoute.OutboundLegs[0].ToChain, "atomone-1")
	// Swap output on Osmosis is photon's IBC denom there
	assert.Equal(t, brokerRoute.Swap.TokenOut.ChainDenom, "ibc/uphoton-osmo")
}

// A broker client returning errors for every attempt must surface as an impossible
// route with the broker error in the message.
func TestPathfinder_BrokerQueryFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("retry backoff makes this test slow; skipping in -short mode")
	}

	routeIndex := router.NewRouteIndex()
	assert.NoError(t, routeIndex.BuildIndex(chains))

	attempts := 0
	failingBroker := &MockBrokerClient{
		brokerType:      "osmosis-sqs",
		contractAddress: "osmo10a3k4hvk37cc4hnxctw4p95fhscd2z6h2rmx0aukc6rm8u9qqx9smfsh7u",
		swapFunc: func(tokenIn, amountIn, tokenOut string, singleRoute *bool) (*brokers.SwapResult, error) {
			attempts++
			return nil, errors.New("sqs unavailable")
		},
	}
	pathfinder := router.NewPathfinder(chains, routeIndex,
		map[string]brokers.BrokerClient{"osmosis-sqs": failingBroker})

	req := models.RouteRequest{
		ChainFrom:       "cosmoshub-4",
		ChainTo:         "juno-1",
		TokenFromDenom:  "uatom",
		TokenToDenom:    "ujuno",
		AmountIn:        "1000000",
		SenderAddress:   testAddr(t, "cosmos"),
		ReceiverAddress: testAddr(t, "juno"),
	}

	response := pathfinder.FindPath(req)

	assert.False(t, response.Success)
	assert.Equal(t, response.RouteType, "impossible")
	if !strings.Contains(response.ErrorMessage, "query failed") {
		t.Errorf("expected broker query failure message, got: %s", response.ErrorMessage)
	}
	// Default retry policy: initial attempt + 3 retries
	assert.Equal(t, attempts, 4)
}
