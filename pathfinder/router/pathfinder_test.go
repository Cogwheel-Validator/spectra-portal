package router_test

import (
	"fmt"
	"testing"

	models "github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/models"
	router "github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/router"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/router/brokers"
	ibcmemo "github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/router/ibc_memo"
	"github.com/zeebo/assert"
)

var chains = []router.PathfinderChain{
	{
		Name:     "Osmosis",
		Id:       "osmosis-1",
		Broker:   true,
		BrokerId: "osmosis-sqs",
		HasPFM:   true,
		Routes: []router.BasicRoute{
			{
				ToChain:      "cosmoshub",
				ToChainId:    "cosmoshub-4",
				ConnectionId: "connection-0",
				ChannelId:    "channel-0",
				PortId:       "transfer",
				AllowedTokens: map[string]router.TokenInfo{
					"ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2": {
						ChainDenom:  "ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2",
						IbcDenom:    "uatom",
						BaseDenom:   "uatom",
						OriginChain: "cosmoshub-4",
						Decimals:    6,
					},
					"uosmo": {
						ChainDenom:  "uosmo",
						IbcDenom:    "ibc/ED07A3391A112B175915CD8FAF43A2DA8E4790EDE12566649D0C2F97716B8518",
						BaseDenom:   "uosmo",
						OriginChain: "osmosis-1",
						Decimals:    6,
					},
				},
			},
			{
				ToChain:      "juno",
				ToChainId:    "juno-1",
				ConnectionId: "connection-1",
				ChannelId:    "channel-1",
				PortId:       "transfer",
				AllowedTokens: map[string]router.TokenInfo{
					"ibc/ujuno-osmosis": {
						ChainDenom:  "ibc/ujuno-osmosis",
						IbcDenom:    "ujuno",
						BaseDenom:   "ujuno",
						OriginChain: "juno-1",
						Decimals:    6,
					},
					"uosmo": {
						ChainDenom:  "uosmo",
						IbcDenom:    "ibc/osmouosmo",
						BaseDenom:   "uosmo",
						OriginChain: "osmosis-1",
						Decimals:    6,
					},
				},
			},
			{
				ToChain:      "atomone",
				ToChainId:    "atomone-1",
				ConnectionId: "connection-2",
				ChannelId:    "channel-2",
				PortId:       "transfer",
				AllowedTokens: map[string]router.TokenInfo{
					"ibc/uatone-osmo": {
						ChainDenom:  "ibc/uatone-osmo",
						IbcDenom:    "uatone",
						BaseDenom:   "uatone",
						OriginChain: "atomone-1",
						Decimals:    6,
					},
					"ibc/uphoton-osmo": {
						ChainDenom:  "ibc/uphoton-osmo",
						IbcDenom:    "uphoton",
						BaseDenom:   "uphoton",
						OriginChain: "atomone-1",
						Decimals:    6,
					},
				},
			},
			{
				ToChain:      "noble",
				ToChainId:    "noble-1",
				ConnectionId: "connection-3",
				ChannelId:    "channel-3",
				PortId:       "transfer",
				AllowedTokens: map[string]router.TokenInfo{
					"ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4": {
						ChainDenom:  "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4",
						IbcDenom:    "uusdc", // What it becomes on Noble (native)
						BaseDenom:   "uusdc",
						OriginChain: "noble-1",
						Decimals:    6,
					},
				},
			},
		},
	},
	{
		Name:     "Cosmos Hub",
		Id:       "cosmoshub-4",
		Broker:   false,
		BrokerId: "",
		HasPFM:   true,
		Routes: []router.BasicRoute{
			{
				ToChain:      "osmosis",
				ToChainId:    "osmosis-1",
				ConnectionId: "connection-0",
				ChannelId:    "channel-0",
				PortId:       "transfer",
				AllowedTokens: map[string]router.TokenInfo{
					"uatom": {
						ChainDenom:  "uatom",
						IbcDenom:    "ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2",
						BaseDenom:   "uatom",
						OriginChain: "cosmoshub-4",
						Decimals:    6,
					},
					"ibc/ED07A3391A112B175915CD8FAF43A2DA8E4790EDE12566649D0C2F97716B8518": {
						ChainDenom:  "ibc/ED07A3391A112B175915CD8FAF43A2DA8E4790EDE12566649D0C2F97716B8518",
						IbcDenom:    "uosmo",
						BaseDenom:   "uosmo",
						OriginChain: "osmosis-1",
						Decimals:    6,
					},
					"ibc/46B44899322F3CD854D2D46DEEF881958467CDD4B3B10086DA49296BBED94BED": {
						ChainDenom: "ibc/46B44899322F3CD854D2D46DEEF881958467CDD4B3B10086DA49296BBED94BED",
						// this will probabably never happen but we need to test is the
						/// token originated from another chain and that ibc origined from another chain
						IbcDenom:    "ibc/osmosis-ujuno",
						BaseDenom:   "ujuno",
						OriginChain: "juno-1",
						Decimals:    6,
					},
				},
			},
			{
				ToChain:      "juno",
				ToChainId:    "juno-1",
				ConnectionId: "connection-1",
				ChannelId:    "channel-1",
				PortId:       "transfer",
				AllowedTokens: map[string]router.TokenInfo{
					"ibc/C4CFF46FD6DE35CA4CF4CE031E643C8FDC9BA4B99AE598E9B0ED98FE3A2319F9": {
						ChainDenom:  "ibc/C4CFF46FD6DE35CA4CF4CE031E643C8FDC9BA4B99AE598E9B0ED98FE3A2319F9",
						IbcDenom:    "ujuno",
						BaseDenom:   "ujuno",
						OriginChain: "juno-1",
						Decimals:    6,
					},
				},
			},
		},
	},
	{
		Name:     "Juno",
		Id:       "juno-1",
		Broker:   false,
		BrokerId: "",
		HasPFM:   true,
		Routes: []router.BasicRoute{
			{
				ToChain:      "cosmoshub",
				ToChainId:    "cosmoshub-4",
				ConnectionId: "connection-1",
				ChannelId:    "channel-1",
				PortId:       "transfer",
				AllowedTokens: map[string]router.TokenInfo{
					"ujuno": {
						ChainDenom:  "ujuno",
						IbcDenom:    "ibc/C4CFF46FD6DE35CA4CF4CE031E643C8FDC9BA4B99AE598E9B0ED98FE3A2319F9",
						BaseDenom:   "ujuno",
						OriginChain: "juno-1",
						Decimals:    6,
					},
				},
			},
			{
				ToChain:      "osmosis",
				ToChainId:    "osmosis-1",
				ConnectionId: "connection-0",
				ChannelId:    "channel-0",
				PortId:       "transfer",
				AllowedTokens: map[string]router.TokenInfo{
					"ujuno": {
						ChainDenom:  "ujuno",
						IbcDenom:    "ibc/ujuno-osmosis",
						BaseDenom:   "ujuno",
						OriginChain: "juno-1",
						Decimals:    6,
					},
				},
			},
			{
				ToChain:      "noble",
				ToChainId:    "noble-1",
				ConnectionId: "connection-3",
				ChannelId:    "channel-3",
				PortId:       "transfer",
				AllowedTokens: map[string]router.TokenInfo{
					"ibc/EAC38D55372F38F1AFD68DF7FE9EF762DCF69F26520643CF3F9D292A738D8034": {
						ChainDenom:  "ibc/EAC38D55372F38F1AFD68DF7FE9EF762DCF69F26520643CF3F9D292A738D8034",
						IbcDenom:    "uusdc", // What it becomes on Noble (native)
						BaseDenom:   "uusdc",
						OriginChain: "noble-1",
						Decimals:    6,
					},
				},
			},
		},
	},
	{
		Name:     "Atom One",
		Id:       "atomone-1",
		Broker:   false,
		BrokerId: "",
		HasPFM:   false,
		Routes: []router.BasicRoute{
			{
				ToChain:      "osmosis",
				ToChainId:    "osmosis-1",
				ConnectionId: "connection-0",
				ChannelId:    "channel-0",
				PortId:       "transfer",
				AllowedTokens: map[string]router.TokenInfo{
					"uatone": {
						ChainDenom:  "uatone",
						IbcDenom:    "ibc/uatone",
						BaseDenom:   "uatone",
						OriginChain: "atomone-1",
						Decimals:    6,
					},
					"uphoton": {
						ChainDenom:  "uphoton",
						IbcDenom:    "ibc/uphoton",
						BaseDenom:   "uphoton",
						OriginChain: "atomone-1",
						Decimals:    6,
					},
				},
			},
		},
	},
	{
		Name:     "Noble",
		Id:       "noble-1",
		Broker:   false,
		BrokerId: "",
		HasPFM:   true,
		Routes: []router.BasicRoute{
			{
				ToChain:      "juno",
				ToChainId:    "juno-1",
				ConnectionId: "connection-1",
				ChannelId:    "channel-1",
				PortId:       "transfer",
				AllowedTokens: map[string]router.TokenInfo{
					"uusdc": {
						ChainDenom:  "uusdc",
						IbcDenom:    "ibc/EAC38D55372F38F1AFD68DF7FE9EF762DCF69F26520643CF3F9D292A738D8034", // What it becomes on Juno
						BaseDenom:   "uusdc",
						OriginChain: "noble-1",
						Decimals:    6,
					},
				},
			},
			{
				ToChain:      "osmosis",
				ToChainId:    "osmosis-1",
				ConnectionId: "connection-0",
				ChannelId:    "channel-0",
				PortId:       "transfer",
				AllowedTokens: map[string]router.TokenInfo{
					"uusdc": {
						ChainDenom:  "uusdc",
						IbcDenom:    "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4", // What it becomes on Osmosis
						BaseDenom:   "uusdc",
						OriginChain: "noble-1",
						Decimals:    6,
					},
				},
			},
		},
	},
}

