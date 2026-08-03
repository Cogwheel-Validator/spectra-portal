package router

import (
	"errors"
	"strings"
	"testing"

	models "github.com/Cogwheel-Validator/spectra-portal/pathfinder/models"
	"github.com/btcsuite/btcutil/bech32"
	"github.com/zeebo/assert"
)

var resolverChains = []PathfinderChain{
	{Name: "Cosmos Hub", Id: "cosmoshub-4", Bech32Prefix: "cosmos", Slip44: 118},
	{Name: "Osmosis", Id: "osmosis-1", Bech32Prefix: "osmo", Slip44: 118},
	// Different SLIP-44 coin type than the Cosmos chains.
	{Name: "Evmos", Id: "evmos_9001-2", Bech32Prefix: "evmos", Slip44: 60},
	// No bech32 prefix configured.
	{Name: "Nameless", Id: "nameless-1", Slip44: 118},
}

func resolverPathfinder() *Pathfinder {
	chainMap := make(map[string]PathfinderChain, len(resolverChains))
	for _, chain := range resolverChains {
		chainMap[chain.Id] = chain
	}
	return &Pathfinder{
		chainsMap:        chainMap,
		addressConverter: NewAddressConverter(resolverChains),
	}
}

// resolverAddr builds a valid bech32 address with the given prefix from 20
// bytes of non-zero account data.
func resolverAddr(t *testing.T, prefix string) string {
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

func TestPlaceholderAddress(t *testing.T) {
	s := resolverPathfinder()

	addr, err := s.placeholderAddress("osmosis-1")
	assert.NoError(t, err)
	assert.True(t, strings.HasPrefix(addr, "osmo1"))

	// Must be the bech32 encoding of 20 zero bytes.
	prefix, data, err := bech32.Decode(addr)
	assert.NoError(t, err)
	assert.Equal(t, prefix, "osmo")
	raw, err := bech32.ConvertBits(data, 5, 8, false)
	assert.NoError(t, err)
	assert.Equal(t, len(raw), 20)
	for _, b := range raw {
		assert.Equal(t, int(b), 0)
	}

	_, err = s.placeholderAddress("unknown-1")
	assert.Error(t, err)
	_, err = s.placeholderAddress("nameless-1")
	assert.Error(t, err)
}

func TestResolveRouteAddresses_PlaceholderMode(t *testing.T) {
	s := resolverPathfinder()
	req := models.RouteRequest{ChainFrom: "cosmoshub-4", ChainTo: "evmos_9001-2"}

	resolved, err := s.resolveRouteAddresses(req, []AddressNeed{
		{ChainID: "cosmoshub-4", Role: RoleSender, Required: true},
		{ChainID: "osmosis-1", Role: RoleSender, Required: true},
		{ChainID: "evmos_9001-2", Role: RoleReceiver, Required: true},
	})
	assert.NoError(t, err)
	assert.True(t, resolved.Placeholders)
	assert.True(t, strings.HasPrefix(resolved.On("cosmoshub-4", RoleSender), "cosmos1"))
	assert.True(t, strings.HasPrefix(resolved.On("osmosis-1", RoleSender), "osmo1"))
	// Placeholder mode works across differing slip-44 coin types - no
	// conversion happens.
	assert.True(t, strings.HasPrefix(resolved.On("evmos_9001-2", RoleReceiver), "evmos1"))
	assert.DeepEqual(t, resolved.NeededChains, []string{"cosmoshub-4", "evmos_9001-2", "osmosis-1"})
}

func TestResolveRouteAddresses_StrictMissingChains(t *testing.T) {
	s := resolverPathfinder()
	req := models.RouteRequest{
		ChainFrom: "cosmoshub-4",
		ChainTo:   "evmos_9001-2",
		Addresses: map[string]string{
			"cosmoshub-4": resolverAddr(t, "cosmos"),
		},
	}

	_, err := s.resolveRouteAddresses(req, []AddressNeed{
		{ChainID: "cosmoshub-4", Role: RoleSender, Required: true},
		{ChainID: "osmosis-1", Role: RoleSender, Required: true},
		// Required=false still counts as missing in strict mode.
		{ChainID: "evmos_9001-2", Role: RoleReceiver, Required: false},
	})
	assert.Error(t, err)
	var missingErr *MissingAddressesError
	assert.True(t, errors.As(err, &missingErr))
	assert.DeepEqual(t, missingErr.ChainIDs, []string{"evmos_9001-2", "osmosis-1"})
}

func TestResolveRouteAddresses_DeriveMode(t *testing.T) {
	s := resolverPathfinder()
	cosmosAddr := resolverAddr(t, "cosmos")
	osmoAddr := resolverAddr(t, "osmo")
	req := models.RouteRequest{
		ChainFrom: "osmosis-1",
		ChainTo:   "cosmoshub-4",
		Addresses: map[string]string{
			"osmosis-1":   osmoAddr,
			"cosmoshub-4": cosmosAddr,
		},
		DeriveMissing: true,
	}

	resolved, err := s.resolveRouteAddresses(req, []AddressNeed{
		{ChainID: "osmosis-1", Role: RoleSender, Required: true},
		{ChainID: "cosmoshub-4", Role: RoleSender, Required: true},
		{ChainID: "cosmoshub-4", Role: RoleReceiver, Required: true},
		// Derivation to a different slip-44 fails, but non-required needs
		// are simply left absent (legacy lenient fallback).
		{ChainID: "evmos_9001-2", Role: RoleReceiver, Required: false},
	})
	assert.NoError(t, err)
	assert.False(t, resolved.Placeholders)
	// Map hit wins over derivation.
	assert.Equal(t, resolved.On("cosmoshub-4", RoleReceiver), cosmosAddr)
	// Derived from the sender's osmo address; same account bytes, cosmos prefix.
	derived, err := s.addressConverter.ConvertAddress(osmoAddr, "cosmoshub-4")
	assert.NoError(t, err)
	assert.Equal(t, resolved.On("cosmoshub-4", RoleSender), derived)
	_, ok := resolved.Lookup("evmos_9001-2", RoleReceiver)
	assert.False(t, ok)
}

func TestResolveRouteAddresses_DeriveModeRequiredFailure(t *testing.T) {
	s := resolverPathfinder()
	req := models.RouteRequest{
		ChainFrom: "cosmoshub-4",
		ChainTo:   "evmos_9001-2",
		Addresses: map[string]string{
			"cosmoshub-4":  resolverAddr(t, "cosmos"),
			"evmos_9001-2": resolverAddr(t, "evmos"),
		},
		DeriveMissing: true,
	}

	// The broker-chain address must derive from the sender, but deriving a
	// cosmos address for an unknown chain fails -> plain error (not a
	// MissingAddressesError), matching the legacy best-effort behavior.
	_, err := s.resolveRouteAddresses(req, []AddressNeed{
		{ChainID: "cosmoshub-4", Role: RoleSender, Required: true},
		{ChainID: "unknown-1", Role: RoleSender, Required: true},
	})
	assert.Error(t, err)
	var missingErr *MissingAddressesError
	assert.False(t, errors.As(err, &missingErr))
}

func TestResolveRouteAddresses_RoleKeyed(t *testing.T) {
	s := resolverPathfinder()
	cosmosAddr := resolverAddr(t, "cosmos")
	osmoAddr := resolverAddr(t, "osmo")
	req := models.RouteRequest{
		ChainFrom: "cosmoshub-4",
		ChainTo:   "osmosis-1",
		Addresses: map[string]string{
			"cosmoshub-4": cosmosAddr,
			"osmosis-1":   osmoAddr,
		},
	}

	resolved, err := s.resolveRouteAddresses(req, []AddressNeed{
		{ChainID: "osmosis-1", Role: RoleSender, Required: true},
		{ChainID: "osmosis-1", Role: RoleReceiver, Required: true},
	})
	assert.NoError(t, err)
	// In map mode both roles resolve to the same supplied address.
	assert.Equal(t, resolved.On("osmosis-1", RoleSender), osmoAddr)
	assert.Equal(t, resolved.On("osmosis-1", RoleReceiver), osmoAddr)
	// NeededChains is deduplicated.
	assert.DeepEqual(t, resolved.NeededChains, []string{"osmosis-1"})
}
