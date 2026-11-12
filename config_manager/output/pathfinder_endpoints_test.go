package output

import (
	"testing"

	"github.com/Cogwheel-Validator/spectra-portal/config_manager/enriched"
	"github.com/zeebo/assert"
)

func TestConvertChainCarriesOnlyHealthyEndpoints(t *testing.T) {
	c := NewPathfinderConverter()
	got := c.convertChain(&enriched.ChainConfig{
		ID: "osmosis-1", AccountType: "cosmos", CosmosSdkVersion: "v0.50.9",
		HealthyRPCs: []enriched.Endpoint{
			{URL: "https://rpc-a", Healthy: true},
			{URL: "https://rpc-down", Healthy: false},
		},
		HealthyRests: []enriched.Endpoint{
			{URL: "https://rest-a", Healthy: true},
			{URL: "", Healthy: true},
			{URL: "https://rest-b", Healthy: true},
		},
	})
	assert.Equal(t, got.RPCs, []string{"https://rpc-a"})
	assert.Equal(t, got.Rests, []string{"https://rest-a", "https://rest-b"})
	assert.Equal(t, got.AccountType, "cosmos")
	assert.Equal(t, got.CosmosSdkVersion, "v0.50.9")
}

func TestConvertChainWithoutEndpointsLeavesThemEmpty(t *testing.T) {
	got := NewPathfinderConverter().convertChain(&enriched.ChainConfig{ID: "x"})
	assert.Equal(t, len(got.RPCs), 0)
	assert.Equal(t, len(got.Rests), 0)
}