// MockRouteData implements ibcmemo.RouteData for testing
type MockRouteData struct {
	operations    []ibcmemo.SwapOperation
	swapVenueName string
}

func (m *MockRouteData) GetOperations() []ibcmemo.SwapOperation {
	return m.operations
}

func (m *MockRouteData) GetSwapVenueName() string {
	return m.swapVenueName
}

// MockMemoBuilder implements ibcmemo.MemoBuilder for testing
type MockMemoBuilder struct {
	contractAddress string
}

func (m *MockMemoBuilder) GetContractAddress() string {
	return m.contractAddress
}

func (m *MockMemoBuilder) BuildSwapMemo(params ibcmemo.SwapMemoParams) (string, error) {
	return `{"wasm":{"contract":"mock","msg":{}}}`, nil
}

func (m *MockMemoBuilder) BuildSwapAndForwardMemo(params ibcmemo.SwapAndForwardParams) (string, error) {
	return `{"wasm":{"contract":"mock","msg":{}}}`, nil
}

func (m *MockMemoBuilder) BuildSwapAndMultiHopMemo(params ibcmemo.SwapAndMultiHopParams) (string, error) {
	return `{"wasm":{"contract":"mock","msg":{}}}`, nil
}

func (m *MockMemoBuilder) BuildForwardSwapMemo(params ibcmemo.ForwardSwapParams) (string, error) {
	return `{"forward":{"next":{"wasm":{}}}}`, nil
}

