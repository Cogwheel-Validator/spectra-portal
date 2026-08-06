//go:build e2e

package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	"connectrpc.com/connect"
	v2beta "github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/services/pathfinder/v2beta"
	"github.com/pelletier/go-toml/v2"
	"google.golang.org/protobuf/types/known/emptypb"
)

// BasicRoutes represents the data that is used for basic pathfinder routing
type BasicRoutes struct {
	FromChainId   string
	ToChainId     string
	FromDenom     string
	ExpectedDenom string
}

// TestCases represents the test cases for pathfinder e2e transfer tests.
type TestCases struct {
	Name           string `toml:"name"`
	ChainFrom      string `toml:"chain_from"`
	TokenFromDenom string `toml:"token_from_denom"`
	AmountIn       string `toml:"amount_in"`
	ChainTo        string `toml:"chain_to"`
	TokenToDenom   string `toml:"token_to_denom"`
	Type           string `toml:"type"`
}

// testCasesFile represents the top-level structure of the test_cases.toml file.
type testCasesFile struct {
	Case []TestCases `toml:"case"`
}

// loadBasicRoutes loads the basic routes for all supported chains. The returned
// routes are used for basic pathfinder e2e transfer tests.
func loadBasicRoutes(t *testing.T) []BasicRoutes {
	client := NewQueryClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(5*time.Second))
	defer cancel()
	chainList, err := client.ListSupportedChains(ctx, connect.NewRequest(&emptypb.Empty{}))
	if err != nil {
		t.Fatal(err)
	}

	var basicRoutes []BasicRoutes
	for _, chain := range chainList.Msg.ChainIds {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(5*time.Second))
		defer cancel()
		chainData, err := client.GetChainInfo(ctx, connect.NewRequest(&v2beta.GetChainInfoRequest{ChainId: chain}))
		if err != nil {
			t.Fatal(err)
		}
		addRoutes(&basicRoutes, chainData.Msg)
	}
	return basicRoutes
}

// addRoutes adds the basic routes for a given chain to the basicRoutes slice.
func addRoutes(basicRoutes *[]BasicRoutes, data *v2beta.GetChainInfoResponse) {
	chainInfo := data.ChainInfo
	chainId := chainInfo.ChainId
	routes := chainInfo.Routes
	for _, route := range routes {
		if len(route.AllowedTokens) > 0 {
			for _, token := range route.AllowedTokens {
				bRoute := BasicRoutes{
					FromChainId:   chainId,
					ToChainId:     route.ToChainId,
					FromDenom:     token.ChainDenom,
					ExpectedDenom: token.CounterpartDenom,
				}
				*basicRoutes = append(*basicRoutes, bRoute)
			}
		}
	}
}

// testAddress represents a single entry in the addresses.toml file.
type testAddress struct {
	ChainId string `toml:"chain_id"`
	Address string `toml:"address"`
}

// addressesFile represents the top-level structure of the addresses.toml file.
type addressesFile struct {
	TestAddress []testAddress `toml:"test_address"`
}

// loadAddresses loads the addresses from the addresses.toml file.
func loadAddresses(t *testing.T) map[string]string {
	addressesToml, err := os.ReadFile("../../e2e/addresses.toml")
	if err != nil {
		t.Fatal(err)
	}
	var parsed addressesFile
	if err := toml.Unmarshal(addressesToml, &parsed); err != nil {
		t.Fatal(err)
	}
	addressesMap := make(map[string]string, len(parsed.TestAddress))
	for _, a := range parsed.TestAddress {
		addressesMap[a.ChainId] = a.Address
	}
	return addressesMap
}

// loadTestCases loads the test cases from the test_cases.toml file.
func loadTestCases(t *testing.T) (
	brokerSwap []TestCases,
	multiHop []TestCases,
	multiHopSwap []TestCases,
) {
	testCasesToml, err := os.ReadFile("../../e2e/test_cases.toml")
	if err != nil {
		t.Fatal(err)
	}
	var parsed testCasesFile
	if err := toml.Unmarshal(testCasesToml, &parsed); err != nil {
		t.Fatal(err)
	}
	for _, cs := range parsed.Case {
		switch cs.Type {
		case "broker_swap":
			brokerSwap = append(brokerSwap, cs)
		case "multihop":
			multiHop = append(multiHop, cs)
		case "multihop_swap":
			multiHopSwap = append(multiHopSwap, cs)
		}
	}
	return brokerSwap, multiHop, multiHopSwap
}
