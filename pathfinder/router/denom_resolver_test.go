package router_test

import (
	"testing"

	router "github.com/Cogwheel-Validator/spectra-portal/pathfinder/router"
	"github.com/zeebo/assert"
)

func setupResolver(t *testing.T) *router.DenomResolver {
	t.Helper()
	routeIndex := router.NewRouteIndex()
	assert.NoError(t, routeIndex.BuildIndex(chains))
	resolver := router.NewDenomResolver(routeIndex)
	resolver.SetChains(chains)
	return resolver
}

func TestDenomResolver_ResolveNativeDenom(t *testing.T) {
	resolver := setupResolver(t)

	info, err := resolver.ResolveDenom("cosmoshub-4", "uatom")
	assert.NoError(t, err)
	assert.Equal(t, info.ChainDenom, "uatom")
	assert.Equal(t, info.BaseDenom, "uatom")
	assert.Equal(t, info.OriginChain, "cosmoshub-4")
	assert.True(t, info.IsNative)
	assert.Equal(t, info.IbcPath, "")
}

func TestDenomResolver_ResolveIBCDenom(t *testing.T) {
	resolver := setupResolver(t)

	// ATOM on Osmosis via its IBC hash
	info, err := resolver.ResolveDenom("osmosis-1",
		"ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2")
	assert.NoError(t, err)
	assert.Equal(t, info.BaseDenom, "uatom")
	assert.Equal(t, info.OriginChain, "cosmoshub-4")
	assert.False(t, info.IsNative)
	assert.Equal(t, info.IbcPath, "transfer/channel-0")
}

func TestDenomResolver_ResolveHumanReadableDenom(t *testing.T) {
	resolver := setupResolver(t)

	// "uatom" is not a key on Osmosis (only its IBC hash is), so this exercises
	// the human-readable fallback.
	info, err := resolver.ResolveDenom("osmosis-1", "uatom")
	assert.NoError(t, err)
	assert.Equal(t, info.ChainDenom,
		"ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2")
	assert.Equal(t, info.OriginChain, "cosmoshub-4")

	// Disambiguation syntax with the origin chain
	info, err = resolver.ResolveDenom("osmosis-1", "uusdc@noble-1")
	assert.NoError(t, err)
	assert.Equal(t, info.ChainDenom,
		"ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4")
	assert.Equal(t, info.OriginChain, "noble-1")

	// Wrong origin in the disambiguation must not match
	_, err = resolver.ResolveDenom("osmosis-1", "uusdc@juno-1")
	assert.Error(t, err)
}

func TestDenomResolver_AmbiguousDenom(t *testing.T) {
	resolver := setupResolver(t)

	// The Cosmos Hub fixture carries two different IBC denoms that both resolve
	// to base denom "ujuno" (channel-1 direct and a via-osmosis variant), so a
	// bare "ujuno" lookup must not silently pick one.
	// Note: ResolveDenom currently masks the "ambiguous" detail with a generic
	// not-found error; either way the lookup must fail.
	_, err := resolver.ResolveDenom("cosmoshub-4", "ujuno")
	assert.Error(t, err)
}

func TestDenomResolver_UnknownDenomAndChain(t *testing.T) {
	resolver := setupResolver(t)

	_, err := resolver.ResolveDenom("cosmoshub-4", "unotexist")
	assert.Error(t, err)

	_, err = resolver.ResolveDenom("unknown-1", "uatom")
	assert.Error(t, err)
}

func TestDenomResolver_InferTokenToDenom(t *testing.T) {
	resolver := setupResolver(t)

	// ATOM from the hub lands on Osmosis as its IBC hash
	denom, err := resolver.InferTokenToDenom("cosmoshub-4", "uatom", "osmosis-1")
	assert.NoError(t, err)
	assert.Equal(t, denom,
		"ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2")

	// USDC from Juno inferred on Osmosis
	denom, err = resolver.InferTokenToDenom("juno-1",
		"ibc/EAC38D55372F38F1AFD68DF7FE9EF762DCF69F26520643CF3F9D292A738D8034", "osmosis-1")
	assert.NoError(t, err)
	assert.Equal(t, denom,
		"ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4")

	// PHOTON does not exist on Juno
	_, err = resolver.InferTokenToDenom("atomone-1", "uphoton", "juno-1")
	assert.Error(t, err)
}

