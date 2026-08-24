package sqsquery

// RouteTokenResponse is the response from the SQS /router/quote endpoint.
type RouteTokenResponse struct {
	AmountIn struct {
		Denom  string `json:"denom"`
		Amount string `json:"amount"`
	} `json:"amount_in"`
	AmountOut               string  `json:"amount_out"`
	Route                   []Route `json:"route"`
	Tokens                  []Token `json:"tokens"`
	LiquidityCap            string  `json:"liquidity_cap"`
	LiquidityCapOverflow    bool    `json:"liquidity_cap_overflow"`
	EffectiveFee            string  `json:"effective_fee"`
	PriceImpact             string  `json:"price_impact"`
	InBaseOutQuoteSpotPrice string  `json:"in_base_out_quote_spot_price"`
}

// Route is a single swap route within a RouteTokenResponse.
type Route struct {
	Pools     []Pool `json:"pools"`
	HasCwPool bool   `json:"has-cw-pool"`
	OutAmount string `json:"out_amount"`
	InAmount  string `json:"in_amount"`
}

// Pool describes a single pool hop within a Route.
type Pool struct {
	ID            int32  `json:"id"`
	Type          int32  `json:"type"`
	Balances      []any  `json:"balances"`
	SpreadFactor  string `json:"spread_factor"`
	TokenOutDenom string `json:"token_out_denom"`
	TakerFee      string `json:"taker_fee"`
	LiquidityCap  string `json:"liquidity_cap"`
}

// Token describes a token referenced in a RouteTokenResponse.
type Token struct {
	Denom        string `json:"denom"`
	LiquidityCap string `json:"liquidity_cap"`
}

// AllPossibleRoutesResponse is the response from the SQS /router/routes endpoint.
type AllPossibleRoutesResponse struct {
	Routes []AllRoutes `json:"Routes"`
	// from here it returns an array of pool stirng ints with empty interface
	// example
	// UniquePoolIDs:
	// 	"1": {},
	// 	"2": {},
	// 	"3": {},
	// 	"4": {}...
	UniquePoolIDs              any  `json:"UniquePoolIDs"`
	ContainsCanonicalOrderbook bool `json:"ContainsCanonicalOrderbook"`
}

// AllRoutes is a single route entry within an AllPossibleRoutesResponse.
type AllRoutes struct {
	Pools            []AllPools `json:"Pools"`
	IsCanonicalOrder bool       `json:"IsCanonicalOrder"`
}

// AllPools describes a single pool hop within an AllRoutes entry.
type AllPools struct {
	ID            int    `json:"ID"`
	TokenInDenom  string `json:"TokenInDenom"`
	TokenOutDenom string `json:"TokenOutDenom"`
}
