package router

// RouteRequest - API POST body
type RouteRequest struct {
	ChainA         string `json:"chainA"`         // e.g., "juno"
	TokenInDenom   string `json:"tokenInDenom"`   // e.g., "ujuno"
	AmountIn       string `json:"amountIn"`       // e.g., "1000000"
	ChainB         string `json:"chainB"`         // e.g., "cosmoshub"
	TokenOutDenom  string `json:"tokenOutDenom"`  // e.g., "uatom"
	SenderAddress  string `json:"senderAddress"`  // For validation
	ReceiverAddress string `json:"receiverAddress"`
}


// DirectIBCExecution - everything frontend needs for simple IBC transfer
type DirectIBCExecution struct {
	SourceChannel      string `json:"sourceChannel"`      // e.g., "channel-0"
	SourcePort         string `json:"sourcePort"`         // e.g., "transfer"
	DestinationChannel string `json:"destinationChannel"`
	TokenDenom         string `json:"tokenDenom"`         // Send denom (ujuno)
	Amount             string `json:"amount"`             // Exact amount to send
	Receiver           string `json:"receiver"`           // Receiver address
	TimeoutHeight      string `json:"timeoutHeight,omitempty"`
	TimeoutTimestamp   uint64 `json:"timeoutTimestamp"`
	Memo               string `json:"memo,omitempty"`
}

// MultiHopExecution - step-by-step execution plan
type MultiHopExecution struct {
	Steps         []ExecutionStep `json:"steps"`
	TotalSteps    int             `json:"totalSteps"`
	EstimatedOut  string          `json:"estimatedOut"` // Expected final amount
	MinimumOut    string          `json:"minimumOut"`   // With slippage
	PriceImpact   float64         `json:"priceImpact"`
	Route         string          `json:"route"`        // Human readable: "JUNO -> ATOM via Osmosis"
}

// ExecutionStep - each step frontend must execute
type ExecutionStep struct {
	StepNumber  int                    `json:"stepNumber"`
	Type        string                 `json:"type"` // "ibc_transfer" | "osmosis_swap" | "wait_for_ibc"
	ChainId     string                 `json:"chainId"`
	Description string                 `json:"description"`
	Data        map[string]interface{} `json:"data"` // Step-specific data
}