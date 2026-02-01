package rpc

import (
	"fmt"
	"strings"

	"github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/models"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/router"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/router/brokers/osmosis"
	ibcmemo "github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/router/ibc_memo"
	v1 "github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/rpc/v1"
	"github.com/btcsuite/btcutil/bech32"
)

// CONVERT FUNCTIONS
// These convert between internal models and protobuf types

/*
Converts internal models.RouteResponse to v1.FindPathResponse
It uses the protobuf oneof to represent the different route types.

Parameters:
- resp: *models.RouteResponse

Returns:
- *v1.FindPathResponse

Errors:
- None
*/
func convertToProtoResponse(resp *models.RouteResponse) *v1.FindPathResponse {
	protoResp := &v1.FindPathResponse{
		Success:      resp.Success,
		ErrorMessage: resp.ErrorMessage,
	}

	// Convert Direct route if present (using protobuf oneof)
	if resp.Direct != nil {
		protoResp.Route = &v1.FindPathResponse_Direct{
			Direct: convertToProtoDirectRoute(resp.Direct),
		}
	}

	// Convert Indirect route if present (using protobuf oneof)
	if resp.Indirect != nil {
		protoResp.Route = &v1.FindPathResponse_Indirect{
			Indirect: convertToProtoIndirectRoute(resp.Indirect),
		}
	}

	// Convert BrokerSwap route if present (using protobuf oneof)
	if resp.BrokerSwap != nil {
		protoResp.Route = &v1.FindPathResponse_BrokerSwap{
			BrokerSwap: convertToProtoBrokerSwapRoute(resp.BrokerSwap),
		}
	}

	return protoResp
}

/*
Converts internal models.DirectRoute to v1.DirectRoute

Parameters:
- direct: *models.DirectRoute

Returns:
- *v1.DirectRoute

Errors:
- None
*/
func convertToProtoDirectRoute(direct *models.DirectRoute) *v1.DirectRoute {
	var transfer *v1.IBCLeg
	if direct.Transfer != nil {
		legs := convertToProtoIBCLeg([]*models.IBCLeg{direct.Transfer})
		if len(legs) > 0 {
			transfer = legs[0]
		}
	}
	return &v1.DirectRoute{
		Transfer: transfer,
	}
}

/*
Converts internal models.IndirectRoute to v1.IndirectRoute

Parameters:
- indirect: *models.IndirectRoute

Returns:
- *v1.IndirectRoute

Errors:
- None
*/
func convertToProtoIndirectRoute(indirect *models.IndirectRoute) *v1.IndirectRoute {
	legs := convertToProtoIBCLeg(indirect.Legs)
	return &v1.IndirectRoute{
		Path:          indirect.Path,
		Legs:          legs,
		SupportsPfm:   indirect.SupportsPFM,
		PfmStartChain: indirect.PFMStartChain,
		PfmMemo:       indirect.PFMMemo,
	}
}

