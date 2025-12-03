package sqsquery

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/shopspring/decimal"
)

type SqsQueryClient struct {
	httpClient *http.Client
	baseURL    string
}

func NewSqsQueryClient(apiUrl string) *SqsQueryClient {
	// Validate the URL
	_, err := url.Parse(apiUrl)
	if err != nil {
		log.Fatalf("Failed to parse API URL: %v", err)
		return nil
	}
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}
	return &SqsQueryClient{
		httpClient: httpClient,
		baseURL:    apiUrl,
	}
}

/*
Returns the best quote it can compute for the exact in or exact out token swap method.

For exact amount in swap method, the tokenIn and tokenOutDenom are required.
For exact amount out swap method, the tokenOut and tokenInDenom are required.
Mixing swap method parameters in other way than specified will result in an error.

When singleRoute parameter is set to true, it gives the best single quote while excluding splits.

No 2 methods can be used together.
So when using this query, you can only use one of the following parameters:
- tokenIn and tokenOutDenom
- tokenOut and tokenInDenom
*/
func (c *SqsQueryClient) GetRoute(
	tokenIn, tokenOut *TokenRequest,
	tokenInDenom, tokenOutDenom *string,
	singleRoute bool) (RouteTokenResponse, error) {
	// check if the tokenIn and tokenOut are not nil
	// there must be at least one of them not nil
	if tokenIn == nil && tokenOut == nil {
		return RouteTokenResponse{}, errors.New("tokenIn and tokenOut cannot be nil")
	}
	if tokenIn != nil && tokenOut != nil {
		return RouteTokenResponse{}, errors.New("tokenIn and tokenOut cannot be used together")
	}

	if tokenInDenom == nil && tokenOutDenom == nil {
		return RouteTokenResponse{}, errors.New("tokenInDenom cannot be nil")
	}
	if tokenInDenom != nil && tokenOutDenom != nil {
		return RouteTokenResponse{}, errors.New("tokenInDenom and tokenOutDenom cannot be used together")
	}

	var newTokenInDenom string
	var newTokenOutDenom string
	var newTokenIn string
	var newTokenOut string

	if tokenInDenom != nil {
		newTokenInDenom = *tokenInDenom
	}
	if tokenOutDenom != nil {
		newTokenOutDenom = *tokenOutDenom
	}
	if tokenIn != nil {
		newTokenIn = string(tokenIn.Amount + tokenIn.Denom)
	}
	if tokenOut != nil {
		newTokenOut = string(tokenOut.Amount + tokenOut.Denom)
	}

	var tokenInParam string
	var tokenOutParam string
	var tokenInDenomParam string
	var tokenOutDenomParam string

	var fullURL string

	if tokenIn != nil && tokenOutDenom != nil {
		tokenInParam = "tokenIn=" + url.QueryEscape(newTokenIn)
		tokenOutDenomParam = "tokenOutDenom=" + url.QueryEscape(newTokenOutDenom)
		fullURL = fmt.Sprintf(
			"%s/route?%s&%s&singleRoute=%t&humanDenoms=false&applyExponents=false&appendBaseFee=true",
			c.baseURL, tokenInParam, tokenOutDenomParam, singleRoute,
		)
	} else if tokenOut != nil && tokenInDenom != nil {
		tokenOutParam = "tokenOut=" + url.QueryEscape(newTokenOut)
		tokenInDenomParam = "tokenInDenom=" + url.QueryEscape(newTokenInDenom)
		fullURL = fmt.Sprintf(
			"%s/route?%s&%s&singleRoute=%t&humanDenoms=false&applyExponents=false&appendBaseFee=true",
			c.baseURL, tokenOutParam, tokenInDenomParam, singleRoute,
		)
	} else {
		return RouteTokenResponse{}, errors.New("invalid parameters")
	}

	resp, err := c.httpClient.Get(fullURL)
	if err != nil {
		return RouteTokenResponse{}, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Failed to close response body: %v", err)
		}
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return RouteTokenResponse{}, err
	}
	var routeTokenResponse RouteTokenResponse
	err = json.Unmarshal(body, &routeTokenResponse)
	if err != nil {
		return RouteTokenResponse{}, err
	}
	return routeTokenResponse, nil
}

func (c *SqsQueryClient) GetTokenPrice(tokenDenom string) (decimal.Decimal, error) {
	fullURL := fmt.Sprintf("%s/token-price?tokenDenom=%s", c.baseURL, url.QueryEscape(tokenDenom))
	resp, err := c.httpClient.Get(fullURL)
	if err != nil {
		return decimal.Decimal{}, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("failed to close response body: %v", err)
		}
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return decimal.Decimal{}, err
	}

	/*The json is in very strange format from which I can't create a type
	it usually contains:
	{
		"ibc/denom_inserted": {
			"ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4": "2.001"
		}
	}
	The denom is kinda the key here, the second value is probably USDC, however I haven't tested
	this more, it could probably be even something like USDT on some pairs and there are some other
	variations of ibc denoms for stablecoins.
	So just collect the string and turn it into a decimal.Decimal
	*/
	var tokenPriceResponse map[string]any
	err = json.Unmarshal(body, &tokenPriceResponse)
	if err != nil {
		return decimal.Decimal{}, err
	}

	tokenPrice, ok := tokenPriceResponse[tokenDenom].(map[string]string)
	if !ok {
		return decimal.Decimal{}, fmt.Errorf("token price error when unmarshalling json, %s", tokenPriceResponse)
	}
	for _, price := range tokenPrice {
		priceDecimal, err := decimal.NewFromString(price)
		if err != nil {
			return decimal.Decimal{}, err
		}
		return priceDecimal, nil
	}

	return decimal.Decimal{}, errors.New("token price not found")
}

func (c *SqsQueryClient) GetAllPossibleRoutes(tokenInDenom, tokenOutDenom string) (AllPossibleRoutesResponse, error) {
	fullURL := fmt.Sprintf(
		"%s/router/routes?tokenInDenom=%s&tokenOutDenom=%s",
		c.baseURL, url.QueryEscape(tokenInDenom), url.QueryEscape(tokenOutDenom),
	)
	resp, err := c.httpClient.Get(fullURL)
	if err != nil {
		return AllPossibleRoutesResponse{}, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("failed to close response body: %v", err)
		}
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return AllPossibleRoutesResponse{}, err
	}
	var allPossibleRoutesResponse AllPossibleRoutesResponse
	err = json.Unmarshal(body, &allPossibleRoutesResponse)
	if err != nil {
		return AllPossibleRoutesResponse{}, err
	}
	return allPossibleRoutesResponse, nil
}
