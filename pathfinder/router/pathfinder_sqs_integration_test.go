package router_test

// Integration tests for the pathfinder wired to the REAL Osmosis SQS broker client
// (sqs_query + brokers/osmosis) against a mock SQS HTTP server. Unlike the unit
// tests, which stub the whole broker, these exercise the full pipeline:
//
//	FindPath -> route index -> SqsBroker.QuerySwap -> HTTP /router/quote ->
//	response parsing -> RouteData conversion -> real memo building
//
// so they catch mismatches between the route planner, the SQS wire format, and
// the ibc-hooks memos we hand to clients for signing.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	models "github.com/Cogwheel-Validator/spectra-portal/pathfinder/models"
	router "github.com/Cogwheel-Validator/spectra-portal/pathfinder/router"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/brokers"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/brokers/osmosis"
	ibcmemo "github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/ibc_memo"
	sqsquery "github.com/Cogwheel-Validator/spectra-portal/pathfinder/sqs_query"
	"github.com/zeebo/assert"
)

const integrationContract = "osmo10a3k4hvk37cc4hnxctw4p95fhscd2z6h2rmx0aukc6rm8u9qqx9smfsh7u"

// sqsRequest captures the parameters the mock SQS received on /router/quote.
type sqsRequest struct {
	TokenIn       string // raw "amount+denom" as SQS expects it
	TokenOutDenom string
	SingleRoute   string
}

// mockSQS records incoming quote requests and answers them with a fixed,
// realistic quote: one pool (id 1400) swapping the input denom into the
// requested output denom, amount out always 2500000.
type mockSQS struct {
	mu       sync.Mutex
	requests []sqsRequest
	server   *httptest.Server
}

func newMockSQS(t *testing.T) *mockSQS {
	t.Helper()
	m := &mockSQS{}

	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/router/quote" {
			http.NotFound(w, r)
			return
		}

		query := r.URL.Query()
		req := sqsRequest{
			TokenIn:       query.Get("tokenIn"),
			TokenOutDenom: query.Get("tokenOutDenom"),
			SingleRoute:   query.Get("singleRoute"),
		}
		m.mu.Lock()
		m.requests = append(m.requests, req)
		m.mu.Unlock()

		// Split "1000000ibc/..." into amount and denom
		amount := req.TokenIn
		denom := ""
		for i, c := range req.TokenIn {
			if c < '0' || c > '9' {
				amount = req.TokenIn[:i]
				denom = req.TokenIn[i:]
				break
			}
		}

		resp := sqsquery.RouteTokenResponse{
			AmountOut: "2500000",
			Route: []sqsquery.Route{
				{
					Pools: []sqsquery.Pool{
						{ID: 1400, Type: 2, SpreadFactor: "0.002", TokenOutDenom: req.TokenOutDenom, TakerFee: "0.001"},
					},
					InAmount:  amount,
					OutAmount: "2500000",
				},
			},
			EffectiveFee: "0.003",
			PriceImpact:  "0.011",
		}
		resp.AmountIn.Denom = denom
		resp.AmountIn.Amount = amount

		w.Header().Set("Content-Type", "application/json")
		assert.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	t.Cleanup(m.server.Close)

	return m
}

func (m *mockSQS) lastRequest(t *testing.T) sqsRequest {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.requests) == 0 {
		t.Fatal("mock SQS received no requests")
	}
	return m.requests[len(m.requests)-1]
}

// setupIntegrationPathfinder wires a real Pathfinder with a real SqsBroker
// pointed at the mock SQS server.
func setupIntegrationPathfinder(t *testing.T, fixture []router.PathfinderChain) (*router.Pathfinder, *mockSQS) {
	t.Helper()

	mock := newMockSQS(t)

	routeIndex := router.NewRouteIndex()
	assert.NoError(t, routeIndex.BuildIndex(fixture))

	broker := osmosis.NewSqsBroker([]string{mock.server.URL}, integrationContract)
	t.Cleanup(broker.Close)

	pathfinder := router.NewPathfinder(fixture, routeIndex,
		map[string]brokers.BrokerClient{"osmosis-sqs": broker})

	return pathfinder, mock
}

