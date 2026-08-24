package addressing_test

import (
	"strings"
	"testing"

	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/addressing"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/routeindex"
	"github.com/btcsuite/btcutil/bech32"
	"github.com/zeebo/assert"
)

// makeAddress builds a valid bech32 address with the given prefix from 20 bytes
// of account data, mirroring how a real account address is encoded.
func makeAddress(t *testing.T, prefix string) string {
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

var slip44Chains = []routeindex.PathfinderChain{
	{Name: "Cosmos Hub", Id: "cosmoshub-4", Bech32Prefix: "cosmos", Slip44: 118},
	{Name: "Osmosis", Id: "osmosis-1", Bech32Prefix: "osmo", Slip44: 118},
	// Different SLIP-44 coin type than the Cosmos chains.
	{Name: "Evmos", Id: "evmos_9001-2", Bech32Prefix: "evmos", Slip44: 60},
}

func TestConvertAddress_SameSlip44(t *testing.T) {
	conv := addressing.NewAddressConverter(slip44Chains)
	cosmosAddr := makeAddress(t, "cosmos")

	got, err := conv.ConvertAddress(cosmosAddr, "osmosis-1")
	assert.NoError(t, err)
	assert.True(t, strings.HasPrefix(got, "osmo1"))

	// The account data must be preserved: converting back yields the original.
	roundTrip, err := conv.ConvertAddress(got, "cosmoshub-4")
	assert.NoError(t, err)
	assert.Equal(t, cosmosAddr, roundTrip)
}

func TestConvertAddress_DifferentSlip44IsBlocked(t *testing.T) {
	conv := addressing.NewAddressConverter(slip44Chains)
	cosmosAddr := makeAddress(t, "cosmos")

	_, err := conv.ConvertAddress(cosmosAddr, "evmos_9001-2")
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "SLIP-44"))
}

func TestConvertAddress_UnknownSourcePrefixIsBlocked(t *testing.T) {
	conv := addressing.NewAddressConverter(slip44Chains)
	// "stars" is not a configured chain, so its SLIP-44 type is unknown.
	starsAddr := makeAddress(t, "stars")

	_, err := conv.ConvertAddress(starsAddr, "osmosis-1")
	assert.Error(t, err)
}

func TestConvertAddress_UnknownTargetChain(t *testing.T) {
	conv := addressing.NewAddressConverter(slip44Chains)
	cosmosAddr := makeAddress(t, "cosmos")

	_, err := conv.ConvertAddress(cosmosAddr, "does-not-exist")
	assert.Error(t, err)
}

func TestDeriveRouteAddresses_DifferentSlip44IsBlocked(t *testing.T) {
	conv := addressing.NewAddressConverter(slip44Chains)
	cosmosAddr := makeAddress(t, "cosmos")

	_, err := conv.DeriveRouteAddresses(cosmosAddr, "evmos_9001-2", cosmosAddr)
	assert.Error(t, err)
}
