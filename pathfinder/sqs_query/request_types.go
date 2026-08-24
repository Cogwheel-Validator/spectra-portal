package sqsquery

// TokenRequest is the token in the GetRoute request
type TokenRequest struct {
	Denom  string `json:"denom"`
	Amount string `json:"amount"`
}
