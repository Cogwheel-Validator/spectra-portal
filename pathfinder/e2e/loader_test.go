//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"connectrpc.com/connect"
	v2beta "github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/services/pathfinder/v2beta"
	v2betaconnect "github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/services/pathfinder/v2beta/v2betaconnect"
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
func loadBasicRoutes() ([]BasicRoutes, error) {
	httpClient := &http.Client{Timeout: 30 * time.Second}
	client := v2betaconnect.NewPathfinderQueryServiceClient(httpClient, BaseURL())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	chainList, err := client.ListSupportedChains(ctx, connect.NewRequest(&emptypb.Empty{}))
	if err != nil {
		return nil, fmt.Errorf("list supported chains: %w", err)
	}

	var basicRoutes []BasicRoutes
	for _, chain := range chainList.Msg.ChainIds {
		chainData, err := func() (*connect.Response[v2beta.GetChainInfoResponse], error) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return client.GetChainInfo(ctx, connect.NewRequest(&v2beta.GetChainInfoRequest{ChainId: chain}))
		}()
		if err != nil {
			return nil, fmt.Errorf("get chain info for %q: %w", chain, err)
		}
		addRoutes(&basicRoutes, chainData.Msg)
	}
	return basicRoutes, nil
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
func loadAddresses() (map[string]string, error) {
	addressesToml, err := os.ReadFile("../../e2e/addresses.toml")
	if err != nil {
		return nil, fmt.Errorf("read addresses.toml: %w", err)
	}
	var parsed addressesFile
	if err := toml.Unmarshal(addressesToml, &parsed); err != nil {
		return nil, fmt.Errorf("parse addresses.toml: %w", err)
	}
	addressesMap := make(map[string]string, len(parsed.TestAddress))
	for _, a := range parsed.TestAddress {
		addressesMap[a.ChainId] = a.Address
	}
	return addressesMap, nil
}

// loadTestCases loads the test cases from the test_cases.toml file.
func loadTestCases() (
	brokerSwap []TestCases,
	multiHop []TestCases,
	multiHopSwap []TestCases,
	err error,
) {
	testCasesToml, err := os.ReadFile("../../e2e/test_cases.toml")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read test_cases.toml: %w", err)
	}
	var parsed testCasesFile
	if err := toml.Unmarshal(testCasesToml, &parsed); err != nil {
		return nil, nil, nil, fmt.Errorf("parse test_cases.toml: %w", err)
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
	return brokerSwap, multiHop, multiHopSwap, nil
}
