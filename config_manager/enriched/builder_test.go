package enriched

import (
	"testing"

	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/input"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/registry"
)

// createTestInputConfigs creates mock input configs for testing
func createTestInputConfigs() map[string]*input.ChainInput {
	pfmTrue := true
	return map[string]*input.ChainInput{
		"atomone-1": {
			Chain: input.ChainMeta{
				Name:         "Atom One",
				ID:           "atomone-1",
				Type:         "cosmos",
				Registry:     "atomone",
				ExplorerURL:  "https://thespectra.io/atomone",
				Slip44:       118,
				Bech32Prefix: "atone",
				RPCs:         []input.APIEndpoint{{URL: "https://atomone-rpc.example.com"}},
				Rest:         []input.APIEndpoint{{URL: "https://atomone-api.example.com"}},
			},
			Tokens: []input.TokenMeta{
				{Denom: "uatone", Name: "Atone", Symbol: "ATONE", Exponent: 6, Icon: "https://example.com/atone.png"},
				{Denom: "uphoton", Name: "Photon", Symbol: "PHOTON", Exponent: 6, Icon: "https://example.com/photon.png"},
			},
		},
		"osmosis-1": {
			Chain: input.ChainMeta{
				Name:         "Osmosis",
				ID:           "osmosis-1",
				Type:         "cosmos",
				Registry:     "osmosis",
				ExplorerURL:  "https://mintscan.io/osmosis",
				Slip44:       118,
				Bech32Prefix: "osmo",
				IsBroker:     true,
				BrokerID:     "osmosis-sqs",
				HasPFM:       &pfmTrue,
				RPCs:         []input.APIEndpoint{{URL: "https://osmosis-rpc.example.com"}},
				Rest:         []input.APIEndpoint{{URL: "https://osmosis-api.example.com"}},
			},
			Tokens: []input.TokenMeta{
				{Denom: "uosmo", Name: "Osmosis", Symbol: "OSMO", Exponent: 6, Icon: "https://example.com/osmo.png"},
			},
		},
	}
}

// createTestInputConfigsWithMultiHop creates configs with multi-hop token
func createTestInputConfigsWithMultiHop() map[string]*input.ChainInput {
	configs := createTestInputConfigs()

	// Add a third chain (Stargaze)
	configs["stargaze-1"] = &input.ChainInput{
		Chain: input.ChainMeta{
			Name:         "Stargaze",
			ID:           "stargaze-1",
			Type:         "cosmos",
			Registry:     "stargaze",
			ExplorerURL:  "https://mintscan.io/stargaze",
			Slip44:       118,
			Bech32Prefix: "stars",
			RPCs:         []input.APIEndpoint{{URL: "https://stargaze-rpc.example.com"}},
			Rest:         []input.APIEndpoint{{URL: "https://stargaze-api.example.com"}},
		},
		Tokens: []input.TokenMeta{
			{Denom: "ustars", Name: "Stargaze", Symbol: "STARS", Exponent: 6, Icon: "https://example.com/stars.png"},
		},
	}

	// Add received token on AtomOne - STARS via Osmosis
	configs["atomone-1"].ReceivedTokens = []input.ReceivedToken{
		{
			OriginDenom: "ustars",
			OriginChain: "stargaze-1",
			ViaChains:   []string{"osmosis-1"},
		},
	}

	return configs
}

// createTestIBCData creates mock IBC registry data
func createTestIBCData() []registry.ChainIbcData {
	return []registry.ChainIbcData{
		{
			Chain1: registry.IbcChainData{
				ChainName:    "atomone",
				ClientID:     "07-tendermint-2",
				ConnectionID: "connection-2",
			},
			Chain2: registry.IbcChainData{
				ChainName:    "osmosis",
				ClientID:     "07-tendermint-3396",
				ConnectionID: "connection-4829",
			},
			Channels: []registry.IbcChannelData{
				{
					Chain1: registry.ChannelChainData{
						ChannelID: "channel-2",
						PortID:    "transfer",
					},
					Chain2: registry.ChannelChainData{
						ChannelID: "channel-94814",
						PortID:    "transfer",
					},
					Ordering: "unordered",
					Version:  "ics20-1",
					Tags: registry.ChannelTags{
						Preferred: true,
						Status:    "ACTIVE",
					},
				},
			},
		},
	}
}

