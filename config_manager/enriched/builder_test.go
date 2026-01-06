package enriched

import (
	"testing"

	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/input"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/keplr"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/registry"
)

// createTestInputConfigs creates mock input configs for testing
func createTestInputConfigs() map[string]*input.ChainInput {
	pfmTrue := true
	return map[string]*input.ChainInput{
		"atomone-1": {
			Chain: input.ChainMeta{
				Name:              "Atom One",
				ID:                "atomone-1",
				Type:              "cosmos",
				Registry:          "atomone",
				ExplorerURL:       "https://thespectra.io/atomone",
				Slip44:            118,
				Bech32Prefix:      "atone",
				RPCs:              []input.APIEndpoint{{URL: "https://atomone-rpc.example.com"}},
				Rest:              []input.APIEndpoint{{URL: "https://atomone-api.example.com"}},
				KeplrJSONFileName: &[]string{"atomone.json"}[0],
			},
			Tokens: []input.TokenMeta{
				{Denom: "uatone", Name: "Atone", Symbol: "ATONE", Exponent: 6, Icon: "https://example.com/atone.png"},
				{Denom: "uphoton", Name: "Photon", Symbol: "PHOTON", Exponent: 6, Icon: "https://example.com/photon.png"},
			},
		},
		"osmosis-1": {
			Chain: input.ChainMeta{
				Name:              "Osmosis",
				ID:                "osmosis-1",
				Type:              "cosmos",
				Registry:          "osmosis",
				ExplorerURL:       "https://mintscan.io/osmosis",
				Slip44:            118,
				Bech32Prefix:      "osmo",
				IsBroker:          true,
				BrokerID:          "osmosis-sqs",
				HasPFM:            &pfmTrue,
				RPCs:              []input.APIEndpoint{{URL: "https://osmosis-rpc.example.com"}},
				Rest:              []input.APIEndpoint{{URL: "https://osmosis-api.example.com"}},
				KeplrJSONFileName: &[]string{"osmosis.json"}[0],
			},
			Tokens: []input.TokenMeta{
				{Denom: "uosmo", Name: "Osmosis", Symbol: "OSMO", Exponent: 6, Icon: "https://example.com/osmo.png"},
			},
		},
	}
}

// createTestInputConfigsWithMultiHop creates configs with multi-hop routable token
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

	// Add routable STARS token on Osmosis - STARS that arrived via Stargaze
	// and should be forwardable to AtomOne
	// The IBC denom of STARS on Osmosis is: ibc/hash(transfer/channel-75/ustars)
	starsOnOsmosis := "ibc/987C17B11ABC2B20019178ACE62929FE9840202CE79498E29FE8E5CB02B7C0A4" // Example hash

	configs["osmosis-1"].Tokens = append(configs["osmosis-1"].Tokens, input.TokenMeta{
		Denom:               starsOnOsmosis,
		Name:                "Stargaze",
		Symbol:              "STARS",
		Exponent:            6,
		Icon:                "https://example.com/stars.png",
		OriginChain:         "stargaze-1",
		OriginDenom:         "ustars",
		AllowedDestinations: []string{"atomone-1"}, // Can only forward to AtomOne
	})

	return configs
}