func (m *MockMemoBuilder) BuildForwardSwapForwardMemo(params ibcmemo.ForwardSwapForwardParams) (string, error) {
	return `{"forward":{"next":{"wasm":{}}}}`, nil
}

func (m *MockMemoBuilder) BuildHopAndSwapMemo(params ibcmemo.HopAndSwapParams) (string, error) {
	return `{"forward":{"next":{"wasm":{"contract":"mock","msg":{}}}}}`, nil
}

// MockBrokerClient implements the brokers.BrokerClient interface for testing
type MockBrokerClient struct {
	brokerType      string
	contractAddress string
	swapFunc        func(tokenIn, amountIn, tokenOut string, singleRoute *bool) (*brokers.SwapResult, error)
}

func (m *MockBrokerClient) QuerySwap(tokenInDenom, tokenInAmount, tokenOutDenom string, singleRoute *bool) (*brokers.SwapResult, error) {
	if m.swapFunc != nil {
		return m.swapFunc(tokenInDenom, tokenInAmount, tokenOutDenom, singleRoute)
	}
	// Fake swap, but in this case we will assume 1:1 swap with 0.3% fee and
	// slipage can be anywhere from 0.1% to 0.9%, for this purpose and to ease testing
	// we will just assume it is 1% in total
	return &brokers.SwapResult{
		AmountIn:     tokenInAmount,
		AmountOut:    "990000", // Assuming 1000000 input - 1% total (0.3% fee + 0.7% slippage)
		PriceImpact:  "0.007",
		EffectiveFee: "0.003",
		RouteData: &MockRouteData{
			operations:    []ibcmemo.SwapOperation{{Pool: "1", DenomIn: tokenInDenom, DenomOut: tokenOutDenom}},
			swapVenueName: "osmosis-poolmanager",
		},
	}, nil
}