// Full route: Cosmos Hub -> Osmosis (swap) -> Juno, smart route enabled.
func TestIntegration_FullBrokerRoute(t *testing.T) {
	pathfinder, mock := setupIntegrationPathfinder(t, chains)

	sender := testAddr(t, "cosmos")
	receiver := testAddr(t, "juno")
	req := models.RouteRequest{
		ChainFrom:      "cosmoshub-4",
		ChainTo:        "juno-1",
		TokenFromDenom: "uatom",
		TokenToDenom:   "ujuno",
		AmountIn:       "1000000",
		Addresses: map[string]string{
			"cosmoshub-4": sender,
			"juno-1":      receiver,
		},
		DeriveMissing: true,
		SmartRoute:    boolPtr(true),
		SlippageBps:   uint32Ptr(150),
	}

	response := pathfinder.FindPath(req)

	assert.True(t, response.Success)
	assert.Equal(t, response.RouteType, "broker_swap")
	brokerRoute := response.BrokerSwap

	// --- What the SQS API was actually asked ---
	atomOnOsmosis := "ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2"
	junoOnOsmosis := "ibc/ujuno-osmosis"
	sqsReq := mock.lastRequest(t)
	// The quote must be requested with the denoms as they exist ON the broker chain
	assert.Equal(t, sqsReq.TokenIn, "1000000"+atomOnOsmosis)
	assert.Equal(t, sqsReq.TokenOutDenom, junoOnOsmosis)
	// SmartRoute=true is forwarded to SQS as singleRoute=true
	assert.Equal(t, sqsReq.SingleRoute, "true")

	// --- Swap quote parsed from the SQS response ---
	assert.Equal(t, brokerRoute.Swap.AmountOut, "2500000")
	assert.Equal(t, brokerRoute.Swap.EffectiveFee, "0.003")
	assert.Equal(t, brokerRoute.Swap.PriceImpact, "0.011")
	routeData, ok := brokerRoute.Swap.RouteData.(*osmosis.RouteData)
	assert.True(t, ok)
	assert.Equal(t, len(routeData.Routes), 1)

	// --- Execution data with the real memo builder ---
	assert.NotNil(t, brokerRoute.Execution)
	assert.NotNil(t, brokerRoute.Execution.Memo)
	assert.Equal(t, *brokerRoute.Execution.IBCReceiver, integrationContract)

	osmoSender := testAddr(t, "osmo")
	assert.Equal(t, *brokerRoute.Execution.RecoverAddress, osmoSender)
	// 2500000 * (10000 - 150) / 10000
	assert.Equal(t, brokerRoute.Execution.MinOutputAmount, "2462500")

	var memo ibcmemo.WasmMemo
	assert.NoError(t, json.Unmarshal([]byte(*brokerRoute.Execution.Memo), &memo))
	assert.Equal(t, memo.Wasm.Contract, integrationContract)

	action := memo.Wasm.Msg.SwapAndAction
	ops := action.UserSwap.SwapExactAssetIn.Operations
	assert.Equal(t, action.UserSwap.SwapExactAssetIn.SwapVenueName, "osmosis-poolmanager")
	assert.Equal(t, len(ops), 1)
	assert.Equal(t, ops[0].Pool, "1400")
	assert.Equal(t, ops[0].DenomIn, atomOnOsmosis)
	assert.Equal(t, ops[0].DenomOut, junoOnOsmosis)

	assert.Equal(t, action.MinAsset.Native.Denom, junoOnOsmosis)
	assert.Equal(t, action.MinAsset.Native.Amount, "2462500")

	info := action.PostSwapAction.IBCTransfer.IBCInfo
	assert.Equal(t, info.SourceChannel, "channel-1") // Osmosis -> Juno
	assert.Equal(t, info.Receiver, receiver)
	assert.Equal(t, info.RecoverAddress, osmoSender)
}

// Swap-only: Cosmos Hub -> Osmosis (swap and stay), smart route enabled.
func TestIntegration_SwapOnlyRoute(t *testing.T) {
	pathfinder, mock := setupIntegrationPathfinder(t, chains)

	receiver := testAddr(t, "osmo")
	req := models.RouteRequest{
		ChainFrom:      "cosmoshub-4",
		ChainTo:        "osmosis-1",
		TokenFromDenom: "uatom",
		TokenToDenom:   "uosmo",
		AmountIn:       "1000000",
		Addresses: map[string]string{
			"cosmoshub-4": testAddr(t, "cosmos"),
			"osmosis-1":   receiver,
		},
		DeriveMissing: true,
		SmartRoute:    boolPtr(true),
		SlippageBps:   uint32Ptr(100),
	}

	response := pathfinder.FindPath(req)

	assert.True(t, response.Success)
	assert.Equal(t, response.RouteType, "broker_swap")
	brokerRoute := response.BrokerSwap
	assert.Equal(t, len(brokerRoute.OutboundLegs), 0)

	sqsReq := mock.lastRequest(t)
	assert.Equal(t, sqsReq.TokenOutDenom, "uosmo")

	// The memo must end in a same-chain transfer to the receiver, not an IBC forward
	var memo ibcmemo.WasmMemo
	assert.NoError(t, json.Unmarshal([]byte(*brokerRoute.Execution.Memo), &memo))
	action := memo.Wasm.Msg.SwapAndAction
	assert.Nil(t, action.PostSwapAction.IBCTransfer)
	assert.NotNil(t, action.PostSwapAction.Transfer)
	assert.Equal(t, action.PostSwapAction.Transfer.ToAddress, receiver)
	// 2500000 * 99%
	assert.Equal(t, action.MinAsset.Native.Amount, "2475000")
}