/*
Converts internal models.BrokerRoute to v1.BrokerSwapRoute

Parameters:
- brokerSwap: *models.BrokerRoute

Returns:
- *v1.BrokerSwapRoute

Errors:
- None
*/
func convertToProtoBrokerSwapRoute(brokerSwap *models.BrokerRoute) *v1.BrokerSwapRoute {
	// Convert legs using the shared conversion function
	inboundLegs := convertToProtoIBCLeg(brokerSwap.InboundLegs)
	outboundLegs := convertToProtoIBCLeg(brokerSwap.OutboundLegs)

	result := &v1.BrokerSwapRoute{
		Path:                brokerSwap.Path,
		InboundLegs:         inboundLegs,
		Swap:                convertToProtoSwapQuote(brokerSwap.Swap),
		OutboundLegs:        outboundLegs,
		OutboundSupportsPfm: brokerSwap.OutboundSupportsPFM,
	}

	// Add execution data if available
	if brokerSwap.Execution != nil {
		execData := &v1.BrokerExecutionData{
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
Converts internal models.IBCLeg to v1.IBCLeg

Parameters:
- leg: *models.IBCLeg

Returns:
- *v1.IBCLeg

Errors:
- None
*/
func convertToProtoIBCLeg(legs []*models.IBCLeg) []*v1.IBCLeg {
	if legs == nil {
		return nil
	}
	protoLegs := make([]*v1.IBCLeg, len(legs))
	for i, leg := range legs {
		protoLegs[i] = &v1.IBCLeg{
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
Converts internal models.TokenMapping to v1.TokenMapping

Parameters:
- token: *models.TokenMapping

Returns:
- *v1.TokenMapping

Errors:
- None
*/
func convertToProtoTokenMapping(token *models.TokenMapping) *v1.TokenMapping {
	if token == nil {
		return nil
	}
	return &v1.TokenMapping{
		ChainDenom:  token.ChainDenom,
		BaseDenom:   token.BaseDenom,
		OriginChain: token.OriginChain,
		IsNative:    token.IsNative,
	}
}

/*
Converts internal models.SwapQuote to v1.SwapQuote

Parameters:
- swap: *models.SwapQuote

Returns:
- *v1.SwapQuote

Errors:
- None
*/
func convertToProtoSwapQuote(swap *models.SwapQuote) *v1.SwapQuote {
	if swap == nil {
		return nil
	}

	protoSwap := &v1.SwapQuote{
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
	switch swap.Broker {
	case "osmosis-sqs":
		if osmosisData, ok := swap.RouteData.(*osmosis.RouteData); ok {
			protoSwap.RouteData = &v1.SwapQuote_OsmosisRouteData{
				OsmosisRouteData: convertOsmosisRouteData(osmosisData),
			}
		}
		// Add more brokers here as you implement them:
		// case "astroport" for example:
	}

	return protoSwap
}

/*
Converts Osmosis RouteData to v1.OsmosisRouteData

Parameters:
- data: *osmosis.RouteData

Returns:
- *v1.OsmosisRouteData

Errors:
- None
*/
func convertOsmosisRouteData(data *osmosis.RouteData) *v1.OsmosisRouteData {
	if data == nil {
		return nil
	}

	routes := make([]*v1.OsmosisRoute, len(data.Routes))
	for i, route := range data.Routes {
		pools := make([]*v1.OsmosisPool, len(route.Pools))
		for j, pool := range route.Pools {
			pools[j] = &v1.OsmosisPool{
				Id:            int32(pool.ID),
				Type:          int32(pool.Type),
				SpreadFactor:  pool.SpreadFactor,
				TokenOutDenom: pool.TokenOutDenom,
				TakerFee:      pool.TakerFee,
				LiquidityCap:  pool.LiquidityCap,
			}
		}

		routes[i] = &v1.OsmosisRoute{
			Pools:     pools,
			HasCwPool: route.HasCwPool,
			OutAmount: route.OutAmount,
			InAmount:  route.InAmount,
		}
	}

	return &v1.OsmosisRouteData{
		Routes:               routes,
		LiquidityCap:         data.LiquidityCap,
		LiquidityCapOverflow: data.LiquidityCapOverflow,
	}
}

func convertToProtoChainInfo(chain *router.PathfinderChain, showSymbols *bool) *v1.ChainInfo {
	return &v1.ChainInfo{
		ChainId:   chain.Id,
		ChainName: chain.Name,
		HasPfm:    chain.HasPFM,
		IsBroker:  chain.Broker,
		Routes:    convertToProtoBasicRoute(chain.Routes, showSymbols),
	}
}

func convertToProtoBasicRoute(routes []router.BasicRoute, showSymbols *bool) []*v1.BasicRoute {
	protoRoutes := make([]*v1.BasicRoute, len(routes))
	for i := range routes {
		protoRoutes[i] = &v1.BasicRoute{
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

func convertToProtoTokenInfo(tokenInfo map[string]router.TokenInfo, sortBySymbol *bool) map[string]*v1.TokenInfo {
	protoTokenInfos := make(map[string]*v1.TokenInfo, len(tokenInfo))
	if *sortBySymbol {
		for _, tokenInfo := range tokenInfo {
			protoTokenInfos[tokenInfo.Symbol+"@"+tokenInfo.OriginChain] = &v1.TokenInfo{
				ChainDenom:  tokenInfo.ChainDenom,
				IbcDenom:    tokenInfo.IbcDenom,
				BaseDenom:   tokenInfo.BaseDenom,
				OriginChain: tokenInfo.OriginChain,
				Decimals:    int32(tokenInfo.Decimals),
				Symbol:      tokenInfo.Symbol,
			}
		}
	} else {
		for denom, tokenInfo := range tokenInfo {
			protoTokenInfos[denom] = &v1.TokenInfo{
				ChainDenom:  tokenInfo.ChainDenom,
				IbcDenom:    tokenInfo.IbcDenom,
				BaseDenom:   tokenInfo.BaseDenom,
				OriginChain: tokenInfo.OriginChain,
				Decimals:    int32(tokenInfo.Decimals),
				Symbol:      tokenInfo.Symbol,
			}
		}
	}
	return protoTokenInfos
}

// validateBech32Address validates that an address is a valid bech32 address
// Returns the prefix if valid, or an error if invalid
func validateBech32Address(address string) (string, error) {
	if address == "" {
		return "", fmt.Errorf("address is empty")
	}

	// Check minimum length (prefix + "1" + data + checksum)
	if len(address) < 10 {
		return "", fmt.Errorf("address too short (minimum 10 characters)")
	}

	// Check for separator
	sepIdx := strings.LastIndex(address, "1")
	if sepIdx < 1 {
		return "", fmt.Errorf("missing bech32 separator '1'")
	}

	// Validate the prefix (human-readable part)
	prefix := address[:sepIdx]
	if prefix == "" {
		return "", fmt.Errorf("empty bech32 prefix")
	}

	// Try to decode as bech32 - this validates the checksum
	decodedPrefix, data, err := bech32.Decode(address)
	if err != nil {
		return "", fmt.Errorf("invalid bech32 address (checksum failed): %w", err)
	}

	// Verify decoded prefix matches
	if decodedPrefix != prefix {
		return "", fmt.Errorf("bech32 prefix mismatch")
	}

	// Verify data is not empty (should be 20 or 32 bytes for cosmos addresses)
	if len(data) == 0 {
		return "", fmt.Errorf("empty address data")
	}

	return prefix, nil
}

func convertToProtoWasmData(wasmData *ibcmemo.WasmMemo) *v1.WasmData {
	if wasmData == nil {
		return nil
	}
	return &v1.WasmData{
		Contract: wasmData.Wasm.Contract,
		Msg: &v1.WasmMsg{
			SwapAndAction: convertToProtoSwapAndAction(wasmData.Wasm.Msg.SwapAndAction),
		},
	}
}

func convertToProtoSwapAndAction(swapAndAction *ibcmemo.SwapAndAction) *v1.SwapAndAction {
	if swapAndAction == nil {
		return nil
	}

	userSwap := v1.UserSwap{
		SwapExactAssetIn: &v1.SwapExactAssetIn{
			SwapVenueName: swapAndAction.UserSwap.SwapExactAssetIn.SwapVenueName,
			Operations:    convertToProtoSwapOperations(swapAndAction.UserSwap.SwapExactAssetIn.Operations),
		},
	}
	minAsset := v1.MinAsset{
		Native: &v1.Asset{
			Amount: swapAndAction.MinAsset.Native.Amount,
			Denom:  swapAndAction.MinAsset.Native.Denom,
		},
	}
	return &v1.SwapAndAction{
		UserSwap:         &userSwap,
		MinAsset:         &minAsset,
		TimeoutTimestamp: swapAndAction.TimeoutTimestamp,
		PostSwapAction:   convertToProtoPostSwapAction(swapAndAction.PostSwapAction),
		// affiliates is already empty as it is just return it empty then
		Affiliates: []string{},
	}
}

func convertToProtoSwapOperations(operations []ibcmemo.SwapOperation) []*v1.SwapOperation {
	if operations == nil {
		return nil
	}
	protoOperations := make([]*v1.SwapOperation, len(operations))
	for i, operation := range operations {
		protoOperations[i] = &v1.SwapOperation{
			Pool:     operation.Pool,
			DenomIn:  operation.DenomIn,
			DenomOut: operation.DenomOut,
		}
	}
	return protoOperations
}

func convertToProtoPostSwapAction(postSwapAction *ibcmemo.PostSwapAction) *v1.PostSwapAction {
	if postSwapAction == nil {
		return nil
	}

	if postSwapAction.IBCTransfer != nil {
		return &v1.PostSwapAction{
			Action: &v1.PostSwapAction_IbcTransfer{
				IbcTransfer: convertToProtoIBCTransfer(postSwapAction.IBCTransfer),
			},
		}
	} else if postSwapAction.Transfer != nil {
		return &v1.PostSwapAction{
			Action: &v1.PostSwapAction_Transfer{
				Transfer: convertToProtoTransfer(postSwapAction.Transfer),
			},
		}
	}
	return nil
}

func convertToProtoIBCTransfer(ibcTransfer *ibcmemo.IBCTransfer) *v1.IBCTransfer {
	if ibcTransfer == nil {
		return nil
	}
	return &v1.IBCTransfer{
		IbcInfo: convertToProtoIBCInfo(ibcTransfer.IBCInfo),
	}
}

func convertToProtoIBCInfo(ibcInfo *ibcmemo.IBCInfo) *v1.IBCInfo {
	if ibcInfo == nil {
		return nil
	}
	return &v1.IBCInfo{
		Memo:           ibcInfo.Memo,
		Receiver:       ibcInfo.Receiver,
		RecoverAddress: ibcInfo.RecoverAddress,
		SourceChannel:  ibcInfo.SourceChannel,
	}
}

func convertToProtoTransfer(transfer *ibcmemo.Transfer) *v1.Transfer {
	if transfer == nil {
		return nil
	}
	return &v1.Transfer{
		ToAddress: transfer.ToAddress,
	}
}