// createTestIBCDataWithStargaze creates IBC data with Stargaze connections
func createTestIBCDataWithStargaze() []registry.ChainIbcData {
	data := createTestIBCData()

	// Add Osmosis <-> Stargaze connection
	data = append(data, registry.ChainIbcData{
		Chain1: registry.IbcChainData{
			ChainName:    "osmosis",
			ClientID:     "07-tendermint-100",
			ConnectionID: "connection-100",
		},
		Chain2: registry.IbcChainData{
			ChainName:    "stargaze",
			ClientID:     "07-tendermint-200",
			ConnectionID: "connection-200",
		},
		Channels: []registry.IbcChannelData{
			{
				Chain1: registry.ChannelChainData{
					ChannelID: "channel-75",
					PortID:    "transfer",
				},
				Chain2: registry.ChannelChainData{
					ChannelID: "channel-0",
					PortID:    "transfer",
				},
				Ordering: "unordered",
				Version:  "ics20-1",
				Tags: registry.ChannelTags{
					Preferred: true,
					Status:    "ACTIVE",
				},
			},
		},
	})

	return data
}

func TestBuildRegistry(t *testing.T) {
	builder := NewBuilder()
	inputConfigs := createTestInputConfigs()
	ibcData := createTestIBCData()

	reg, err := builder.BuildRegistry(inputConfigs, ibcData)
	if err != nil {
		t.Fatalf("BuildRegistry() error = %v", err)
	}

	// Verify we have both chains
	if len(reg.Chains) != 2 {
		t.Errorf("BuildRegistry() returned %d chains, want 2", len(reg.Chains))
	}

	// Verify AtomOne chain
	atomone, ok := reg.Chains["atomone-1"]
	if !ok {
		t.Fatal("BuildRegistry() missing atomone-1 chain")
	}
	if atomone.Name != "Atom One" {
		t.Errorf("atomone Name = %q, want 'Atom One'", atomone.Name)
	}
	if len(atomone.NativeTokens) != 2 {
		t.Errorf("atomone NativeTokens = %d, want 2", len(atomone.NativeTokens))
	}

	// Verify Osmosis chain
	osmosis, ok := reg.Chains["osmosis-1"]
	if !ok {
		t.Fatal("BuildRegistry() missing osmosis-1 chain")
	}
	if !osmosis.IsBroker {
		t.Error("osmosis should be a broker")
	}
	if osmosis.BrokerID != "osmosis-sqs" {
		t.Errorf("osmosis BrokerID = %q, want 'osmosis-sqs'", osmosis.BrokerID)
	}
	if !osmosis.HasPFM {
		t.Error("osmosis should have PFM support")
	}
}

func TestBuildRoutes(t *testing.T) {
	builder := NewBuilder()
	inputConfigs := createTestInputConfigs()
	ibcData := createTestIBCData()

	reg, err := builder.BuildRegistry(inputConfigs, ibcData)
	if err != nil {
		t.Fatalf("BuildRegistry() error = %v", err)
	}

	// Verify AtomOne has a route to Osmosis
	atomone := reg.Chains["atomone-1"]
	if len(atomone.Routes) != 1 {
		t.Fatalf("atomone Routes = %d, want 1", len(atomone.Routes))
	}

	route := atomone.Routes[0]
	if route.ToChainID != "osmosis-1" {
		t.Errorf("route ToChainID = %q, want 'osmosis-1'", route.ToChainID)
	}
	if route.ChannelID != "channel-2" {
		t.Errorf("route ChannelID = %q, want 'channel-2'", route.ChannelID)
	}
	if route.ConnectionID != "connection-2" {
		t.Errorf("route ConnectionID = %q, want 'connection-2'", route.ConnectionID)
	}
	if route.CounterpartyChannelID != "channel-94814" {
		t.Errorf("route CounterpartyChannelID = %q, want 'channel-94814'", route.CounterpartyChannelID)
	}

	// Verify Osmosis has a route to AtomOne
	osmosis := reg.Chains["osmosis-1"]
	if len(osmosis.Routes) != 1 {
		t.Fatalf("osmosis Routes = %d, want 1", len(osmosis.Routes))
	}

	osmosisRoute := osmosis.Routes[0]
	if osmosisRoute.ToChainID != "atomone-1" {
		t.Errorf("osmosis route ToChainID = %q, want 'atomone-1'", osmosisRoute.ToChainID)
	}
	if osmosisRoute.ChannelID != "channel-94814" {
		t.Errorf("osmosis route ChannelID = %q, want 'channel-94814'", osmosisRoute.ChannelID)
	}
}

