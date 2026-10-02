import type { AminoConverters } from "@cosmjs/stargate";
import type {
    SwapAmountInRoute,
    SwapAmountInSplitRoute,
} from "@/lib/generated/osmosis/osmosis/poolmanager/v1beta1/swap_route";
import {
    MsgSplitRouteSwapExactAmountIn,
    MsgSwapExactAmountIn,
} from "@/lib/generated/osmosis/osmosis/poolmanager/v1beta1/tx";

// ts-proto doesn't generate Amino converters, so these are written by hand. Ledger
// devices can only sign Amino JSON, and the chain rebuilds that JSON from the
// protobuf it receives to verify the signature, so the shape here has to match
// Osmosis' x/tx aminojson output exactly:
//   - type names come from each message's `(amino.name)` proto option
//   - field names are the proto (snake_case) names
//   - uint64 (`pool_id`) is encoded as a string, `cosmossdk.io/math.Int` as its string value

interface AminoSwapAmountInRoute {
    pool_id: string;
    token_out_denom: string;
}

interface AminoSwapAmountInSplitRoute {
    pools: AminoSwapAmountInRoute[];
    token_in_amount: string;
}

export interface AminoMsgSwapExactAmountIn {
    sender: string;
    routes: AminoSwapAmountInRoute[];
    token_in: { denom: string; amount: string };
    token_out_min_amount: string;
}

export interface AminoMsgSplitRouteSwapExactAmountIn {
    sender: string;
    routes: AminoSwapAmountInSplitRoute[];
    token_in_denom: string;
    token_out_min_amount: string;
}

const routeToAmino = (route: SwapAmountInRoute): AminoSwapAmountInRoute => ({
    pool_id: route.poolId.toString(),
    token_out_denom: route.tokenOutDenom,
});

const routeFromAmino = (route: AminoSwapAmountInRoute): SwapAmountInRoute => ({
    poolId: Number(route.pool_id),
    tokenOutDenom: route.token_out_denom,
});

/**
 * Amino converters for the Osmosis poolmanager swap messages this app sends.
 * @returns AminoConverters to spread into a cosmjs AminoTypes instance
 */
export function createOsmosisAminoConverters(): AminoConverters {
    return {
        "/osmosis.poolmanager.v1beta1.MsgSwapExactAmountIn": {
            aminoType: "osmosis/poolmanager/swap-exact-amount-in",
            toAmino: ({
                sender,
                routes,
                tokenIn,
                tokenOutMinAmount,
            }: MsgSwapExactAmountIn): AminoMsgSwapExactAmountIn => {
                if (!tokenIn) {
                    throw new Error("MsgSwapExactAmountIn is missing tokenIn");
                }
                return {
                    sender,
                    routes: routes.map(routeToAmino),
                    token_in: { denom: tokenIn.denom, amount: tokenIn.amount },
                    token_out_min_amount: tokenOutMinAmount,
                };
            },
            fromAmino: ({
                sender,
                routes,
                token_in,
                token_out_min_amount,
            }: AminoMsgSwapExactAmountIn): MsgSwapExactAmountIn =>
                MsgSwapExactAmountIn.fromPartial({
                    sender,
                    routes: routes.map(routeFromAmino),
                    tokenIn: token_in,
                    tokenOutMinAmount: token_out_min_amount,
                }),
        },
        "/osmosis.poolmanager.v1beta1.MsgSplitRouteSwapExactAmountIn": {
            aminoType: "osmosis/poolmanager/split-amount-in",
            toAmino: ({
                sender,
                routes,
                tokenInDenom,
                tokenOutMinAmount,
            }: MsgSplitRouteSwapExactAmountIn): AminoMsgSplitRouteSwapExactAmountIn => ({
                sender,
                routes: routes.map((route: SwapAmountInSplitRoute) => ({
                    pools: route.pools.map(routeToAmino),
                    token_in_amount: route.tokenInAmount,
                })),
                token_in_denom: tokenInDenom,
                token_out_min_amount: tokenOutMinAmount,
            }),
            fromAmino: ({
                sender,
                routes,
                token_in_denom,
                token_out_min_amount,
            }: AminoMsgSplitRouteSwapExactAmountIn): MsgSplitRouteSwapExactAmountIn =>
                MsgSplitRouteSwapExactAmountIn.fromPartial({
                    sender,
                    routes: routes.map((route) => ({
                        pools: route.pools.map(routeFromAmino),
                        tokenInAmount: route.token_in_amount,
                    })),
                    tokenInDenom: token_in_denom,
                    tokenOutMinAmount: token_out_min_amount,
                }),
        },
    };
}