// Same-chain swap on Osmosis: smart contract execution data instead of a memo.
func TestIntegration_SameChainSwap(t *testing.T) {
	pathfinder, mock := setupIntegrationPathfinder(t, chains)

	receiver := testAddr(t, "osmo")
	req := models.RouteRequest{
		ChainFrom:      "osmosis-1",
		ChainTo:        "osmosis-1",
		TokenFromDenom: "uosmo",
		TokenToDenom:   "ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2",
		AmountIn:       "1000000",
		Addresses: map[string]string{
			"osmosis-1": testAddr(t, "osmo"),
		},
		DeriveMissing: true,
		SmartRoute:    boolPtr(true),
		SlippageBps:   uint32Ptr(100),
	}

	response := pathfinder.FindPath(req)

	assert.True(t, response.Success)
	assert.Equal(t, response.RouteType, "broker_swap")

	// Source is the broker, so the swap input is the native denom directly
	sqsReq := mock.lastRequest(t)
	assert.Equal(t, sqsReq.TokenIn, "1000000uosmo")

	execution := response.BrokerSwap.Execution
	assert.NotNil(t, execution)
	assert.Nil(t, execution.Memo)
	assert.NotNil(t, execution.SmartContractData)
	assert.Equal(t, execution.SmartContractData.Wasm.Contract, integrationContract)
	transfer := execution.SmartContractData.Wasm.Msg.SwapAndAction.PostSwapAction.Transfer
	assert.NotNil(t, transfer)
	assert.Equal(t, transfer.ToAddress, receiver)
}

// 4-chain route: Cosmos Hub -> Osmosis (swap to USDC) -> Noble (unwind) -> Juno.
// The memo must forward through Noble via a nested PFM forward.
func TestIntegration_FourChainRoute(t *testing.T) {
	pathfinder, mock := setupIntegrationPathfinder(t, chains)

	receiver := testAddr(t, "juno")
	req := models.RouteRequest{
		ChainFrom:      "cosmoshub-4",
		ChainTo:        "juno-1",
		TokenFromDenom: "uatom",
		TokenToDenom:   "ibc/EAC38D55372F38F1AFD68DF7FE9EF762DCF69F26520643CF3F9D292A738D8034",
		AmountIn:       "1000000",
		Addresses: map[string]string{
			"cosmoshub-4": testAddr(t, "cosmos"),
			"juno-1":      receiver,
		},
		DeriveMissing: true,
		SmartRoute:    boolPtr(true),
		SlippageBps:   uint32Ptr(100),
	}

	response := pathfinder.FindPath(req)

	assert.True(t, response.Success)
	assert.Equal(t, response.RouteType, "broker_swap")
	brokerRoute := response.BrokerSwap
	assert.Equal(t, len(brokerRoute.OutboundLegs), 2)

	// SQS is asked to swap into USDC as it exists on Osmosis
	usdcOnOsmosis := "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4"
	assert.Equal(t, mock.lastRequest(t).TokenOutDenom, usdcOnOsmosis)

	var memo ibcmemo.WasmMemo
	assert.NoError(t, json.Unmarshal([]byte(*brokerRoute.Execution.Memo), &memo))

	info := memo.Wasm.Msg.SwapAndAction.PostSwapAction.IBCTransfer.IBCInfo
	// First outbound hop: Osmosis -> Noble
	assert.Equal(t, info.SourceChannel, "channel-3")
	assert.Equal(t, info.Receiver, testAddr(t, "noble"))

	// Second hop rides inside the ibc_info memo as a PFM forward: Noble -> Juno
	var nested ibcmemo.ForwardMemo
	assert.NoError(t, json.Unmarshal([]byte(info.Memo), &nested))
	assert.Equal(t, nested.Forward.Channel, "channel-1")
	assert.Equal(t, nested.Forward.Port, "transfer")
	assert.Equal(t, nested.Forward.Receiver, receiver)
}

