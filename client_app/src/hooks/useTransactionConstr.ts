import { toUtf8 } from "@cosmjs/encoding";
import type { EncodeObject } from "@cosmjs/proto-signing";
import { useCallback } from "react";
import type {
    SwapAmountInRoute,
    SwapAmountInSplitRoute,
} from "@/lib/generated/osmosis/osmosis/poolmanager/v1beta1/swap_route";
import type { WasmMsg } from "@/lib/generated/pathfinder/pathfinder_route_pb";

export default function useTransactionConstructor() {
    /**
     * Creates an IBC transfer message
     */
    const createIbcTransferMessage = useCallback(
        (
            sender: string,
            receiver: string,
            sourceChannel: string,
            sourcePort: string,
            token: { amount: string; denom: string },
            memo: string = "",
            timeoutMinutes: number = 10,
        ): EncodeObject => {
            const timeoutTimestamp = `${(Date.now() + timeoutMinutes * 60 * 1000).toString()}000000`;

            return {
                typeUrl: "/ibc.applications.transfer.v1.MsgTransfer",
                value: {
                    sourcePort,
                    sourceChannel,
                    token,
                    sender,
                    receiver,
                    timeoutHeight: {
                        revisionHeight: "0",
                        revisionNumber: "0",
                    },
                    timeoutTimestamp,
                    memo,
                },
            };
        },
        [],
    );

    /**
     * Creates an Osmosis swap message (single route)
     */
    const createSwapMessage = useCallback(
        (
            sender: string,
            tokenIn: { amount: string; denom: string },
            routes: SwapAmountInRoute[],
            tokenOutMinAmount: string,
        ): EncodeObject => {
            const routeCount: number = routes.length;
            if (routeCount === 0) {
                throw new Error("No routes provided");
            }
            return {
                typeUrl: "/osmosis.poolmanager.v1beta1.MsgSwapExactAmountIn",
                value: {
                    sender,
                    tokenIn,
                    routes,
                    tokenOutMinAmount,
                },
            };
        },
        [],
    );

    /**
     * Creates an Osmosis split route swap message (multiple routes)
     */
    const createSplitRouteSwapMessage = useCallback(
        (
            sender: string,
            tokenInDenom: string,
            routes: SwapAmountInSplitRoute[],
            tokenOutMinAmount: string,
        ): EncodeObject => {
            if (routes.length === 0) {
                throw new Error("No routes provided");
            }
            return {
                typeUrl: "/osmosis.poolmanager.v1beta1.MsgSplitRouteSwapExactAmountIn",
                value: {
                    sender,
                    routes,
                    tokenInDenom,
                    tokenOutMinAmount,
                },
            };
        },
        [],
    );

    // TODO: This is hardcoded to Osmosis and the simple swap_and_action.
    // In the future this needs to be handled very differently...
    const createWasmExecutionMessage = useCallback(
        (
            sender: string,
            contract: string,
            msg: WasmMsg,
            funds: { denom: string; amount: string }[] = [],
        ): EncodeObject => {
            // Ensure fund amounts are strings
            const normalizedFunds = funds.map((fund) => ({
                denom: fund.denom,
                amount: String(fund.amount),
            }));

            // Manually construct the contract message to match Skip Go's format
            // The contract expects a plain JSON object, not protobuf-serialized format
            if (!msg.swapAndAction) {
                throw new Error("swapAndAction is required in WasmMsg");
            }

            const contractMsg = {
                swap_and_action: {
                    user_swap: msg.swapAndAction.userSwap
                        ? {
                              swap_exact_asset_in: {
                                  swap_venue_name:
                                      msg.swapAndAction.userSwap.swapExactAssetIn?.swapVenueName ||
                                      "",
                                  operations:
                                      msg.swapAndAction.userSwap.swapExactAssetIn?.operations.map(
                                          (op) => ({
                                              pool: op.pool,
                                              denom_in: op.denomIn,
                                              denom_out: op.denomOut,
                                          }),
                                      ) || [],
                              },
                          }
                        : undefined,
                    min_asset: msg.swapAndAction.minAsset
                        ? {
                              native: {
                                  amount: msg.swapAndAction.minAsset.native?.amount || "0",
                                  denom: msg.swapAndAction.minAsset.native?.denom || "",
                              },
                          }
                        : undefined,
                    timeout_timestamp: Number(msg.swapAndAction.timeoutTimestamp),
                    post_swap_action:
                        msg.swapAndAction.postSwapAction?.action.case === "ibcTransfer"
                            ? {
                                  ibc_transfer: {
                                      ibc_info: {
                                          memo:
                                              msg.swapAndAction.postSwapAction.action.value.ibcInfo
                                                  ?.memo || "",
                                          receiver:
                                              msg.swapAndAction.postSwapAction.action.value.ibcInfo
                                                  ?.receiver || "",
                                          recover_address:
                                              msg.swapAndAction.postSwapAction.action.value.ibcInfo
                                                  ?.recoverAddress || "",
                                          source_channel:
                                              msg.swapAndAction.postSwapAction.action.value.ibcInfo
                                                  ?.sourceChannel || "",
                                      },
                                  },
                              }
                            : undefined,
                    affiliates: msg.swapAndAction.affiliates || [],
                },
            };

            const jsonString = JSON.stringify(contractMsg);

            const msgBytes = toUtf8(jsonString);

            return {
                typeUrl: "/cosmwasm.wasm.v1.MsgExecuteContract",
                value: {
                    sender,
                    contract,
                    msg: msgBytes,
                    funds: normalizedFunds,
                },
            };
        },
        [],
    );

    return {
        createIbcTransferMessage,
        createSwapMessage,
        createSplitRouteSwapMessage,
        createWasmExecutionMessage,
    };
}
