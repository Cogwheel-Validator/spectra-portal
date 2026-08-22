package v2betahandlers

import (
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/models"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/brokers/osmosis"
	ibcmemo "github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/ibc_memo"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router/routeindex"
	v2beta "github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/services/pathfinder/v2beta"
)

// CONVERT FUNCTIONS
// These convert between internal models and protobuf types

/*
Converts internal models.RouteResponse to v2beta.FindPathResponse
It uses the protobuf oneof to represent the different route types.

Mock responses (route found from an empty addresses array) are flagged with
RESPONSE_CODE_MOCK_ADDRESSES; the router has already stripped their execution
data and memos. RequiredChains lists every chain that needs an address entry
in a real request.

Parameters:
- resp: *models.RouteResponse

Returns:
- *v2beta.FindPathResponse

Errors:
- None
*/
func convertToProtoResponse(resp *models.RouteResponse) *v2beta.FindPathResponse {
	protoResp := &v2beta.FindPathResponse{
		Success:        resp.Success,
		ErrorMessage:   resp.ErrorMessage,
		RequiredChains: resp.RequiredChains,
	}

	if resp.Success {
		if resp.Mock {
			protoResp.ResponseCode = v2beta.ResponseCode_RESPONSE_CODE_MOCK_ADDRESSES
		} else {
			protoResp.ResponseCode = v2beta.ResponseCode_RESPONSE_CODE_OK
		}
	}

	// Convert Direct route if present (using protobuf oneof)
	if resp.Direct != nil {
		protoResp.Route = &v2beta.FindPathResponse_Direct{
			Direct: convertToProtoDirectRoute(resp.Direct),
		}
	}

	// Convert Indirect route if present (using protobuf oneof)
	if resp.Indirect != nil {
		protoResp.Route = &v2beta.FindPathResponse_Indirect{
			Indirect: convertToProtoIndirectRoute(resp.Indirect),
		}
	}

	// Convert BrokerSwap route if present (using protobuf oneof)
	if resp.BrokerSwap != nil {
		protoResp.Route = &v2beta.FindPathResponse_BrokerSwap{
			BrokerSwap: convertToProtoBrokerSwapRoute(resp.BrokerSwap),
		}
	}

	return protoResp
}

// convertToProtoStreamingResponse converts internal models.RouteResponse to
// v2beta.FindPathStreamingResponse. The streaming variant always carries real
// addresses, so it has no mock/required-chains fields.
func convertToProtoStreamingResponse(resp *models.RouteResponse) *v2beta.FindPathStreamingResponse {
	protoResp := &v2beta.FindPathStreamingResponse{
		Success:      resp.Success,
		ErrorMessage: resp.ErrorMessage,
	}

	if resp.Direct != nil {
		protoResp.Route = &v2beta.FindPathStreamingResponse_Direct{
			Direct: convertToProtoDirectRoute(resp.Direct),
		}
	}

	if resp.Indirect != nil {
		protoResp.Route = &v2beta.FindPathStreamingResponse_Indirect{
			Indirect: convertToProtoIndirectRoute(resp.Indirect),
		}
	}

	if resp.BrokerSwap != nil {
		protoResp.Route = &v2beta.FindPathStreamingResponse_BrokerSwap{
			BrokerSwap: convertToProtoBrokerSwapRoute(resp.BrokerSwap),
		}
	}

	return protoResp
}

/*
Converts internal models.DirectRoute to v2beta.DirectRoute

Parameters:
- direct: *models.DirectRoute

Returns:
- *v2beta.DirectRoute

Errors:
- None
*/
func convertToProtoDirectRoute(direct *models.DirectRoute) *v2beta.DirectRoute {
	var transfer *v2beta.IBCLeg
	if direct.Transfer != nil {
		legs := convertToProtoIBCLeg([]*models.IBCLeg{direct.Transfer})
		if len(legs) > 0 {
			transfer = legs[0]
		}
	}
	return &v2beta.DirectRoute{
		Transfer: transfer,
	}
}

