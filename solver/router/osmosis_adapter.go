package router

import (
	sqsquery "github.com/Cogwheel-Validator/spectra-ibc-hub/solver/sqs_query"
)

// OsmosisRouteData is a Go struct that matches the proto definition
// This allows us to maintain type safety while still using interface{} in SwapResult
type OsmosisRouteData struct {
	Routes               []OsmosisRoute `json:"routes"`
	LiquidityCap         string         `json:"liquidity_cap"`
	LiquidityCapOverflow bool           `json:"liquidity_cap_overflow"`
}

type OsmosisRoute struct {
	Pools     []OsmosisPool `json:"pools"`
	HasCwPool bool          `json:"has_cw_pool"`
	OutAmount string        `json:"out_amount"`
	InAmount  string        `json:"in_amount"`
}

type OsmosisPool struct {
	ID            int32    `json:"id"`
	Type          int32    `json:"type"`
	SpreadFactor  string `json:"spread_factor"`
	TokenOutDenom string `json:"token_out_denom"`
	TakerFee      string `json:"taker_fee"`
	LiquidityCap  string `json:"liquidity_cap"`
}

// ConvertSqsResponseToRouteData converts the SQS API response to typed OsmosisRouteData
func ConvertSqsResponseToRouteData(sqsResponse sqsquery.RouteTokenResponse) *OsmosisRouteData {
	routes := make([]OsmosisRoute, 0, len(sqsResponse.Route))
	
	for _, sqsRoute := range sqsResponse.Route {
		pools := make([]OsmosisPool, 0, len(sqsRoute.Pools))
		
		for _, sqsPool := range sqsRoute.Pools {
			pools = append(pools, OsmosisPool{
				ID:            sqsPool.ID,
				Type:          sqsPool.Type,
				SpreadFactor:  sqsPool.SpreadFactor,
				TokenOutDenom: sqsPool.TokenOutDenom,
				TakerFee:      sqsPool.TakerFee,
				LiquidityCap:  sqsPool.LiquidityCap,
			})
		}
		
		routes = append(routes, OsmosisRoute{
			Pools:     pools,
			HasCwPool: sqsRoute.HasCwPool,
			OutAmount: sqsRoute.OutAmount,
			InAmount:  sqsRoute.InAmount,
		})
	}
	
	return &OsmosisRouteData{
		Routes:               routes,
		LiquidityCap:         sqsResponse.LiquidityCap,
		LiquidityCapOverflow: sqsResponse.LiquidityCapOverflow,
	}
}

