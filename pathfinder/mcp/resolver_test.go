package mcp

import (
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/denomresolver"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/routeindex"
)

func newResolver(chains []routeindex.PathfinderChain, idx *routeindex.RouteIndex) *denomresolver.DenomResolver {
	r := denomresolver.NewDenomResolver(idx)
	r.SetChains(chains)
	return r
}