func (m *MockBrokerClient) GetBrokerType() string {
	return m.brokerType
}

func (m *MockBrokerClient) GetMemoBuilder() ibcmemo.MemoBuilder {
	return &MockMemoBuilder{contractAddress: m.contractAddress}
}

func (m *MockBrokerClient) GetSmartContractBuilder() brokers.SmartContractBuilder {
	return &MockSmartContractBuilder{contractAddress: m.contractAddress}
}

func (m *MockBrokerClient) Close() {
	// No-op for mock
}

// MockSmartContractBuilder implements brokers.SmartContractBuilder for testing
type MockSmartContractBuilder struct {
	contractAddress string
}

func (m *MockSmartContractBuilder) BuildSwapAndTransfer(params ibcmemo.SwapMemoParams) (*ibcmemo.WasmMemo, error) {
	return ibcmemo.NewWasmMemo(m.contractAddress, ibcmemo.NewWasmMsg(
		ibcmemo.NewSwapAndAction(
			ibcmemo.NewUserSwap("osmosis-poolmanager", []ibcmemo.SwapOperation{{Pool: "1", DenomIn: params.TokenInDenom, DenomOut: params.TokenOutDenom}}),
			ibcmemo.NewMinAsset(params.TokenOutDenom, params.MinOutputAmount),
			params.TimeoutTimestamp,
			ibcmemo.NewTransferAction(params.ReceiverAddress),
		),
	)), nil
}

func (m *MockSmartContractBuilder) BuildSwapAndForward(params ibcmemo.SwapAndForwardParams) (*ibcmemo.WasmMemo, error) {
	return ibcmemo.NewWasmMemo(m.contractAddress, ibcmemo.NewWasmMsg(
		ibcmemo.NewSwapAndAction(
			ibcmemo.NewUserSwap("osmosis-poolmanager", []ibcmemo.SwapOperation{{Pool: "1", DenomIn: params.TokenInDenom, DenomOut: params.TokenOutDenom}}),
			ibcmemo.NewMinAsset(params.TokenOutDenom, params.MinOutputAmount),
			params.TimeoutTimestamp,
			ibcmemo.NewIBCTransferAction(params.SourceChannel, params.ForwardReceiver, params.ForwardMemo, params.RecoverAddress),
		),
	)), nil
}

// setupTestPathfinder creates a solver with test chains and a mock broker client
func setupTestPathfinder() (*router.Pathfinder, *router.RouteIndex) {
	// Build index with test chains
	routeIndex := router.NewRouteIndex()
	err := routeIndex.BuildIndex(chains)
	if err != nil {
		panic(fmt.Sprintf("failed to build index: %v", err))
	}

	// Create mock osmosis broker client
	brokerClients := map[string]brokers.BrokerClient{
		"osmosis-sqs": &MockBrokerClient{
			brokerType:      "osmosis-sqs",
			contractAddress: "osmo10a3k4hvk37cc4hnxctw4p95fhscd2z6h2rmx0aukc6rm8u9qqx9smfsh7u",
			swapFunc: func(tokenIn, amountIn, tokenOut string, singleRoute *bool) (*brokers.SwapResult, error) {
				// Simulate realistic swap
				return &brokers.SwapResult{
					AmountIn:     amountIn,
					AmountOut:    "980000", // ~2% slippage
					PriceImpact:  "0.015",
					EffectiveFee: "0.005",
					RouteData: &MockRouteData{
						operations:    []ibcmemo.SwapOperation{{Pool: "1", DenomIn: tokenIn, DenomOut: tokenOut}},
						swapVenueName: "osmosis-poolmanager",
					},
				}, nil
			},
		},
	}

	pathfinder := router.NewPathfinder(chains, routeIndex, brokerClients)
	return pathfinder, routeIndex
}

