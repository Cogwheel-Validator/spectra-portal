package router_test

import (
	"testing"

	models "github.com/Cogwheel-Validator/spectra-portal/pathfinder/models"
	"github.com/zeebo/assert"
)

// Mock discovery: an empty address map finds the route, reports which chains
// need addresses, and never emits execution data or memos.
func TestPathfinder_MockDiscovery_BrokerRoute(t *testing.T) {
	pathfinder, _ := setupTestPathfinder()

	req := models.RouteRequest{
		ChainFrom:      "cosmoshub-4",
		ChainTo:        "juno-1",
		TokenFromDenom: "uatom",
		TokenToDenom:   "ujuno",
		AmountIn:       "1000000",
		SmartRoute:     boolPtr(true),
		SlippageBps:    uint32Ptr(100),
	}

	response := pathfinder.FindPath(req)

	assert.True(t, response.Success)
	assert.True(t, response.Mock)
	assert.Equal(t, response.RouteType, "broker_swap")
	assert.NotNil(t, response.BrokerSwap)
	// The route topology and quote are the answer...
	assert.NotNil(t, response.BrokerSwap.Swap)
	// ...but nothing signable is ever built from placeholder addresses.
	assert.Nil(t, response.BrokerSwap.Execution)
	// The dev learns exactly which chains need an address in a real request.
	assert.DeepEqual(t, response.RequiredChains, []string{"cosmoshub-4", "juno-1", "osmosis-1"})
}

func TestPathfinder_MockDiscovery_DirectRoute(t *testing.T) {
	pathfinder, _ := setupTestPathfinder()

	req := models.RouteRequest{
		ChainFrom:      "cosmoshub-4",
		ChainTo:        "osmosis-1",
		TokenFromDenom: "uatom",
		AmountIn:       "1000000",
		TokenToDenom:   "ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2",
		SmartRoute:     boolPtr(true),
		SlippageBps:    uint32Ptr(100),
	}

	response := pathfinder.FindPath(req)

	assert.True(t, response.Success)
	assert.True(t, response.Mock)
	assert.Equal(t, response.RouteType, "direct")
	assert.DeepEqual(t, response.RequiredChains, []string{"cosmoshub-4", "osmosis-1"})
}

// Strict v2 mode: a non-empty address map missing a chain the route needs
// (here the osmosis-1 broker) fails hard, naming the missing chains.
func TestPathfinder_StrictMissingBrokerAddress(t *testing.T) {
	pathfinder, _ := setupTestPathfinder()

	req := models.RouteRequest{
		ChainFrom:      "cosmoshub-4",
		ChainTo:        "juno-1",
		TokenFromDenom: "uatom",
		TokenToDenom:   "ujuno",
		AmountIn:       "1000000",
		Addresses: map[string]string{
			"cosmoshub-4": testAddr(t, "cosmos"),
			"juno-1":      testAddr(t, "juno"),
		},
		SmartRoute:  boolPtr(true),
		SlippageBps: uint32Ptr(100),
	}

	response := pathfinder.FindPath(req)

	assert.False(t, response.Success)
	assert.False(t, response.Mock)
	assert.Equal(t, response.RouteType, "impossible")
	assert.DeepEqual(t, response.MissingAddressChains, []string{"osmosis-1"})
}

// Strict v2 mode with a complete address map builds real execution data from
// the supplied addresses, with no derivation involved.
func TestPathfinder_StrictFullAddressMap(t *testing.T) {
	pathfinder, _ := setupTestPathfinder()

	junoAddr := testAddr(t, "juno")
	osmoAddr := testAddr(t, "osmo")

	req := models.RouteRequest{
		ChainFrom:      "cosmoshub-4",
		ChainTo:        "juno-1",
		TokenFromDenom: "uatom",
		TokenToDenom:   "ujuno",
		AmountIn:       "1000000",
		Addresses: map[string]string{
			"cosmoshub-4": testAddr(t, "cosmos"),
			"osmosis-1":   osmoAddr,
			"juno-1":      junoAddr,
		},
		SmartRoute:  boolPtr(true),
		SlippageBps: uint32Ptr(100),
	}

	response := pathfinder.FindPath(req)

	assert.True(t, response.Success)
	assert.False(t, response.Mock)
	assert.NotNil(t, response.BrokerSwap)
	assert.NotNil(t, response.BrokerSwap.Execution)
	// The recover address is the caller-supplied broker-chain address, not a
	// derived one.
	assert.NotNil(t, response.BrokerSwap.Execution.RecoverAddress)
	assert.Equal(t, *response.BrokerSwap.Execution.RecoverAddress, osmoAddr)
	assert.DeepEqual(t, response.RequiredChains, []string{"cosmoshub-4", "juno-1", "osmosis-1"})
}

// Manual (non-smart) requests skip execution data but still report the
// chains an execution would require.
func TestPathfinder_ManualRouteStillReportsRequiredChains(t *testing.T) {
	pathfinder, _ := setupTestPathfinder()

	req := models.RouteRequest{
		ChainFrom:      "cosmoshub-4",
		ChainTo:        "juno-1",
		TokenFromDenom: "uatom",
		TokenToDenom:   "ujuno",
		AmountIn:       "1000000",
		SmartRoute:     boolPtr(false),
		SlippageBps:    uint32Ptr(100),
	}

	response := pathfinder.FindPath(req)

	assert.True(t, response.Success)
	assert.True(t, response.Mock)
	assert.Nil(t, response.BrokerSwap.Execution)
	assert.DeepEqual(t, response.RequiredChains, []string{"cosmoshub-4", "juno-1", "osmosis-1"})
}
