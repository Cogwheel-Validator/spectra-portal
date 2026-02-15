package osmosis

import (
	"os"
	"time"

	"github.com/rs/zerolog"

	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/brokers"
	ibcmemo "github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/ibc_memo"
	sqsquery "github.com/Cogwheel-Validator/spectra-portal/pathfinder/sqs_query"
)

var log zerolog.Logger

func init() {
	out := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}
	log = zerolog.New(out).With().Timestamp().Str("component", "osmosis-broker").Logger()
}

// SqsBroker implements brokers.BrokerClient for Osmosis using the SQS API
type SqsBroker struct {
	client               *sqsquery.SqsQueryClient
	memoBuilder          *MemoBuilder
	smartContractBuilder *SmartContractBuilder
}

// NewSqsBroker creates a new Osmosis SQS broker client with a single endpoint
func NewSqsBroker(sqsApiUrls []string, contractAddress string) *SqsBroker {
	return &SqsBroker{
		client:               sqsquery.NewSqsQueryClient(sqsApiUrls),
		memoBuilder:          NewMemoBuilder(contractAddress),
		smartContractBuilder: NewSmartContractBuilder(contractAddress),
	}
}

// NewSqsBrokerWithFailover creates a new Osmosis SQS broker client with failover support
func NewSqsBrokerWithFailover(sqsApiUrls []string, contractAddress string) *SqsBroker {
	return &SqsBroker{
		client:               sqsquery.NewSqsQueryClientWithFailover(sqsApiUrls, sqsquery.DefaultFailoverConfig()),
		memoBuilder:          NewMemoBuilder(contractAddress),
		smartContractBuilder: NewSmartContractBuilder(contractAddress),
	}
}

// QuerySwap implements brokers.BrokerClient interface for Osmosis SQS
func (o *SqsBroker) QuerySwap(
	tokenInDenom, tokenInAmount, tokenOutDenom string,
	singleRoute *bool,
) (*brokers.SwapResult, error) {
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
	if singleRoute == nil {
		singleRoute = new(bool)
		*singleRoute = false
	}
	response, err := o.client.GetRoute(tokenIn, nil, nil, &tokenOutDenom, *singleRoute)
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

	// Convert SQS response to typed RouteData
	routeData := ConvertSqsResponseToRouteData(response)

	// Convert SQS response to standardized SwapResult
	return &brokers.SwapResult{
		AmountIn:     response.AmountIn.Amount,
		AmountOut:    response.AmountOut,
		PriceImpact:  response.PriceImpact,
		EffectiveFee: response.EffectiveFee,
		RouteData:    routeData,
	}, nil
}

// GetBrokerType returns the broker type identifier
func (o *SqsBroker) GetBrokerType() string {
	return "osmosis-sqs"
}

// GetMemoBuilder returns the memo builder for Osmosis
func (o *SqsBroker) GetMemoBuilder() ibcmemo.MemoBuilder {
	return o.memoBuilder
}

// GetSmartContractBuilder returns the smart contract builder for Osmosis
func (o *SqsBroker) GetSmartContractBuilder() brokers.SmartContractBuilder {
	return o.smartContractBuilder
}

// Close cleans up resources used by the broker client
func (o *SqsBroker) Close() {
	if o.client != nil {
		o.client.Close()
	}
}