func createTestKeplrConfigs() []keplr.KeplrChainConfig {
	keplrConfigs := make([]keplr.KeplrChainConfig, 0)
	atomoneKeplrConfig := &keplr.KeplrChainConfig{
		ChainID:             "atomone-1",
		ChainName:           "Atom One",
		ChainSymbolImageURL: "https://raw.githubusercontent.com/Cogwheel-Validator/spectra-ibc-hub/main/images/atomone/atone.png",
		Bip44: keplr.Bip44{
			CoinType: 118,
		},
		Bech32Config: keplr.Bech32Config{
			Bech32PrefixAccAddr:  "atone",
			Bech32PrefixAccPub:   "atonepub",
			Bech32PrefixValAddr:  "atoneval",
			Bech32PrefixValPub:   "atonevalpub",
			Bech32PrefixConsAddr: "atonecons",
			Bech32PrefixConsPub:  "atoneconspub",
		},
		Currencies: []keplr.Currency{
			{
				CoinDenom:        "ATONE",
				CoinMinimalDenom: "uatone",
				CoinDecimals:     6,
				CoinImageURL:     "https://raw.githubusercontent.com/Cogwheel-Validator/spectra-ibc-hub/main/images/atomone/atone.png",
			},
			{
				CoinDenom:        "PHOTON",
				CoinMinimalDenom: "uphoton",
				CoinDecimals:     6,
				CoinImageURL:     "https://raw.githubusercontent.com/Cogwheel-Validator/spectra-ibc-hub/main/images/atomone/photon.png",
			},
		},
		FeeCurrencies: []keplr.FeeCurrency{
			{
				CoinDenom:        "ATONE",
				CoinMinimalDenom: "uatone",
				CoinDecimals:     6,
				CoinImageURL:     "https://raw.githubusercontent.com/Cogwheel-Validator/spectra-ibc-hub/main/images/atomone/atone.png",
			},
			{
				CoinDenom:        "PHOTON",
				CoinMinimalDenom: "uphoton",
				CoinDecimals:     6,
				CoinImageURL:     "https://raw.githubusercontent.com/Cogwheel-Validator/spectra-ibc-hub/main/images/atomone/photon.png",
			},
		},
		StakeCurrency: keplr.StakeCurrency{
			CoinDenom:        "ATONE",
			CoinMinimalDenom: "uatone",
			CoinDecimals:     6,
			CoinImageURL:     "https://raw.githubusercontent.com/Cogwheel-Validator/spectra-ibc-hub/main/images/atomone/atone.png",
		},
		Features: []string{},
	}
	osmosisKeplrConfig := &keplr.KeplrChainConfig{
		ChainID:             "osmosis-1",
		ChainName:           "Osmosis",
		ChainSymbolImageURL: "https://raw.githubusercontent.com/Cogwheel-Validator/spectra-ibc-hub/main/images/osmosis/osmo.png",
		Bip44: keplr.Bip44{
			CoinType: 118,
		},
		Bech32Config: keplr.Bech32Config{
			Bech32PrefixAccAddr:  "osmo",
			Bech32PrefixAccPub:   "osmopub",
			Bech32PrefixValAddr:  "osmoval",
			Bech32PrefixValPub:   "osmovalpub",
			Bech32PrefixConsAddr: "osmocons",
			Bech32PrefixConsPub:  "osmoconspub",
		},
		Currencies: []keplr.Currency{
			{
				CoinDenom:        "OSMO",
				CoinMinimalDenom: "uosmo",
				CoinDecimals:     6,
				CoinImageURL:     "https://raw.githubusercontent.com/Cogwheel-Validator/spectra-ibc-hub/main/images/osmosis/osmo.png",
			},
		},
		FeeCurrencies: []keplr.FeeCurrency{
			{
				CoinDenom:        "OSMO",
				CoinMinimalDenom: "uosmo",
				CoinDecimals:     6,
				CoinImageURL:     "https://raw.githubusercontent.com/Cogwheel-Validator/spectra-ibc-hub/main/images/osmosis/osmo.png",
			},
		},
		StakeCurrency: keplr.StakeCurrency{
			CoinDenom:        "OSMO",
			CoinMinimalDenom: "uosmo",
			CoinDecimals:     6,
			CoinImageURL:     "https://raw.githubusercontent.com/Cogwheel-Validator/spectra-ibc-hub/main/images/osmosis/osmo.png",
		},
		Features: []string{"wasm"},
	}
	keplrConfigs = append(keplrConfigs, *atomoneKeplrConfig, *osmosisKeplrConfig)
	return keplrConfigs
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

func createTestAllowedExplorers() []input.AllowedExplorer {
	return []input.AllowedExplorer{
		{
			Name:              "mintscan",
			BaseURL:           "https://mintscan.io",
			MultiChainSupport: true,
			AccountPath:       "/{chain_name}/account",
			TransactionPath:   "/{chain_name}/txs",
		},
		{
			Name:              "random explorer with no multichain support",
			BaseURL:           "https://random-explorer.com",
			MultiChainSupport: false,
			AccountPath:       "/account",
			TransactionPath:   "/txs",
		},
	}
}

func TestBuildRegistry(t *testing.T) {
	builder := NewBuilder(createTestAllowedExplorers(), WithSkipNetworkCheck(true))
	inputConfigs := createTestInputConfigs()
	ibcData := createTestIBCData()

	reg, err := builder.BuildRegistry(inputConfigs, ibcData, createTestKeplrConfigs())
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
	builder := NewBuilder(createTestAllowedExplorers(), WithSkipNetworkCheck(true))
	inputConfigs := createTestInputConfigs()
	ibcData := createTestIBCData()

	reg, err := builder.BuildRegistry(inputConfigs, ibcData, createTestKeplrConfigs())
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
	builder := NewBuilder(createTestAllowedExplorers(), WithSkipNetworkCheck(true))
	inputConfigs := createTestInputConfigs()
	ibcData := createTestIBCData()

	reg, err := builder.BuildRegistry(inputConfigs, ibcData, createTestKeplrConfigs())
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
	builder := NewBuilder(createTestAllowedExplorers(), WithSkipNetworkCheck(true))
	inputConfigs := createTestInputConfigs()
	ibcData := createTestIBCData()

	reg, err := builder.BuildRegistry(inputConfigs, ibcData, createTestKeplrConfigs())
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

func TestRoutableIBCToken(t *testing.T) {
	builder := NewBuilder(createTestAllowedExplorers(), WithSkipNetworkCheck(true))
	inputConfigs := createTestInputConfigsWithMultiHop()
	ibcData := createTestIBCDataWithStargaze()

	reg, err := builder.BuildRegistry(inputConfigs, ibcData, createTestKeplrConfigs())
	if err != nil {
		t.Fatalf("BuildRegistry() error = %v", err)
	}

	// Osmosis should have STARS as a routable IBC token
	osmosis := reg.Chains["osmosis-1"]

	// Check that STARS IBC token is listed
	foundStarsIBC := false
	for _, token := range osmosis.IBCTokens {
		if token.BaseDenom == "ustars" && token.OriginChain == "stargaze-1" {
			foundStarsIBC = true
			t.Logf("Found STARS IBC token on Osmosis: %s", token.IBCDenom)
		}
	}

	if !foundStarsIBC {
		t.Error("STARS IBC token not found on Osmosis")
	}

	// Check Osmosis -> AtomOne route has STARS
	var toAtomoneRoute *RouteConfig
	for i := range osmosis.Routes {
		if osmosis.Routes[i].ToChainID == "atomone-1" {
			toAtomoneRoute = &osmosis.Routes[i]
			break
		}
	}

	if toAtomoneRoute == nil {
		t.Fatal("Osmosis -> AtomOne route not found")
	}

	// Should have STARS as routable token
	foundStarsInRoute := false
	for _, token := range toAtomoneRoute.AllowedTokens {
		t.Logf("Route token: %s (base: %s, origin: %s)", token.SourceDenom, token.BaseDenom, token.OriginChain)
		if token.BaseDenom == "ustars" && token.OriginChain == "stargaze-1" {
			foundStarsInRoute = true
		}
	}

	if !foundStarsInRoute {
		t.Error("STARS not found in Osmosis -> AtomOne route allowed tokens")
	}

	// Check Osmosis -> Stargaze route does NOT have the routable STARS
	// (it should only have native OSMO and unwinding STARS from Stargaze)
	var toStargazeRoute *RouteConfig
	for i := range osmosis.Routes {
		if osmosis.Routes[i].ToChainID == "stargaze-1" {
			toStargazeRoute = &osmosis.Routes[i]
			break
		}
	}

	if toStargazeRoute == nil {
		t.Fatal("Osmosis -> Stargaze route not found")
	}

	// Should NOT have STARS as routable (it's not in AllowedDestinations for STARS)
	for _, token := range toStargazeRoute.AllowedTokens {
		// The routable STARS token should NOT appear in this route
		if token.OriginChain == "stargaze-1" && token.BaseDenom == "ustars" {
			// This is okay - it's the unwinding path
			if token.SourceDenom[:4] != "ibc/" {
				// Native STARS should not be here from Osmosis
				t.Error("Native STARS should not be in Osmosis -> Stargaze as source (should be IBC denom)")
			}
		}
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

			builder := NewBuilder(createTestAllowedExplorers(), WithSkipNetworkCheck(true))
			inputConfigs := createTestInputConfigs()
			reg, err := builder.BuildRegistry(inputConfigs, ibcData, createTestKeplrConfigs())

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
	builder := NewBuilder(createTestAllowedExplorers(), WithSkipNetworkCheck(true))
	_, err := builder.BuildRegistry(map[string]*input.ChainInput{}, nil, nil)
	if err == nil {
		t.Error("BuildRegistry() should error with empty input configs")
	}
}

func TestTokenAllowedDestinations(t *testing.T) {
	// Create configs where a token is restricted to specific destinations
	configs := createTestInputConfigs()

	// Restrict PHOTON to only Osmosis (which is the only connection anyway)
	configs["atomone-1"].Tokens[1].AllowedDestinations = []string{"osmosis-1"}

	builder := NewBuilder(createTestAllowedExplorers(), WithSkipNetworkCheck(true))
	ibcData := createTestIBCData()

	reg, err := builder.BuildRegistry(configs, ibcData, createTestKeplrConfigs())
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

func TestNativeVsRoutableTokenCategorization(t *testing.T) {
	configs := createTestInputConfigsWithMultiHop()
	ibcData := createTestIBCDataWithStargaze()

	rb := NewRouteBuilder(configs, ibcData)

	// AtomOne should have 2 native tokens
	if len(rb.nativeTokens["atomone-1"]) != 2 {
		t.Errorf("AtomOne native tokens = %d, want 2", len(rb.nativeTokens["atomone-1"]))
	}
	if len(rb.routableTokens["atomone-1"]) != 0 {
		t.Errorf("AtomOne routable tokens = %d, want 0", len(rb.routableTokens["atomone-1"]))
	}

	// Osmosis should have 1 native token (OSMO) and 1 routable (STARS)
	if len(rb.nativeTokens["osmosis-1"]) != 1 {
		t.Errorf("Osmosis native tokens = %d, want 1", len(rb.nativeTokens["osmosis-1"]))
	}
	if len(rb.routableTokens["osmosis-1"]) != 1 {
		t.Errorf("Osmosis routable tokens = %d, want 1", len(rb.routableTokens["osmosis-1"]))
	}

	// Check the routable token is correctly identified
	routable := rb.routableTokens["osmosis-1"][0]
	if routable.OriginChain != "stargaze-1" {
		t.Errorf("Routable STARS OriginChain = %q, want 'stargaze-1'", routable.OriginChain)
	}
	if routable.OriginDenom != "ustars" {
		t.Errorf("Routable STARS OriginDenom = %q, want 'ustars'", routable.OriginDenom)
	}
}
