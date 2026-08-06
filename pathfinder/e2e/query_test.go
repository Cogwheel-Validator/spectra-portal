//go:build e2e

package e2e

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/zeebo/assert"
	"google.golang.org/protobuf/types/known/emptypb"
)

// TestListSupportedChains is a cheap connectivity smoke test: if this
// fails, the server isn't reachable and the heavier FindPath suites won't
// tell you anything useful either.
func TestListSupportedChains(t *testing.T) {
	client := NewQueryClient(t)

	resp, err := client.ListSupportedChains(context.Background(), connect.NewRequest(&emptypb.Empty{}))
	assert.NoError(t, err)
	assert.True(t, len(resp.Msg.GetChainIds()) > 0)
}
