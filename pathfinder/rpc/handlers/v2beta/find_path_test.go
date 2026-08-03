package v2betahandlers

import (
	"strings"
	"testing"

	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/handlers/common"
	v2beta "github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/services/pathfinder/v2beta"
	"github.com/btcsuite/btcutil/bech32"
	"github.com/zeebo/assert"
)

func testServer() *FindPathServer {
	chains := []router.PathfinderChain{
		{Name: "Cosmos Hub", Id: "cosmoshub-4", Bech32Prefix: "cosmos", Slip44: 118},
		{Name: "Osmosis", Id: "osmosis-1", Bech32Prefix: "osmo", Slip44: 118},
	}
	pathfinder := router.NewPathfinder(chains, router.NewRouteIndex(), nil)
	return NewFindPathServer(common.Deps{Pathfinder: pathfinder})
}

func testAddr(t *testing.T, prefix string) string {
	t.Helper()
	raw := make([]byte, 20)
	for i := range raw {
		raw[i] = byte(i + 1)
	}
	converted, err := bech32.ConvertBits(raw, 8, 5, true)
	assert.NoError(t, err)
	addr, err := bech32.Encode(prefix, converted)
	assert.NoError(t, err)
	return addr
}

func baseParams(t *testing.T, addresses []*v2beta.ChainAddress) findPathParams {
	t.Helper()
	return findPathParams{
		ChainFrom:      "cosmoshub-4",
		TokenFromDenom: "uatom",
		AmountIn:       "1000000",
		ChainTo:        "osmosis-1",
		TokenToDenom:   "uosmo",
		Addresses:      addresses,
	}
}

func TestValidateRequest_FullAddresses(t *testing.T) {
	s := testServer()
	cosmosAddr := testAddr(t, "cosmos")
	osmoAddr := testAddr(t, "osmo")

	addresses, err := s.validateRequest(baseParams(t, []*v2beta.ChainAddress{
		{ChainId: "cosmoshub-4", Address: cosmosAddr},
		{ChainId: "osmosis-1", Address: osmoAddr},
	}), true)
	assert.NoError(t, err)
	assert.Equal(t, addresses["cosmoshub-4"], cosmosAddr)
	assert.Equal(t, addresses["osmosis-1"], osmoAddr)
}

func TestValidateRequest_EmptyAddresses(t *testing.T) {
	s := testServer()

	// Unary mock mode: empty is allowed and yields a nil map.
	addresses, err := s.validateRequest(baseParams(t, nil), true)
	assert.NoError(t, err)
	assert.Nil(t, addresses)

	// Streaming: empty is rejected.
	_, err = s.validateRequest(baseParams(t, nil), false)
	assert.Error(t, err)
}

func TestValidateRequest_DuplicateChain(t *testing.T) {
	s := testServer()
	cosmosAddr := testAddr(t, "cosmos")

	_, err := s.validateRequest(baseParams(t, []*v2beta.ChainAddress{
		{ChainId: "cosmoshub-4", Address: cosmosAddr},
		{ChainId: "cosmoshub-4", Address: cosmosAddr},
	}), true)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "duplicate"))
}

func TestValidateRequest_WrongPrefix(t *testing.T) {
	s := testServer()

	_, err := s.validateRequest(baseParams(t, []*v2beta.ChainAddress{
		{ChainId: "cosmoshub-4", Address: testAddr(t, "osmo")},
		{ChainId: "osmosis-1", Address: testAddr(t, "osmo")},
	}), true)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "prefix"))
}

func TestValidateRequest_UnknownChain(t *testing.T) {
	s := testServer()

	_, err := s.validateRequest(baseParams(t, []*v2beta.ChainAddress{
		{ChainId: "cosmoshub-4", Address: testAddr(t, "cosmos")},
		{ChainId: "juno-1", Address: testAddr(t, "juno")},
	}), true)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "unknown chain"))
}

func TestValidateRequest_MissingSourceOrDest(t *testing.T) {
	s := testServer()

	// Non-empty list must cover both chain_from and chain_to.
	_, err := s.validateRequest(baseParams(t, []*v2beta.ChainAddress{
		{ChainId: "osmosis-1", Address: testAddr(t, "osmo")},
		{ChainId: "osmosis-1", Address: testAddr(t, "osmo")},
	}), true)
	assert.Error(t, err)

	_, err = s.validateRequest(baseParams(t, []*v2beta.ChainAddress{
		{ChainId: "cosmoshub-4", Address: testAddr(t, "cosmos")},
	}), true)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "osmosis-1"))
}
