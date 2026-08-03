package router

import (
	"fmt"

	models "github.com/Cogwheel-Validator/spectra-portal/pathfinder/models"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/brokers"
	ibcmemo "github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/ibc_memo"
)

// minOutputWithSlippage applies the request's slippage tolerance to the broker's
// quoted output amount. On failure it logs and falls back to the unadjusted amount
// so the route stays usable.
func minOutputWithSlippage(req models.RouteRequest, swapResult *brokers.SwapResult) string {
	minOutput := swapResult.AmountOut
	if req.SlippageBps != nil {
		calculated, calcErr := brokers.CalculateMinOutput(swapResult.AmountOut, *req.SlippageBps)
		if calcErr != nil {
			// If for some reason it does fail at least try to return some value
			pathfinderLog.Warn().Err(calcErr).Msg("Failed to calculate min output, using original amount")
		} else {
			minOutput = calculated
		}
	}
	return minOutput
}

// tokenInDenomOnBroker returns the denom of the inbound token once it has landed
// on the broker chain after all inbound IBC transfers.
func tokenInDenomOnBroker(hopInfo *MultiHopInfo) string {
	if len(hopInfo.InboundIntermediateTokens) > 0 {
		// Multi-hop: use the IBC denom from the last intermediate token
		lastIntToken := hopInfo.InboundIntermediateTokens[len(hopInfo.InboundIntermediateTokens)-1]
		return lastIntToken.IbcDenom
	}
	// Single hop: use the direct IBC denom
	return hopInfo.TokenIn.IbcDenom
}

// buildSwapOnlyExecution builds execution data for swap-only routes (destination is broker)
// Supports both single-hop and multi-hop inbound paths
func (s *Pathfinder) buildSwapOnlyExecution(
	req models.RouteRequest,
	hopInfo *MultiHopInfo,
	swapResult *brokers.SwapResult,
	memoBuilder ibcmemo.MemoBuilder,
	brokerExists bool,
	resolved *ResolvedAddresses,
) (*models.BrokerExecutionData, error) {
	if !brokerExists {
		return nil, fmt.Errorf("broker chain %s not found", hopInfo.BrokerChainId)
	}
	// Check if there is IBC hook, only needed for now but when more "Broker Chains" are added
	// some of these checks will need to be modified
	contractAddress := memoBuilder.GetContractAddress()
	if contractAddress == "" {
		return nil, fmt.Errorf("ibc-hooks contract not configured for broker %s", hopInfo.BrokerChainId)
	}

	brokerAddr := resolved.On(hopInfo.BrokerChainId, RoleSender)
	destAddr := resolved.On(req.ChainTo, RoleReceiver)

	minOutput := minOutputWithSlippage(req, swapResult)
	tokenInDenom := tokenInDenomOnBroker(hopInfo)

	var memo string
	var err error

	// Check if we need multi-hop inbound (Forward + Swap)
	if len(hopInfo.InboundRoutes) > 1 {
		inboundHops := s.buildInboundHops(hopInfo, resolved)

		if len(hopInfo.InboundRoutes) == 2 {
			// Two inbound hops (source -> intermediate -> broker): use HopAndSwap memo.
			// First leg is signed by user; memo contains full nested forward (outer: to intermediate, inner: to broker + wasm).
			memo, err = memoBuilder.BuildHopAndSwapMemo(ibcmemo.HopAndSwapParams{
				InboundHops: inboundHops,
				SwapParams: ibcmemo.SwapAndForwardParams{
					SwapMemoParams: ibcmemo.SwapMemoParams{
						TokenInDenom:     tokenInDenom,
						TokenOutDenom:    hopInfo.TokenOutOnBroker.ChainDenom,
						MinOutputAmount:  minOutput,
						RouteData:        swapResult.RouteData,
						TimeoutTimestamp: ibcmemo.DefaultTimeoutTimestamp(),
						RecoverAddress:   brokerAddr,
						ReceiverAddress:  destAddr,
					},
					SourceChannel:   "",
					ForwardReceiver: destAddr,
					ForwardMemo:     "",
				},
			})
			if err != nil {
				return nil, fmt.Errorf("failed to build hop-and-swap memo: %w", err)
			}
		} else {
			// More than two inbound hops: build Forward + Swap memo (memo describes remaining hops only).
			memoInboundHops := inboundHops[1:]
			memo, err = memoBuilder.BuildForwardSwapMemo(ibcmemo.ForwardSwapParams{
				InboundHops: memoInboundHops,
				SwapParams: ibcmemo.SwapAndForwardParams{
					SwapMemoParams: ibcmemo.SwapMemoParams{
						TokenInDenom:     tokenInDenom,
						TokenOutDenom:    hopInfo.TokenOutOnBroker.ChainDenom,
						MinOutputAmount:  minOutput,
						RouteData:        swapResult.RouteData,
						TimeoutTimestamp: ibcmemo.DefaultTimeoutTimestamp(),
						RecoverAddress:   brokerAddr,
						ReceiverAddress:  destAddr,
					},
					SourceChannel:   "",
					ForwardReceiver: destAddr,
					ForwardMemo:     "",
				},
			})
			if err != nil {
				return nil, fmt.Errorf("failed to build forward+swap memo: %w", err)
			}
		}
	} else {
		// Single-hop inbound: build simple Swap memo
		memo, err = memoBuilder.BuildSwapMemo(ibcmemo.SwapMemoParams{
			TokenInDenom:     tokenInDenom,
			TokenOutDenom:    hopInfo.TokenOutOnBroker.ChainDenom,
			MinOutputAmount:  minOutput,
			RouteData:        swapResult.RouteData,
			TimeoutTimestamp: ibcmemo.DefaultTimeoutTimestamp(),
			RecoverAddress:   brokerAddr,
			ReceiverAddress:  destAddr, // For swap-only, receiver is on broker chain
		})
		if err != nil {
			return nil, fmt.Errorf("failed to build wasm memo: %w", err)
		}
	}

	// For 2-hop inbound (hop-and-swap), first transfer goes to intermediate chain; receiver is PFM
	ibcReceiver := contractAddress
	if len(hopInfo.InboundRoutes) == 2 {
		chainId := hopInfo.InboundRoutes[0].ToChainId
		addr, ok := resolved.Lookup(chainId, RoleSender)
		if !ok {
			return nil, fmt.Errorf("no address available for intermediate chain %s", chainId)
		}
		ibcReceiver = addr
	}

	return &models.BrokerExecutionData{
		Memo:            &memo,
		IBCReceiver:     &ibcReceiver,
		RecoverAddress:  &brokerAddr,
		MinOutputAmount: minOutput,
		UsesWasm:        true,
		Description:     fmt.Sprintf("IBC transfer with swap on %s", hopInfo.BrokerChainId),
	}, nil
}

