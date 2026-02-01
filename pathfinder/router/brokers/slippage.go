package brokers

import (
	"fmt"
	"strconv"
)

// calculateMinOutputInternal calculates minimum output with slippage tolerance.
// slippageBps is basis points (e.g., 100 = 1%)
// minOutput = expected * (10000 - slippageBps) / 10000
func calculateMinOutputInternal(expectedOutput string, slippageBps uint32) (string, error) {
	// Parse the expected output
	expected, err := strconv.ParseInt(expectedOutput, 10, 64)
	if err != nil {
		return "", fmt.Errorf("failed to parse expected output: %w", err)
	}

	// Calculate minimum with slippage
	minOutput := expected * int64(10000-slippageBps) / 10000

	return strconv.FormatInt(minOutput, 10), nil
}