// Multi-hop inbound: Neutron -> Cosmos Hub -> Osmosis (swap) -> Juno.
// The memo attached to the first transfer must describe the remaining path only
// (hub -> broker), ending in the wasm swap that forwards to Juno.
func TestIntegration_MultiHopInboundRoute(t *testing.T) {
	pathfinder, mock := setupIntegrationPathfinder(t, multiHopChains)

	receiver := testAddr(t, "juno")
	req := models.RouteRequest{
		ChainFrom:      "neutron-1",
		ChainTo:        "juno-1",
		TokenFromDenom: "untrn",
		TokenToDenom:   "ujuno",
		AmountIn:       "1000000",
		Addresses: map[string]string{
			"neutron-1": testAddr(t, "neutron"),
			"juno-1":    receiver,
		},
		DeriveMissing: true,
		SmartRoute:    boolPtr(true),
		SlippageBps:   uint32Ptr(100),
	}

	response := pathfinder.FindPath(req)

	assert.True(t, response.Success)
	assert.Equal(t, response.RouteType, "broker_swap")
	brokerRoute := response.BrokerSwap
	assert.Equal(t, len(brokerRoute.InboundLegs), 2)

	// The quote uses the denom NTRN has after landing on Osmosis
	sqsReq := mock.lastRequest(t)
	assert.Equal(t, sqsReq.TokenIn, "1000000ibc/untrn-osmo")
	assert.Equal(t, sqsReq.TokenOutDenom, "ibc/ujuno-osmo")

	var memo ibcmemo.ForwardMemo
	assert.NoError(t, json.Unmarshal([]byte(*brokerRoute.Execution.Memo), &memo))

	// The top-level forward covers the SECOND inbound hop (hub -> osmosis),
	// because the first hop is the transfer the user signs.
	assert.Equal(t, memo.Forward.Channel, "channel-141")
	assert.Equal(t, memo.Forward.Receiver, integrationContract)
	assert.NotNil(t, memo.Forward.Next)
	assert.NotNil(t, memo.Forward.Next.Wasm)

	// Inside the wasm: swap then IBC forward Osmosis -> Juno
	info := memo.Forward.Next.Wasm.Msg.SwapAndAction.PostSwapAction.IBCTransfer.IBCInfo
	assert.Equal(t, info.SourceChannel, "channel-42")
	assert.Equal(t, info.Receiver, receiver)
}

// The pathfinder must pick direct and indirect routes WITHOUT ever calling SQS.
func TestIntegration_NonBrokerRoutesSkipSQS(t *testing.T) {
	pathfinder, mock := setupIntegrationPathfinder(t, chains)

	// Direct route
	direct := pathfinder.FindPath(models.RouteRequest{
		ChainFrom:      "cosmoshub-4",
		ChainTo:        "osmosis-1",
		TokenFromDenom: "uatom",
		TokenToDenom:   "ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2",
		AmountIn:       "1000000",
		Addresses: map[string]string{
			"cosmoshub-4": testAddr(t, "cosmos"),
			"osmosis-1":   testAddr(t, "osmo"),
		},
		DeriveMissing: true,
	})
	assert.True(t, direct.Success)
	assert.Equal(t, direct.RouteType, "direct")

	// Indirect route (USDC Juno -> Noble -> Osmosis)
	indirect := pathfinder.FindPath(models.RouteRequest{
		ChainFrom:      "juno-1",
		ChainTo:        "osmosis-1",
		TokenFromDenom: "ibc/EAC38D55372F38F1AFD68DF7FE9EF762DCF69F26520643CF3F9D292A738D8034",
		TokenToDenom:   "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4",
		AmountIn:       "1000000",
		Addresses: map[string]string{
			"juno-1":    testAddr(t, "juno"),
			"osmosis-1": testAddr(t, "osmo"),
		},
		DeriveMissing: true,
	})
	assert.True(t, indirect.Success)
	assert.Equal(t, indirect.RouteType, "indirect")

	mock.mu.Lock()
	defer mock.mu.Unlock()
	assert.Equal(t, len(mock.requests), 0)
}

// Sanity check that the memo strings the integration produces round-trip as
// compact JSON (no stray whitespace or unescaped fragments) - clients embed
// them verbatim in MsgTransfer memos.
func TestIntegration_MemoIsCompactJSON(t *testing.T) {
	pathfinder, _ := setupIntegrationPathfinder(t, chains)

	response := pathfinder.FindPath(models.RouteRequest{
		ChainFrom:      "cosmoshub-4",
		ChainTo:        "juno-1",
		TokenFromDenom: "uatom",
		TokenToDenom:   "ujuno",
		AmountIn:       "1000000",
		Addresses: map[string]string{
			"cosmoshub-4": testAddr(t, "cosmos"),
			"juno-1":      testAddr(t, "juno"),
		},
		DeriveMissing: true,
		SmartRoute:    boolPtr(true),
		SlippageBps:   uint32Ptr(100),
	})

	assert.True(t, response.Success)
	memo := *response.BrokerSwap.Execution.Memo

	var buf map[string]any
	assert.NoError(t, json.Unmarshal([]byte(memo), &buf))
	compacted, err := json.Marshal(buf)
	assert.NoError(t, err)
	assert.Equal(t, len(memo), len(compacted))
	assert.False(t, strings.Contains(memo, "\n"))
}