// buildSwapAndForwardExecution builds execution data for swap+forward routes
// Supports both single-hop and multi-hop inbound/outbound via nested PFM memos
func (s *Pathfinder) buildSwapAndForwardExecution(
	req models.RouteRequest,
	hopInfo *MultiHopInfo,
	swapResult *brokers.SwapResult,
	outboundLegs []*models.IBCLeg,
	memoBuilder ibcmemo.MemoBuilder,
	brokerExists bool,
	resolved *ResolvedAddresses,
) (*models.BrokerExecutionData, error) {
	contractAddress := memoBuilder.GetContractAddress()
	if !brokerExists || contractAddress == "" {
		return nil, fmt.Errorf("ibc-hooks contract not configured for broker")
	}

	if len(outboundLegs) == 0 {
		return nil, fmt.Errorf("no outbound legs provided")
	}

	brokerAddr := resolved.On(hopInfo.BrokerChainId, RoleSender)
	destAddr := resolved.On(req.ChainTo, RoleReceiver)

	minOutput := minOutputWithSlippage(req, swapResult)
	tokenInDenom := tokenInDenomOnBroker(hopInfo)

	// Determine the receiver for the first IBC transfer (after swap)
	// If there are more hops, receiver should be on the intermediate chain
	firstHopReceiver := destAddr
	if len(outboundLegs) > 1 {
		// For multi-hop, the receiver is on the first intermediate chain
		intermediateChain := outboundLegs[0].ToChain
		if intermediateAddr, ok := resolved.Lookup(intermediateChain, RoleReceiver); ok {
			firstHopReceiver = intermediateAddr
		} else {
			// Fallback to destination address (PFM will use it anyway)
			pathfinderLog.Warn().Str("chain", intermediateChain).Msg("No address for intermediate chain, using destination address")
		}
	}

	// Build the memo based on inbound and outbound hop counts
	var memo string
	var err error
	hasMultiHopInbound := len(hopInfo.InboundRoutes) > 1
	hasMultiHopOutbound := len(outboundLegs) > 1

	if hasMultiHopInbound {
		// Multi-hop inbound: use ForwardSwap or ForwardSwapForward.
		// The memo is attached to the first IBC transfer (source → first intermediate).
		// It must describe only the remaining hops (first intermediate → broker → ...).
		inboundHops := s.buildInboundHops(hopInfo, resolved)
		memoInboundHops := inboundHops[1:]

		if hasMultiHopOutbound {
			// Forward + Swap + MultiHop Forward (case 5.4)
			outboundHops := s.buildOutboundHops(outboundLegs, destAddr, resolved)

			memo, err = memoBuilder.BuildForwardSwapForwardMemo(ibcmemo.ForwardSwapForwardParams{
				InboundHops: memoInboundHops,
				SwapParams: ibcmemo.SwapAndMultiHopParams{
					SwapMemoParams: ibcmemo.SwapMemoParams{
						TokenInDenom:     tokenInDenom,
						TokenOutDenom:    hopInfo.TokenOutOnBroker.ChainDenom,
						MinOutputAmount:  minOutput,
						RouteData:        swapResult.RouteData,
						TimeoutTimestamp: ibcmemo.DefaultTimeoutTimestamp(),
						RecoverAddress:   brokerAddr,
						ReceiverAddress:  firstHopReceiver,
					},
					OutboundHops:  outboundHops,
					FinalReceiver: destAddr,
				},
			})
		} else {
			// Forward + Swap + Single Forward (case 5.2)
			memo, err = memoBuilder.BuildForwardSwapMemo(ibcmemo.ForwardSwapParams{
				InboundHops: memoInboundHops,
				SwapParams: ibcmemo.SwapAndForwardParams{
					SwapMemoParams: ibcmemo.SwapMemoParams{
						TokenInDenom:     tokenInDenom,
						TokenOutDenom:    hopInfo.TokenOutOnBroker.ChainDenom,
						MinOutputAmount:  minOutput,
						RouteData:        swapResult.RouteData,
						TimeoutTimestamp: ibcmemo.DefaultTimeoutTimestamp(),
						RecoverAddress:   brokerAddr,
						ReceiverAddress:  firstHopReceiver,
					},
					SourceChannel:   outboundLegs[0].Channel,
					ForwardReceiver: firstHopReceiver,
					ForwardMemo:     "",
				},
			})
		}
	} else {
		// Single-hop inbound
		if hasMultiHopOutbound {
			// Swap + MultiHop Forward (case 5.3)
			outboundHops := s.buildOutboundHops(outboundLegs, destAddr, resolved)

			memo, err = memoBuilder.BuildSwapAndMultiHopMemo(ibcmemo.SwapAndMultiHopParams{
				SwapMemoParams: ibcmemo.SwapMemoParams{
					TokenInDenom:     tokenInDenom,
					TokenOutDenom:    hopInfo.TokenOutOnBroker.ChainDenom,
					MinOutputAmount:  minOutput,
					RouteData:        swapResult.RouteData,
					TimeoutTimestamp: ibcmemo.DefaultTimeoutTimestamp(),
					RecoverAddress:   brokerAddr,
					ReceiverAddress:  firstHopReceiver,
				},
				OutboundHops:  outboundHops,
				FinalReceiver: destAddr,
			})
		} else {
			// Swap + Single Forward (case 5.1)
			memo, err = memoBuilder.BuildSwapAndForwardMemo(ibcmemo.SwapAndForwardParams{
				SwapMemoParams: ibcmemo.SwapMemoParams{
					TokenInDenom:     tokenInDenom,
					TokenOutDenom:    hopInfo.TokenOutOnBroker.ChainDenom,
					MinOutputAmount:  minOutput,
					RouteData:        swapResult.RouteData,
					TimeoutTimestamp: ibcmemo.DefaultTimeoutTimestamp(),
					RecoverAddress:   brokerAddr,
					ReceiverAddress:  firstHopReceiver,
				},
				SourceChannel:   outboundLegs[0].Channel,
				ForwardReceiver: firstHopReceiver,
				ForwardMemo:     "",
			})
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to build wasm memo: %w", err)
	}

	// Build description
	description := fmt.Sprintf("IBC transfer with swap on %s", hopInfo.BrokerChainId)
	if hasMultiHopInbound {
		description = fmt.Sprintf("Multi-hop IBC (%d hops) with swap on %s", len(hopInfo.InboundRoutes), hopInfo.BrokerChainId)
	}
	if len(outboundLegs) == 1 {
		description += fmt.Sprintf(" and forward to %s", req.ChainTo)
	} else {
		description += fmt.Sprintf(" and forward via %d hops to %s", len(outboundLegs), req.ChainTo)
	}

	return &models.BrokerExecutionData{
		Memo:            &memo,
		IBCReceiver:     &contractAddress,
		RecoverAddress:  &brokerAddr,
		MinOutputAmount: minOutput,
		UsesWasm:        true,
		Description:     description,
	}, nil
}

// buildSmartContractSwapExecution builds execution data for same-chain swap (source == broker == dest)
// Returns smart contract data instead of IBC memo since no IBC transfer is needed.
func (s *Pathfinder) buildSmartContractSwapExecution(
	req models.RouteRequest,
	hopInfo *MultiHopInfo,
	swapResult *brokers.SwapResult,
	scBuilder brokers.SmartContractBuilder,
	brokerExists bool,
	resolved *ResolvedAddresses,
) (*models.BrokerExecutionData, error) {
	if !brokerExists {
		return nil, fmt.Errorf("broker chain %s not found", hopInfo.BrokerChainId)
	}

	minOutput := minOutputWithSlippage(req, swapResult)

	// Build smart contract data for same-chain swap
	scData, err := scBuilder.BuildSwapAndTransfer(ibcmemo.SwapMemoParams{
		TokenInDenom:     hopInfo.TokenIn.ChainDenom, // Native denom since source is broker
		TokenOutDenom:    hopInfo.TokenOutOnBroker.ChainDenom,
		MinOutputAmount:  minOutput,
		RouteData:        swapResult.RouteData,
		TimeoutTimestamp: ibcmemo.DefaultTimeoutTimestamp(),
		RecoverAddress:   resolved.On(req.ChainFrom, RoleSender), // On same chain, use sender as recover
		ReceiverAddress:  resolved.On(req.ChainTo, RoleReceiver), // Final destination on same chain
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build smart contract data: %w", err)
	}

	return &models.BrokerExecutionData{
		SmartContractData: scData,
		MinOutputAmount:   minOutput,
		UsesWasm:          true,
		Description:       fmt.Sprintf("Smart contract swap on %s", hopInfo.BrokerChainId),
	}, nil
}

// buildSmartContractSwapAndForwardExecution builds execution data for broker-as-source routes
// (source == broker, dest != broker). Returns smart contract data with IBC forward built-in.
func (s *Pathfinder) buildSmartContractSwapAndForwardExecution(
	req models.RouteRequest,
	hopInfo *MultiHopInfo,
	swapResult *brokers.SwapResult,
	outboundLegs []*models.IBCLeg,
	scBuilder brokers.SmartContractBuilder,
	brokerExists bool,
	resolved *ResolvedAddresses,
) (*models.BrokerExecutionData, error) {
	if !brokerExists {
		return nil, fmt.Errorf("broker chain %s not found", hopInfo.BrokerChainId)
	}
	if len(outboundLegs) == 0 {
		return nil, fmt.Errorf("no outbound legs for swap-and-forward")
	}

	senderAddr := resolved.On(req.ChainFrom, RoleSender)
	destAddr := resolved.On(req.ChainTo, RoleReceiver)

	minOutput := minOutputWithSlippage(req, swapResult)

	// For single outbound hop, use simple swap+forward
	// For multi-hop, we'd need PFM memo in the forward action
	firstLeg := outboundLegs[0]
	var forwardMemo string
	if len(outboundLegs) > 1 {
		// Build PFM memo for remaining hops
		forwardMemo = s.generatePFMMemo(outboundLegs[1:], destAddr)
	}

	// Build smart contract data for swap + IBC forward
	scData, err := scBuilder.BuildSwapAndForward(ibcmemo.SwapAndForwardParams{
		SwapMemoParams: ibcmemo.SwapMemoParams{
			TokenInDenom:     hopInfo.TokenIn.ChainDenom, // Native denom since source is broker
			TokenOutDenom:    hopInfo.TokenOutOnBroker.ChainDenom,
			MinOutputAmount:  minOutput,
			RouteData:        swapResult.RouteData,
			TimeoutTimestamp: ibcmemo.DefaultTimeoutTimestamp(),
			RecoverAddress:   senderAddr, // On broker, use sender as recover
			ReceiverAddress:  destAddr,   // Used for forward action
		},
		SourceChannel:   firstLeg.Channel,
		ForwardReceiver: destAddr,
		ForwardMemo:     forwardMemo,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build smart contract data: %w", err)
	}

	description := fmt.Sprintf("Smart contract swap on %s then IBC to %s", hopInfo.BrokerChainId, req.ChainTo)
	if len(outboundLegs) > 1 {
		description = fmt.Sprintf("Smart contract swap on %s then IBC forward via %d hops", hopInfo.BrokerChainId, len(outboundLegs))
	}

	return &models.BrokerExecutionData{
		SmartContractData: scData,
		MinOutputAmount:   minOutput,
		UsesWasm:          true,
		Description:       description,
	}, nil
}

// buildInboundHops converts inbound routes to IBCHop slice for memo building.
// For intermediate hops, Receiver is set to the address on that hop's destination chain.
// The last hop's receiver is left empty; the memo builder uses the broker contract
// address for it.
func (s *Pathfinder) buildInboundHops(hopInfo *MultiHopInfo, resolved *ResolvedAddresses) []ibcmemo.IBCHop {
	hops := make([]ibcmemo.IBCHop, len(hopInfo.InboundRoutes))
	for i, route := range hopInfo.InboundRoutes {
		receiver := ""
		if i < len(hopInfo.InboundRoutes)-1 {
			// Intermediate hop: receiver address on the hop's destination chain
			if addr, ok := resolved.Lookup(route.ToChainId, RoleReceiver); ok {
				receiver = addr
			}
		}
		hops[i] = ibcmemo.IBCHop{
			Channel:  route.ChannelId,
			Port:     route.PortId,
			Receiver: receiver,
			Timeout:  ibcmemo.DefaultTimeoutTimestamp(),
		}
	}
	return hops
}

// buildOutboundHops converts outbound legs to IBCHop slice for memo building
func (s *Pathfinder) buildOutboundHops(outboundLegs []*models.IBCLeg, finalReceiver string, resolved *ResolvedAddresses) []ibcmemo.IBCHop {
	hops := make([]ibcmemo.IBCHop, len(outboundLegs))
	for i, leg := range outboundLegs {
		receiver := finalReceiver
		if i < len(outboundLegs)-1 {
			// Intermediate hop - receiver address on the next chain
			nextChain := outboundLegs[i+1].FromChain
			if nextAddr, ok := resolved.Lookup(nextChain, RoleReceiver); ok {
				receiver = nextAddr
			}
		}
		hops[i] = ibcmemo.IBCHop{
			Channel:  leg.Channel,
			Port:     leg.Port,
			Receiver: receiver,
			Timeout:  ibcmemo.DefaultTimeoutTimestamp(),
		}
	}
	return hops
}

// executionAddressNeeds declares the addresses the execution builder for the
// route's shape will consume, so they can be resolved (or reported as
// missing/required) in one place before any memo is built.
func executionAddressNeeds(req models.RouteRequest, hopInfo *MultiHopInfo, outboundLegs []*models.IBCLeg) []AddressNeed {
	needs := []AddressNeed{
		{ChainID: req.ChainFrom, Role: RoleSender, Required: true},
		{ChainID: req.ChainTo, Role: RoleReceiver, Required: true},
	}

	if !hopInfo.SourceIsBroker {
		// The recover address lives on the broker chain and follows the
		// sender's account.
		needs = append(needs, AddressNeed{ChainID: hopInfo.BrokerChainId, Role: RoleSender, Required: true})

		// Inbound intermediate hop receivers (multi-hop inbound memos).
		for i, route := range hopInfo.InboundRoutes {
			if i < len(hopInfo.InboundRoutes)-1 {
				needs = append(needs, AddressNeed{ChainID: route.ToChainId, Role: RoleReceiver, Required: false})
			}
		}
		// For 2-hop inbound the first transfer's receiver is the sender's
		// account on the intermediate chain.
		if len(hopInfo.InboundRoutes) == 2 {
			needs = append(needs, AddressNeed{ChainID: hopInfo.InboundRoutes[0].ToChainId, Role: RoleSender, Required: true})
		}
	}

	// Outbound intermediate hop receivers (first-hop receiver and multi-hop
	// outbound memos). The final leg's destination is req.ChainTo, already
	// covered above.
	for i, leg := range outboundLegs {
		if i < len(outboundLegs)-1 {
			needs = append(needs, AddressNeed{ChainID: leg.ToChain, Role: RoleReceiver, Required: false})
		}
	}

	return needs
}
