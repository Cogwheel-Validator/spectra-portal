package router

import (
	"fmt"

	models "github.com/Cogwheel-Validator/spectra-portal/pathfinder/models"
)

// checkPFMSupport checks if all intermediate chains in the path support PFM
// For a path A -> B -> C, only B needs PFM support (the forwarding chain)
func (s *Pathfinder) checkPFMSupport(path []string) bool {
	if len(path) <= 2 {
		return false // No intermediate chains
	}

	// Check all intermediate chains (exclude first and last)
	for i := 1; i < len(path)-1; i++ {
		if !s.routeIndex.pfmChains[path[i]] {
			return false
		}
	}

	return true
}

// generatePFMMemo generates an IBC memo for PFM forwarding
// Format: {"forward":{"receiver":"<addr>","port":"transfer","channel":"<channel>"}}
// For multi-hop, we nest the forward messages
func (s *Pathfinder) generatePFMMemo(legs []*models.IBCLeg, finalReceiver string) string {
	if len(legs) == 0 {
		return ""
	}

	// Build memo from the last leg backwards
	var buildMemo func(legIndex int, receiver string) string
	buildMemo = func(legIndex int, receiver string) string {
		if legIndex >= len(legs) {
			return ""
		}

		leg := legs[legIndex]

		if legIndex == len(legs)-1 {
			// Last leg - use final receiver
			return fmt.Sprintf(`{"forward":{"receiver":"%s","port":"%s","channel":"%s"}}`,
				receiver, leg.Port, leg.Channel)
		}

		// Intermediate leg - nest the next memo
		nextMemo := buildMemo(legIndex+1, receiver)
		// For intermediate hops, the receiver should be the intermediate chain's module account
		// But in PFM, the memo itself handles forwarding, so we use a placeholder
		return fmt.Sprintf(`{"forward":{"receiver":"%s","port":"%s","channel":"%s","next":%s}}`,
			receiver, leg.Port, leg.Channel, nextMemo)
	}

	// Start from the first leg (after source chain)
	return buildMemo(1, finalReceiver)
}
