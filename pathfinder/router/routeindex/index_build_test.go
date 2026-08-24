package routeindex

import (
	"testing"

	"github.com/zeebo/assert"
)

func TestRouteKey(t *testing.T) {
	assert.Equal(t, routeKey("chain-a", "chain-b", "uatom"), "chain-a->chain-b:uatom")
}

func TestBuildIndex_EmptyChains(t *testing.T) {
	ri := NewRouteIndex()
	assert.Error(t, ri.BuildIndex(nil))
	assert.Error(t, ri.BuildIndex([]PathfinderChain{}))
}