func TestPathfinder_DirectRoute(t *testing.T) {
	pathfinder, _ := setupTestPathfinder()

	req := models.RouteRequest{
		ChainFrom:       "cosmoshub-4",
		ChainTo:         "osmosis-1",
		TokenFromDenom:  "uatom",
		TokenToDenom:    "ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2",
		AmountIn:        "1000000",
		SenderAddress:   "cosmos1sender",
		ReceiverAddress: "osmo1receiver",
	}

	response := pathfinder.FindPath(req)

	t.Logf("Response: %+v", response)
	assert.True(t, response.Success)
	assert.Equal(t, response.RouteType, "direct")
	assert.NotNil(t, response.Direct)
	assert.NotNil(t, response.Direct.Transfer)

	// Verify transfer details
	transfer := response.Direct.Transfer
	assert.Equal(t, transfer.FromChain, "cosmoshub-4")
	assert.Equal(t, transfer.ToChain, "osmosis-1")
	assert.Equal(t, transfer.Amount, "1000000")
	assert.NotNil(t, transfer.Token)
	assert.Equal(t, transfer.Token.ChainDenom, "uatom")
	assert.Equal(t, transfer.Token.BaseDenom, "uatom")
	assert.Equal(t, transfer.Token.OriginChain, "cosmoshub-4")

	// if all goes well
	t.Logf("Direct route test passed")
}

func TestPathfinder_BrokerSwapRoute(t *testing.T) {
	pathfinder, _ := setupTestPathfinder()

	req := models.RouteRequest{
		ChainFrom:       "cosmoshub-4",
		ChainTo:         "juno-1",
		TokenFromDenom:  "uatom",
		TokenToDenom:    "ujuno",
		AmountIn:        "1000000",
		SenderAddress:   "cosmos1sender",
		ReceiverAddress: "juno1receiver",
	}

	response := pathfinder.FindPath(req)

	t.Logf("Response: %+v", response)
	assert.True(t, response.Success)
	assert.Equal(t, response.RouteType, "broker_swap")
	assert.NotNil(t, response.BrokerSwap)

	brokerRoute := response.BrokerSwap
	assert.Equal(t, len(brokerRoute.Path), 3)
	assert.Equal(t, brokerRoute.Path[0], "cosmoshub-4")
	assert.Equal(t, brokerRoute.Path[1], "osmosis-1")
	assert.Equal(t, brokerRoute.Path[2], "juno-1")

	// Verify inbound legs (Cosmos -> Osmosis)
	assert.Equal(t, 1, len(brokerRoute.InboundLegs))
	assert.Equal(t, brokerRoute.InboundLegs[0].FromChain, "cosmoshub-4")
	assert.Equal(t, brokerRoute.InboundLegs[0].ToChain, "osmosis-1")
	assert.Equal(t, brokerRoute.InboundLegs[0].Amount, "1000000")

	// Verify swap details
	assert.NotNil(t, brokerRoute.Swap)
	assert.Equal(t, brokerRoute.Swap.Broker, "osmosis-sqs")
	assert.NotNil(t, brokerRoute.Swap.TokenIn)
	assert.NotNil(t, brokerRoute.Swap.TokenOut)
	assert.Equal(t, brokerRoute.Swap.AmountOut, "980000")

	// Verify outbound legs (Osmosis -> Juno)
	assert.Equal(t, len(brokerRoute.OutboundLegs), 1)
	assert.Equal(t, brokerRoute.OutboundLegs[0].FromChain, "osmosis-1")
	assert.Equal(t, brokerRoute.OutboundLegs[0].ToChain, "juno-1")
	assert.Equal(t, brokerRoute.OutboundLegs[0].Amount, "980000")

	// Verify PFM support
	t.Logf("OutboundSupportsPFM: %v", brokerRoute.OutboundSupportsPFM)
	// Note: With SmartRoute = nil (default), execution data is not generated
	// This is expected - execution data is only built when SmartRoute = true
	if brokerRoute.Execution != nil {
		if brokerRoute.Execution.Memo != nil {
			t.Logf("Execution Memo: %s", *brokerRoute.Execution.Memo)
		}
	} else {
		t.Log("Note: Execution data not available (SmartRoute not enabled in test)")
	}
	if !brokerRoute.OutboundSupportsPFM {
		t.Log("Note: Outbound PFM not supported, will require manual forwarding")
	}

	// if all goes well
	t.Logf("Broker swap route test passed")
}

