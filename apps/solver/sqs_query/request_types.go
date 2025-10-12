package sqsquery

// This is the token in the request GetRoute
type TokenRequest struct {
	Denom  string `json:"denom"`
	Amount string `json:"amount"`
}