package brokers

import (
	"testing"

	"github.com/zeebo/assert"
)

func TestCalculateMinOutput(t *testing.T) {
	testCases := []struct {
		name        string
		expected    string
		slippageBps uint32
		want        string
	}{
		{name: "1% slippage", expected: "1000000", slippageBps: 100, want: "990000"},
		{name: "1% slippage odd amount", expected: "980000", slippageBps: 100, want: "970200"},
		{name: "0.5% slippage", expected: "1000000", slippageBps: 50, want: "995000"},
		{name: "zero slippage", expected: "1000000", slippageBps: 0, want: "1000000"},
		{name: "full slippage", expected: "1000000", slippageBps: 10000, want: "0"},
		{name: "integer division truncates", expected: "999", slippageBps: 50, want: "994"},
		{name: "small amount", expected: "1", slippageBps: 100, want: "0"},
		{name: "zero amount", expected: "0", slippageBps: 100, want: "0"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CalculateMinOutput(tc.expected, tc.slippageBps)
			assert.NoError(t, err)
			assert.Equal(t, got, tc.want)
		})
	}
}

func TestCalculateMinOutput_InvalidInput(t *testing.T) {
	_, err := CalculateMinOutput("not-a-number", 100)
	assert.Error(t, err)

	_, err = CalculateMinOutput("", 100)
	assert.Error(t, err)

	_, err = CalculateMinOutput("1.5", 100)
	assert.Error(t, err)

	// Larger than int64
	_, err = CalculateMinOutput("99999999999999999999999999", 100)
	assert.Error(t, err)
}