func TestDenomResolver_GetTokenDenomsAcrossChains(t *testing.T) {
	resolver := setupResolver(t)

	// USDC (native to Noble) is known on Noble, Osmosis and Juno
	denoms, found := resolver.GetTokenDenomsAcrossChains("uusdc", "noble-1", "")
	assert.True(t, found)
	assert.Equal(t, len(denoms), 3)

	byChain := map[string]string{}
	for _, d := range denoms {
		byChain[d.ChainID] = d.Denom
	}
	assert.Equal(t, byChain["noble-1"], "uusdc")
	assert.Equal(t, byChain["osmosis-1"],
		"ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4")
	assert.Equal(t, byChain["juno-1"],
		"ibc/EAC38D55372F38F1AFD68DF7FE9EF762DCF69F26520643CF3F9D292A738D8034")

	// Filtered to a single chain
	denoms, found = resolver.GetTokenDenomsAcrossChains("uusdc", "noble-1", "juno-1")
	assert.True(t, found)
	assert.Equal(t, len(denoms), 1)
	assert.Equal(t, denoms[0].ChainID, "juno-1")

	// Unknown token
	_, found = resolver.GetTokenDenomsAcrossChains("unotexist", "nowhere-1", "")
	assert.False(t, found)
}

func TestDenomResolver_GetChainTokens(t *testing.T) {
	resolver := setupResolver(t)

	tokens, err := resolver.GetChainTokens("noble-1")
	assert.NoError(t, err)
	assert.Equal(t, tokens.ChainID, "noble-1")
	assert.Equal(t, tokens.ChainName, "Noble")
	assert.Equal(t, len(tokens.NativeTokens), 1)
	assert.Equal(t, tokens.NativeTokens[0].Denom, "uusdc")
	assert.Equal(t, len(tokens.IBCTokens), 0)

	_, err = resolver.GetChainTokens("unknown-1")
	assert.Error(t, err)
}

func TestDenomResolver_GetDenomOnChain(t *testing.T) {
	resolver := setupResolver(t)

	token := &router.TokenInfo{
		ChainDenom:  "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4",
		IbcDenom:    "uusdc",
		BaseDenom:   "uusdc",
		OriginChain: "noble-1",
	}

	// Returning to the origin chain unwinds to the base denom
	assert.Equal(t, resolver.GetDenomOnChain(token, "noble-1"), "uusdc")
	// Any other chain uses the IBC denom
	assert.Equal(t, resolver.GetDenomOnChain(token, "juno-1"), "uusdc")

	native := &router.TokenInfo{
		ChainDenom: "uatom", IbcDenom: "ibc/ATOMHASH", BaseDenom: "uatom", OriginChain: "cosmoshub-4",
	}
	assert.Equal(t, resolver.GetDenomOnChain(native, "cosmoshub-4"), "uatom")
	assert.Equal(t, resolver.GetDenomOnChain(native, "osmosis-1"), "ibc/ATOMHASH")

	assert.True(t, resolver.IsTokenNativeToChain(native, "cosmoshub-4"))
	assert.False(t, resolver.IsTokenNativeToChain(native, "osmosis-1"))
}

func TestDenomResolver_CreateTokenMapping(t *testing.T) {
	resolver := setupResolver(t)

	mapping, err := resolver.CreateTokenMapping("cosmoshub-4", "uatom")
	assert.NoError(t, err)
	assert.Equal(t, mapping.ChainDenom, "uatom")
	assert.True(t, mapping.IsNative)

	// Unknown denom falls back to assuming a native token rather than failing
	mapping, err = resolver.CreateTokenMapping("cosmoshub-4", "ucustom")
	assert.NoError(t, err)
	assert.Equal(t, mapping.ChainDenom, "ucustom")
	assert.Equal(t, mapping.OriginChain, "cosmoshub-4")
	assert.True(t, mapping.IsNative)
}

func TestDenomResolver_ResolveToChainDenom(t *testing.T) {
	resolver := setupResolver(t)

	denom, err := resolver.ResolveToChainDenom("osmosis-1", "uatom")
	assert.NoError(t, err)
	assert.Equal(t, denom,
		"ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2")

	_, err = resolver.ResolveToChainDenom("osmosis-1", "unotexist")
	assert.Error(t, err)
}