func TestRouteAllowedTokens(t *testing.T) {
	builder := NewBuilder()
	inputConfigs := createTestInputConfigs()
	ibcData := createTestIBCData()

	reg, err := builder.BuildRegistry(inputConfigs, ibcData)
	if err != nil {
		t.Fatalf("BuildRegistry() error = %v", err)
	}

	// Check AtomOne -> Osmosis route has tokens
	atomone := reg.Chains["atomone-1"]
	route := atomone.Routes[0]

	// Should have:
	// - 2 native tokens (uatone, uphoton) that can be sent to Osmosis
	// - 1 token from Osmosis (uosmo) that can be sent back
	if len(route.AllowedTokens) < 3 {
		t.Errorf("route AllowedTokens = %d, want at least 3", len(route.AllowedTokens))
	}

	// Check that uatone is in the allowed tokens
	foundAtone := false
	foundOsmoBack := false
	for _, token := range route.AllowedTokens {
		if token.BaseDenom == "uatone" && token.OriginChain == "atomone-1" {
			foundAtone = true
			if token.SourceDenom != "uatone" {
				t.Errorf("uatone SourceDenom = %q, want 'uatone'", token.SourceDenom)
			}
			// DestinationDenom should be an IBC hash
			if len(token.DestinationDenom) < 10 || token.DestinationDenom[:4] != "ibc/" {
				t.Errorf("uatone DestinationDenom should be IBC hash, got %q", token.DestinationDenom)
			}
		}
		if token.BaseDenom == "uosmo" && token.OriginChain == "osmosis-1" {
			foundOsmoBack = true
			// Source should be IBC denom of OSMO on AtomOne
			if token.SourceDenom[:4] != "ibc/" {
				t.Errorf("uosmo SourceDenom should be IBC hash, got %q", token.SourceDenom)
			}
			// Destination should be native uosmo
			if token.DestinationDenom != "uosmo" {
				t.Errorf("uosmo DestinationDenom = %q, want 'uosmo'", token.DestinationDenom)
			}
		}
	}

	if !foundAtone {
		t.Error("uatone not found in AllowedTokens")
	}
	if !foundOsmoBack {
		t.Error("uosmo (unwinding back to Osmosis) not found in AllowedTokens")
	}
}

func TestIBCTokensComputed(t *testing.T) {
	builder := NewBuilder()
	inputConfigs := createTestInputConfigs()
	ibcData := createTestIBCData()

	reg, err := builder.BuildRegistry(inputConfigs, ibcData)
	if err != nil {
		t.Fatalf("BuildRegistry() error = %v", err)
	}

	// AtomOne should have OSMO as an IBC token
	atomone := reg.Chains["atomone-1"]
	if len(atomone.IBCTokens) != 1 {
		t.Fatalf("atomone IBCTokens = %d, want 1", len(atomone.IBCTokens))
	}

	osmoIBC := atomone.IBCTokens[0]
	if osmoIBC.BaseDenom != "uosmo" {
		t.Errorf("IBC token BaseDenom = %q, want 'uosmo'", osmoIBC.BaseDenom)
	}
	if osmoIBC.OriginChain != "osmosis-1" {
		t.Errorf("IBC token OriginChain = %q, want 'osmosis-1'", osmoIBC.OriginChain)
	}
	if osmoIBC.IBCDenom[:4] != "ibc/" {
		t.Errorf("IBC token IBCDenom should be hash, got %q", osmoIBC.IBCDenom)
	}

	// Osmosis should have ATONE and PHOTON as IBC tokens
	osmosis := reg.Chains["osmosis-1"]
	if len(osmosis.IBCTokens) != 2 {
		t.Fatalf("osmosis IBCTokens = %d, want 2", len(osmosis.IBCTokens))
	}
}

