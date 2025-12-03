package router

import (
	sqsquery "github.com/Cogwheel-Validator/spectra-ibc-hub/solver/sqs_query"
)

// This is an adapter for any Osmosis based broker that implements the BrokerClient interface

// OsmosisSqsBroker implements BrokerClient for Osmosis using the SQS API
type OsmosisSqsBroker struct {
	client *sqsquery.SqsQueryClient
}

// NewOsmosisSqsBroker creates a new Osmosis SQS broker client
func NewOsmosisSqsBroker(sqsApiUrl string) *OsmosisSqsBroker {
	return &OsmosisSqsBroker{
		client: sqsquery.NewSqsQueryClient(sqsApiUrl),
	}
}

// QuerySwap implements BrokerClient interface for Osmosis SQS
func (o *OsmosisSqsBroker) QuerySwap(tokenInDenom, tokenInAmount, tokenOutDenom string) (*SwapResult, error) {
	// Create the token input with amount and denom
	tokenIn := &sqsquery.TokenRequest{
		Denom:  tokenInDenom,
		Amount: tokenInAmount,
	}

	// Query SQS for the route
	// singleRoute=false allows for split routes across multiple pools
	response, err := o.client.GetRoute(tokenIn, nil, nil, &tokenOutDenom, false)
	if err != nil {
		return nil, err
	}

	// Convert SQS response to typed OsmosisRouteData
	routeData := ConvertSqsResponseToRouteData(response)

	// Convert SQS response to standardized SwapResult
	return &SwapResult{
		AmountIn:     response.AmountIn.Amount,
		AmountOut:    response.AmountOut,
		PriceImpact:  response.PriceImpact,
		EffectiveFee: response.EffectiveFee,
		RouteData:    routeData,
	}, nil
}

// GetBrokerType returns the broker type identifier
func (o *OsmosisSqsBroker) GetBrokerType() string {
	return "osmosis"
}