/*
Converts internal models.IndirectRoute to v2beta.IndirectRoute

Parameters:
- indirect: *models.IndirectRoute

Returns:
- *v2beta.IndirectRoute

Errors:
- None
*/
func convertToProtoIndirectRoute(indirect *models.IndirectRoute) *v2beta.IndirectRoute {
	legs := convertToProtoIBCLeg(indirect.Legs)
	return &v2beta.IndirectRoute{
		Path:          indirect.Path,
		Legs:          legs,
		SupportsPfm:   indirect.SupportsPFM,
		PfmStartChain: indirect.PFMStartChain,
		PfmMemo:       indirect.PFMMemo,
	}
}

/*
Converts internal models.BrokerRoute to v2beta.BrokerSwapRoute

Parameters:
- brokerSwap: *models.BrokerRoute

Returns:
- *v2beta.BrokerSwapRoute

Errors:
- None
*/
func convertToProtoBrokerSwapRoute(brokerSwap *models.BrokerRoute) *v2beta.BrokerSwapRoute {
	// Convert legs using the shared conversion function
	inboundLegs := convertToProtoIBCLeg(brokerSwap.InboundLegs)
	outboundLegs := convertToProtoIBCLeg(brokerSwap.OutboundLegs)

	result := &v2beta.BrokerSwapRoute{
		Path:                brokerSwap.Path,
		InboundLegs:         inboundLegs,
		Swap:                convertToProtoSwapQuote(brokerSwap.Swap),
		OutboundLegs:        outboundLegs,
		OutboundSupportsPfm: brokerSwap.OutboundSupportsPFM,
	}

	// Add execution data if available
	if brokerSwap.Execution != nil {
		execData := &v2beta.BrokerExecutionData{
			MinOutputAmount: brokerSwap.Execution.MinOutputAmount,
			UsesWasm:        brokerSwap.Execution.UsesWasm,
			Description:     brokerSwap.Execution.Description,
		}
		if brokerSwap.Execution.Memo != nil {
			execData.Memo = brokerSwap.Execution.Memo
		}
		if brokerSwap.Execution.IBCReceiver != nil {
			execData.IbcReceiver = brokerSwap.Execution.IBCReceiver
		}
		if brokerSwap.Execution.RecoverAddress != nil {
			execData.RecoverAddress = *brokerSwap.Execution.RecoverAddress
		}
		if brokerSwap.Execution.SmartContractData != nil {
			execData.SmartContractData = convertToProtoWasmData(brokerSwap.Execution.SmartContractData)
		}
		result.Execution = execData
	}

	return result
}

/*
Converts internal models.IBCLeg to v2beta.IBCLeg

Parameters:
- leg: *models.IBCLeg

Returns:
- *v2beta.IBCLeg

Errors:
- None
*/
func convertToProtoIBCLeg(legs []*models.IBCLeg) []*v2beta.IBCLeg {
	if legs == nil {
		return nil
	}
	protoLegs := make([]*v2beta.IBCLeg, len(legs))
	for i, leg := range legs {
		protoLegs[i] = &v2beta.IBCLeg{
			FromChain: leg.FromChain,
			ToChain:   leg.ToChain,
			Channel:   leg.Channel,
			Port:      leg.Port,
			Token:     convertToProtoTokenMapping(leg.Token),
			Amount:    leg.Amount,
		}
	}
	return protoLegs
}

/*
Converts internal models.TokenMapping to v2beta.TokenMapping

Parameters:
- token: *models.TokenMapping

Returns:
- *v2beta.TokenMapping

Errors:
- None
*/
func convertToProtoTokenMapping(token *models.TokenMapping) *v2beta.TokenMapping {
	if token == nil {
		return nil
	}
	return &v2beta.TokenMapping{
		ChainDenom:  token.ChainDenom,
		BaseDenom:   token.BaseDenom,
		OriginChain: token.OriginChain,
		IsNative:    token.IsNative,
	}
}

