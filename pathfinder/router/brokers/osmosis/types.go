// Package osmosis provides Osmosis-specific implementations for the broker interface.
package osmosis

import (
	"strconv"

	ibcmemo "github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/router/ibc_memo"
	sqsquery "github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/sqs_query"
)

const (
	// SwapVenueName is the swap venue identifier for Osmosis poolmanager
	SwapVenueName = "osmosis-poolmanager"
)

// RouteData contains Osmosis-specific routing information.
// Implements the ibcmemo.RouteData interface.
type RouteData struct {
	Routes               []Route `json:"routes"`
	LiquidityCap         string  `json:"liquidity_cap"`
	LiquidityCapOverflow bool    `json:"liquidity_cap_overflow"`
}

// Route represents a single swap route on Osmosis
type Route struct {
	Pools     []Pool `json:"pools"`
	HasCwPool bool   `json:"has_cw_pool"`
	OutAmount string `json:"out_amount"`
	InAmount  string `json:"in_amount"`
}

// Pool represents a single pool in a swap route
type Pool struct {
	ID            int32  `json:"id"`
	Type          int32  `json:"type"`
	SpreadFactor  string `json:"spread_factor"`
	TokenOutDenom string `json:"token_out_denom"`
	TakerFee      string `json:"taker_fee"`
	LiquidityCap  string `json:"liquidity_cap"`
}

// GetOperations implements ibcmemo.RouteData interface
// Converts the Osmosis route data to swap operations
func (r *RouteData) GetOperations() []ibcmemo.SwapOperation {
	if len(r.Routes) == 0 {
		return nil
	}

	// Use the first route (best route from SQS)
	route := r.Routes[0]
	if len(route.Pools) == 0 {
		return nil
	}

	return nil // Caller needs to provide tokenInDenom, use GetOperationsWithInput
}

// GetOperationsWithInput converts route data to swap operations with the given input denom
func (r *RouteData) GetOperationsWithInput(tokenInDenom string) []ibcmemo.SwapOperation {
	if len(r.Routes) == 0 || len(r.Routes[0].Pools) == 0 {
		return nil
	}

	// This assumes the request made to the pathfinder used the singe route set to true
	// TODO: address this part somehow
	route := r.Routes[0]
	operations := make([]ibcmemo.SwapOperation, len(route.Pools))
	currentDenomIn := tokenInDenom

	for i, pool := range route.Pools {
		operations[i] = ibcmemo.SwapOperation{
			Pool:     strconv.Itoa(int(pool.ID)),
			DenomIn:  currentDenomIn,
			DenomOut: pool.TokenOutDenom,
		}
		// The output of this pool is the input of the next
		currentDenomIn = pool.TokenOutDenom
	}

	return operations
}

// GetSwapVenueName implements ibcmemo.RouteData interface
func (r *RouteData) GetSwapVenueName() string {
	return SwapVenueName
}

// ConvertSqsResponseToRouteData converts the SQS API response to typed RouteData
func ConvertSqsResponseToRouteData(sqsResponse sqsquery.RouteTokenResponse) *RouteData {
	routes := make([]Route, 0, len(sqsResponse.Route))

	for _, sqsRoute := range sqsResponse.Route {
		pools := make([]Pool, 0, len(sqsRoute.Pools))

		for _, sqsPool := range sqsRoute.Pools {
			pools = append(pools, Pool{
				ID:            sqsPool.ID,
				Type:          sqsPool.Type,
				SpreadFactor:  sqsPool.SpreadFactor,
				TokenOutDenom: sqsPool.TokenOutDenom,
				TakerFee:      sqsPool.TakerFee,
				LiquidityCap:  sqsPool.LiquidityCap,
			})
		}

		routes = append(routes, Route{
			Pools:     pools,
			HasCwPool: sqsRoute.HasCwPool,
			OutAmount: sqsRoute.OutAmount,
			InAmount:  sqsRoute.InAmount,
		})
	}

	return &RouteData{
		Routes:               routes,
		LiquidityCap:         sqsResponse.LiquidityCap,
		LiquidityCapOverflow: sqsResponse.LiquidityCapOverflow,
	}
}
