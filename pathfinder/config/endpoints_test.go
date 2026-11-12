package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/config"
	"github.com/zeebo/assert"
)

func TestChainLoaderReadsEndpointFieldsAndStaysCompatibleWithOldConfigs(t *testing.T) {
	dir := t.TempDir()
	withEndpoints := `version = 'v1'
[[chains]]
name = 'Osmosis'
id = 'osmosis-1'
bech32_prefix = 'osmo'
slip44 = 118
account_type = 'cosmos'
cosmos_sdk_version = 'v0.50.9'
rpcs = ['https://rpc.example']
rests = ['https://rest.example']
`
	old := `version = 'v1'
[[chains]]
name = 'Osmosis'
id = 'osmosis-1'
bech32_prefix = 'osmo'
slip44 = 118
`
	l := config.NewChainConfigLoader()

	p := filepath.Join(dir, "new.toml")
	assert.NoError(t, os.WriteFile(p, []byte(withEndpoints), 0o600))
	chains, err := l.LoadFromFile(p)
	assert.NoError(t, err)
	assert.Equal(t, chains[0].Rests, []string{"https://rest.example"})
	assert.Equal(t, chains[0].RPCs, []string{"https://rpc.example"})
	assert.Equal(t, chains[0].AccountType, "cosmos")
	assert.Equal(t, chains[0].CosmosSdkVersion, "v0.50.9")

	// Configs generated before the endpoint fields existed must still load.
	p = filepath.Join(dir, "old.toml")
	assert.NoError(t, os.WriteFile(p, []byte(old), 0o600))
	chains, err = l.LoadFromFile(p)
	assert.NoError(t, err)
	assert.Equal(t, len(chains[0].Rests), 0)
}