/*
Converts internal models.SwapQuote to v2beta.SwapQuote

Parameters:
- swap: *models.SwapQuote

Returns:
- *v2beta.SwapQuote

Errors:
- None
*/
func convertToProtoSwapQuote(swap *models.SwapQuote) *v2beta.SwapQuote {
	if swap == nil {
		return nil
	}

	protoSwap := &v2beta.SwapQuote{
		Broker:       swap.Broker,
		TokenIn:      convertToProtoTokenMapping(swap.TokenIn),
		TokenOut:     convertToProtoTokenMapping(swap.TokenOut),
		AmountIn:     swap.AmountIn,
		AmountOut:    swap.AmountOut,
		PriceImpact:  swap.PriceImpact,
		EffectiveFee: swap.EffectiveFee,
	}

	// Convert broker-specific RouteData based on broker type
	// This is the key part - converting interface{} to typed oneof
	switch swap.Broker { //nolint:gocritic
	case "osmosis-sqs":
		if osmosisData, ok := swap.RouteData.(*osmosis.RouteData); ok {
			protoSwap.RouteData = &v2beta.SwapQuote_OsmosisRouteData{
				OsmosisRouteData: convertOsmosisRouteData(osmosisData),
			}
		}
		// Add more brokers here as you implement them:
		// case "astroport" for example:
	}

	return protoSwap
}

/*
Converts Osmosis RouteData to v2beta.OsmosisRouteData

Parameters:
- data: *osmosis.RouteData

Returns:
- *v2beta.OsmosisRouteData

Errors:
- None
*/
func convertOsmosisRouteData(data *osmosis.RouteData) *v2beta.OsmosisRouteData {
	if data == nil {
		return nil
	}

	routes := make([]*v2beta.OsmosisRoute, len(data.Routes))
	for i, route := range data.Routes {
		pools := make([]*v2beta.OsmosisPool, len(route.Pools))
		for j, pool := range route.Pools {
			pools[j] = &v2beta.OsmosisPool{
				Id:            pool.ID,
				Type:          pool.Type,
				SpreadFactor:  pool.SpreadFactor,
				TokenOutDenom: pool.TokenOutDenom,
				TakerFee:      pool.TakerFee,
				LiquidityCap:  pool.LiquidityCap,
			}
		}

		routes[i] = &v2beta.OsmosisRoute{
			Pools:     pools,
			HasCwPool: route.HasCwPool,
			OutAmount: route.OutAmount,
			InAmount:  route.InAmount,
		}
	}

	return &v2beta.OsmosisRouteData{
		Routes:               routes,
		LiquidityCap:         data.LiquidityCap,
		LiquidityCapOverflow: data.LiquidityCapOverflow,
	}
}

func convertToProtoChainInfo(chain *routeindex.PathfinderChain, showSymbols *bool) *v2beta.ChainInfo {
	return &v2beta.ChainInfo{
		ChainId:   chain.Id,
		ChainName: chain.Name,
		HasPfm:    chain.HasPFM,
		IsBroker:  chain.Broker,
		Routes:    convertToProtoBasicRoute(chain.Routes, showSymbols),
	}
}

func convertToProtoBasicRoute(routes []routeindex.BasicRoute, showSymbols *bool) []*v2beta.BasicRoute {
	protoRoutes := make([]*v2beta.BasicRoute, len(routes))
	for i := range routes {
		protoRoutes[i] = &v2beta.BasicRoute{
			ToChain:       routes[i].ToChain,
			ToChainId:     routes[i].ToChainId,
			ConnectionId:  routes[i].ConnectionId,
			ChannelId:     routes[i].ChannelId,
			PortId:        routes[i].PortId,
			AllowedTokens: convertToProtoTokenInfo(routes[i].AllowedTokens, showSymbols),
		}
	}
	return protoRoutes
}

func convertToProtoTokenInfo(tokenInfo map[string]routeindex.TokenInfo, sortBySymbol *bool) map[string]*v2beta.TokenInfo {
	protoTokenInfos := make(map[string]*v2beta.TokenInfo, len(tokenInfo))
	if *sortBySymbol {
		for _, tokenInfo := range tokenInfo {
			protoTokenInfos[tokenInfo.Symbol+"@"+tokenInfo.OriginChain] = &v2beta.TokenInfo{
				ChainDenom:       tokenInfo.ChainDenom,
				CounterpartDenom: tokenInfo.IbcDenom,
				BaseDenom:        tokenInfo.BaseDenom,
				OriginChain:      tokenInfo.OriginChain,
				Decimals:         int32(tokenInfo.Decimals), //nolint:gosec // G115: Decimals is chain metadata, always within int32 range
				Symbol:           tokenInfo.Symbol,
			}
		}
	} else {
		for denom, tokenInfo := range tokenInfo {
			protoTokenInfos[denom] = &v2beta.TokenInfo{
				ChainDenom:       tokenInfo.ChainDenom,
				CounterpartDenom: tokenInfo.IbcDenom,
				BaseDenom:        tokenInfo.BaseDenom,
				OriginChain:      tokenInfo.OriginChain,
				Decimals:         int32(tokenInfo.Decimals), //nolint:gosec // G115: Decimals is chain metadata, always within int32 range
				Symbol:           tokenInfo.Symbol,
			}
		}
	}
	return protoTokenInfos
}