func TestPathfinder_IndirectRoute(t *testing.T) {
	pathfinder, _ := setupTestPathfinder()

	// Test USDC from Juno -> Noble -> Osmosis (indirect route without swap)
	req := models.RouteRequest{
		ChainFrom:       "juno-1",
		ChainTo:         "osmosis-1",
		TokenFromDenom:  "ibc/EAC38D55372F38F1AFD68DF7FE9EF762DCF69F26520643CF3F9D292A738D8034",
		TokenToDenom:    "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4",
		AmountIn:        "5000000",
		SenderAddress:   "juno1sender",
		ReceiverAddress: "osmo1receiver",
	}

	response := pathfinder.FindPath(req)

	t.Logf("Response: %+v", response)
	assert.True(t, response.Success)
	assert.Equal(t, response.RouteType, "indirect")
	assert.NotNil(t, response.Indirect)

	indirect := response.Indirect
	assert.Equal(t, len(indirect.Path), 3)
	assert.Equal(t, indirect.Path[0], "juno-1")
	assert.Equal(t, indirect.Path[1], "noble-1")
	assert.Equal(t, indirect.Path[2], "osmosis-1")

	// Verify legs
	assert.Equal(t, len(indirect.Legs), 2)

	// First leg: Juno -> Noble
	leg1 := indirect.Legs[0]
	assert.Equal(t, leg1.FromChain, "juno-1")
	assert.Equal(t, leg1.ToChain, "noble-1")
	assert.Equal(t, leg1.Amount, "5000000")

	// Second leg: Noble -> Osmosis
	leg2 := indirect.Legs[1]
	assert.Equal(t, leg2.FromChain, "noble-1")
	assert.Equal(t, leg2.ToChain, "osmosis-1")

	// Verify PFM support - Noble supports PFM, so it should be enabled
	assert.True(t, indirect.SupportsPFM)
	if indirect.PFMMemo == "" {
		t.Error("Expected PFM memo for multi-hop route with PFM support")
	}
	t.Logf("PFM Memo: %s", indirect.PFMMemo)

	t.Logf("Indirect route test passed - USDC routes through Noble with PFM!")
}

func TestPathfinder_ImpossibleRoute(t *testing.T) {
	pathfinder, _ := setupTestPathfinder()

	// Try to route to a non-existent chain
	req := models.RouteRequest{
		ChainFrom:       "cosmoshub-4",
		ChainTo:         "nonexistent-1",
		TokenFromDenom:  "uatom",
		TokenToDenom:    "unonexist",
		AmountIn:        "1000000",
		SenderAddress:   "cosmos1sender",
		ReceiverAddress: "nonexist1receiver",
	}

	response := pathfinder.FindPath(req)

	t.Logf("Response: %+v", response)
	assert.False(t, response.Success)
	assert.Equal(t, response.RouteType, "impossible")
	if response.ErrorMessage == "" {
		t.Error("Expected error message for impossible route")
	}

	// if all goes well
	t.Logf("Impossible route test passed")
}

func TestPathfinder_AllChainPairs(t *testing.T) {
	pathfinder, _ := setupTestPathfinder()

	testCases := []struct {
		name        string
		from        string
		to          string
		tokenFrom   string
		tokenTo     string
		expectRoute string
	}{
		{
			name:        "Cosmos to Osmosis (direct)",
			from:        "cosmoshub-4",
			to:          "osmosis-1",
			tokenFrom:   "uatom",
			tokenTo:     "ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2",
			expectRoute: "direct",
		},
		{
			name:        "Juno to Osmosis (direct)",
			from:        "juno-1",
			to:          "osmosis-1",
			tokenFrom:   "ujuno",
			tokenTo:     "ibc/ujuno-osmosis",
			expectRoute: "direct",
		},
		{
			name:        "Cosmos to Juno (broker swap)",
			from:        "cosmoshub-4",
			to:          "juno-1",
			tokenFrom:   "uatom",
			tokenTo:     "ujuno",
			expectRoute: "broker_swap",
		},
		{
			name:        "AtomOne to Osmosis (indirect)",
			from:        "atomone-1",
			to:          "osmosis-1",
			tokenFrom:   "uatone",
			tokenTo:     "ibc/uatone-osmo",
			expectRoute: "indirect",
		},
		{
			name:        "Juno to Osmosis USDC (indirect via Noble)",
			from:        "juno-1",
			to:          "osmosis-1",
			tokenFrom:   "ibc/EAC38D55372F38F1AFD68DF7FE9EF762DCF69F26520643CF3F9D292A738D8034",
			tokenTo:     "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4",
			expectRoute: "indirect",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := models.RouteRequest{
				ChainFrom:       tc.from,
				ChainTo:         tc.to,
				TokenFromDenom:  tc.tokenFrom,
				TokenToDenom:    tc.tokenTo,
				AmountIn:        "1000000",
				SenderAddress:   "sender123",
				ReceiverAddress: "receiver456",
			}

			response := pathfinder.FindPath(req)

			t.Logf("%s: RouteType=%s, Success=%v", tc.name, response.RouteType, response.Success)

			if !response.Success {
				t.Errorf("Expected success for %s", tc.name)
			}
			if response.RouteType != tc.expectRoute {
				t.Errorf("Expected %s route type, got %s", tc.expectRoute, response.RouteType)
			}
		})
	}

	// if all goes well
	t.Logf("All chain pairs test passed")
}