func TestMultiHopReceivedToken(t *testing.T) {
	builder := NewBuilder()
	inputConfigs := createTestInputConfigsWithMultiHop()
	ibcData := createTestIBCDataWithStargaze()

	reg, err := builder.BuildRegistry(inputConfigs, ibcData)
	if err != nil {
		t.Fatalf("BuildRegistry() error = %v", err)
	}

	// AtomOne should have STARS as an IBC token (via Osmosis)
	atomone := reg.Chains["atomone-1"]

	// Should have OSMO (direct) + STARS (multi-hop)
	if len(atomone.IBCTokens) < 2 {
		t.Errorf("atomone IBCTokens = %d, want at least 2", len(atomone.IBCTokens))
	}

	foundStars := false
	for _, token := range atomone.IBCTokens {
		if token.BaseDenom == "ustars" {
			foundStars = true
			if token.OriginChain != "stargaze-1" {
				t.Errorf("STARS OriginChain = %q, want 'stargaze-1'", token.OriginChain)
			}
			if token.IBCDenom[:4] != "ibc/" {
				t.Errorf("STARS IBCDenom should be hash, got %q", token.IBCDenom)
			}
			t.Logf("Found STARS on AtomOne: %s", token.IBCDenom)
		}
	}

	if !foundStars {
		t.Error("STARS (multi-hop via Osmosis) not found on AtomOne")
	}
}

func TestStatusCheck(t *testing.T) {
	// Test that both "ACTIVE" and "live" status are accepted
	tests := []struct {
		status   string
		expected bool
	}{
		{"ACTIVE", true},
		{"active", true},
		{"LIVE", true},
		{"live", true},
		{"inactive", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			ibcData := []registry.ChainIbcData{
				{
					Chain1: registry.IbcChainData{ChainName: "atomone", ConnectionID: "conn-1"},
					Chain2: registry.IbcChainData{ChainName: "osmosis", ConnectionID: "conn-2"},
					Channels: []registry.IbcChannelData{
						{
							Chain1:   registry.ChannelChainData{ChannelID: "ch-1", PortID: "transfer"},
							Chain2:   registry.ChannelChainData{ChannelID: "ch-2", PortID: "transfer"},
							Ordering: "unordered",
							Version:  "ics20-1",
							Tags:     registry.ChannelTags{Preferred: true, Status: tt.status},
						},
					},
				},
			}

			builder := NewBuilder()
			inputConfigs := createTestInputConfigs()
			reg, err := builder.BuildRegistry(inputConfigs, ibcData)

			if err != nil {
				t.Fatalf("BuildRegistry() error = %v", err)
			}

			atomone := reg.Chains["atomone-1"]
			hasRoute := len(atomone.Routes) > 0

			if hasRoute != tt.expected {
				t.Errorf("status %q: hasRoute = %v, want %v", tt.status, hasRoute, tt.expected)
			}
		})
	}
}

func TestEmptyInputConfigs(t *testing.T) {
	builder := NewBuilder()
	_, err := builder.BuildRegistry(map[string]*input.ChainInput{}, nil)
	if err == nil {
		t.Error("BuildRegistry() should error with empty input configs")
	}
}

func TestTokenAllowedDestinations(t *testing.T) {
	// Create configs where a token is restricted to specific destinations
	configs := createTestInputConfigs()

	// Restrict PHOTON to only Osmosis (which is the only connection anyway)
	configs["atomone-1"].Tokens[1].AllowedDestinations = []string{"osmosis-1"}

	builder := NewBuilder()
	ibcData := createTestIBCData()

	reg, err := builder.BuildRegistry(configs, ibcData)
	if err != nil {
		t.Fatalf("BuildRegistry() error = %v", err)
	}

	atomone := reg.Chains["atomone-1"]
	route := atomone.Routes[0]

	// Both uatone and uphoton should be in AllowedTokens
	foundPhoton := false
	for _, token := range route.AllowedTokens {
		if token.BaseDenom == "uphoton" && token.OriginChain == "atomone-1" {
			foundPhoton = true
		}
	}

	if !foundPhoton {
		t.Error("uphoton should be allowed to osmosis-1")
	}
}