func convertToProtoWasmData(wasmData *ibcmemo.WasmMemo) *v2beta.WasmData {
	if wasmData == nil {
		return nil
	}
	return &v2beta.WasmData{
		Contract: wasmData.Wasm.Contract,
		Msg: &v2beta.WasmMsg{
			SwapAndAction: convertToProtoSwapAndAction(wasmData.Wasm.Msg.SwapAndAction),
		},
	}
}

func convertToProtoSwapAndAction(swapAndAction *ibcmemo.SwapAndAction) *v2beta.SwapAndAction {
	if swapAndAction == nil {
		return nil
	}

	userSwap := v2beta.UserSwap{
		SwapExactAssetIn: &v2beta.SwapExactAssetIn{
			SwapVenueName: swapAndAction.UserSwap.SwapExactAssetIn.SwapVenueName,
			Operations:    convertToProtoSwapOperations(swapAndAction.UserSwap.SwapExactAssetIn.Operations),
		},
	}
	minAsset := v2beta.MinAsset{
		Native: &v2beta.Asset{
			Amount: swapAndAction.MinAsset.Native.Amount,
			Denom:  swapAndAction.MinAsset.Native.Denom,
		},
	}
	return &v2beta.SwapAndAction{
		UserSwap:         &userSwap,
		MinAsset:         &minAsset,
		TimeoutTimestamp: swapAndAction.TimeoutTimestamp,
		PostSwapAction:   convertToProtoPostSwapAction(swapAndAction.PostSwapAction),
		// affiliates is already empty as it is just return it empty then
		Affiliates: []string{},
	}
}

func convertToProtoSwapOperations(operations []ibcmemo.SwapOperation) []*v2beta.SwapOperation {
	if operations == nil {
		return nil
	}
	protoOperations := make([]*v2beta.SwapOperation, len(operations))
	for i, operation := range operations {
		protoOperations[i] = &v2beta.SwapOperation{
			Pool:     operation.Pool,
			DenomIn:  operation.DenomIn,
			DenomOut: operation.DenomOut,
		}
	}
	return protoOperations
}

func convertToProtoPostSwapAction(postSwapAction *ibcmemo.PostSwapAction) *v2beta.PostSwapAction {
	if postSwapAction == nil {
		return nil
	}

	if postSwapAction.IBCTransfer != nil {
		return &v2beta.PostSwapAction{
			Action: &v2beta.PostSwapAction_IbcTransfer{
				IbcTransfer: convertToProtoIBCTransfer(postSwapAction.IBCTransfer),
			},
		}
	} else if postSwapAction.Transfer != nil {
		return &v2beta.PostSwapAction{
			Action: &v2beta.PostSwapAction_Transfer{
				Transfer: convertToProtoTransfer(postSwapAction.Transfer),
			},
		}
	}
	return nil
}

func convertToProtoIBCTransfer(ibcTransfer *ibcmemo.IBCTransfer) *v2beta.IBCTransfer {
	if ibcTransfer == nil {
		return nil
	}
	return &v2beta.IBCTransfer{
		IbcInfo: convertToProtoIBCInfo(ibcTransfer.IBCInfo),
	}
}

func convertToProtoIBCInfo(ibcInfo *ibcmemo.IBCInfo) *v2beta.IBCInfo {
	if ibcInfo == nil {
		return nil
	}
	return &v2beta.IBCInfo{
		Memo:           ibcInfo.Memo,
		Receiver:       ibcInfo.Receiver,
		RecoverAddress: ibcInfo.RecoverAddress,
		SourceChannel:  ibcInfo.SourceChannel,
	}
}

func convertToProtoTransfer(transfer *ibcmemo.Transfer) *v2beta.Transfer {
	if transfer == nil {
		return nil
	}
	return &v2beta.Transfer{
		ToAddress: transfer.ToAddress,
	}
}
