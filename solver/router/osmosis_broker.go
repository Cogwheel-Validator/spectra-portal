package router

import (
	"os"
	"time"

	"github.com/rs/zerolog"

	sqsquery "github.com/Cogwheel-Validator/spectra-ibc-hub/solver/sqs_query"
)

var log zerolog.Logger

func init() {
	out := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}
	log = zerolog.New(out).With().Timestamp().Str("component", "osmosis-broker").Logger()
}

// This is an adapter for any Osmosis based broker that implements the BrokerClient interface

// OsmosisSqsBroker implements BrokerClient for Osmosis using the SQS API
type OsmosisSqsBroker struct {
	client *sqsquery.SqsQueryClient
}

// NewOsmosisSqsBroker creates a new Osmosis SQS broker client with a single endpoint
func NewOsmosisSqsBroker(sqsApiUrl string) *OsmosisSqsBroker {
	return &OsmosisSqsBroker{
		client: sqsquery.NewSqsQueryClient(sqsApiUrl),
	}
}

// NewOsmosisSqsBrokerWithFailover creates a new Osmosis SQS broker client with failover support
func NewOsmosisSqsBrokerWithFailover(primaryURL string, backupURLs []string) *OsmosisSqsBroker {
	return &OsmosisSqsBroker{
		client: sqsquery.NewSqsQueryClientWithFailover(primaryURL, backupURLs, sqsquery.DefaultFailoverConfig()),
	}
}

// QuerySwap implements BrokerClient interface for Osmosis SQS
func (o *OsmosisSqsBroker) QuerySwap(tokenInDenom, tokenInAmount, tokenOutDenom string) (*SwapResult, error) {
	log.Debug().
		Str("tokenIn", tokenInDenom).
		Str("amount", tokenInAmount).
		Str("tokenOut", tokenOutDenom).
		Msg("Querying SQS for swap route")

	// Create the token input with amount and denom
	tokenIn := &sqsquery.TokenRequest{
		Denom:  tokenInDenom,
		Amount: tokenInAmount,
	}

	// Query SQS for the route
	// singleRoute=false allows for split routes across multiple pools
	response, err := o.client.GetRoute(tokenIn, nil, nil, &tokenOutDenom, false)
	if err != nil {
		log.Error().Err(err).
			Str("tokenIn", tokenInDenom).
			Str("tokenOut", tokenOutDenom).
			Msg("SQS query failed")
		return nil, err
	}

	log.Debug().
		Str("amountIn", response.AmountIn.Amount).
		Str("amountOut", response.AmountOut).
		Str("priceImpact", response.PriceImpact).
		Msg("SQS query successful")

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
	return "osmosis-sqs"
}

// Close cleans up resources used by the broker client
func (o *OsmosisSqsBroker) Close() {
	if o.client != nil {
		o.client.Close()
	}
}