func TestPathfinder_GetChainInfo(t *testing.T) {
	pathfinder, _ := setupTestPathfinder()

	// Test getting valid chain info
	chain, err := pathfinder.GetChainInfo("cosmoshub-4")
	assert.NoError(t, err)
	assert.Equal(t, chain.Name, "Cosmos Hub")
	assert.Equal(t, chain.Id, "cosmoshub-4")
	assert.True(t, chain.HasPFM)
	assert.False(t, chain.Broker)

	// Test getting broker chain info
	brokerChain, err := pathfinder.GetChainInfo("osmosis-1")
	assert.NoError(t, err)
	assert.Equal(t, brokerChain.Name, "Osmosis")
	assert.True(t, brokerChain.Broker)
	assert.Equal(t, brokerChain.BrokerId, "osmosis-sqs")

	// Test non-existent chain
	_, err = pathfinder.GetChainInfo("nonexistent-1")
	assert.Error(t, err)

	// if all goes well
	t.Logf("GetChainInfo test passed")
}

func TestPathfinder_GetAllChains(t *testing.T) {
	pathfinder, _ := setupTestPathfinder()

	chains := pathfinder.GetAllChains()

	t.Logf("All chains: %v", chains)
	assert.True(t, len(chains) == 5)

	// Check that all expected chains are present
	chainMap := make(map[string]bool)
	for _, chain := range chains {
		chainMap[chain] = true
	}

	assert.True(t, chainMap["cosmoshub-4"])
	assert.True(t, chainMap["osmosis-1"])
	assert.True(t, chainMap["juno-1"])
	assert.True(t, chainMap["atomone-1"])
	assert.True(t, chainMap["noble-1"])

	// if all goes well
	t.Logf("GetAllChains test passed")
}

// Benchmark tests
func BenchmarkPathfinder_DirectRoute(b *testing.B) {
	pathfinder, _ := setupTestPathfinder()

	req := models.RouteRequest{
		ChainFrom:       "cosmoshub-4",
		ChainTo:         "osmosis-1",
		TokenFromDenom:  "uatom",
		TokenToDenom:    "ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2",
		AmountIn:        "1000000",
		SenderAddress:   "cosmos1sender",
		ReceiverAddress: "osmo1receiver",
	}

	for b.Loop() {
		pathfinder.FindPath(req)
	}
}

func BenchmarkPathfinder_BrokerSwapRoute(b *testing.B) {
	pathfinder, _ := setupTestPathfinder()

	req := models.RouteRequest{
		ChainFrom:       "cosmoshub-4",
		ChainTo:         "juno-1",
		TokenFromDenom:  "uatom",
		TokenToDenom:    "ujuno",
		AmountIn:        "1000000",
		SenderAddress:   "cosmos1sender",
		ReceiverAddress: "juno1receiver",
	}

	for b.Loop() {
		pathfinder.FindPath(req)
	}
}

func BenchmarkPathfinder_IndirectRoute(b *testing.B) {
	pathfinder, _ := setupTestPathfinder()

	req := models.RouteRequest{
		ChainFrom:      "juno-1",
		ChainTo:        "osmosis-1",
		TokenFromDenom: "ibc/EAC38D55372F38F1AFD68DF7FE9EF762DCF69F26520643CF3F9D292A738D8034",
		TokenToDenom:   "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4",
	}

	for b.Loop() {
		pathfinder.FindPath(req)
	}
}
