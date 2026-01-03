import type { EncodeObject } from "@cosmjs/proto-signing";
import { useCallback } from "react";
import type { TransactionOptions, TransactionResult } from "@/context/walletContext";
import { useWallet } from "@/context/walletContext";
import type { SwapAmountInRoute, SwapAmountInSplitRoute } from "@/lib/generated/osmosis/osmosis/poolmanager/v1beta1/swap_route";
import type { 
    MsgSplitRouteSwapExactAmountIn, 
    MsgSwapExactAmountIn 
} from "@/lib/generated/osmosis/osmosis/poolmanager/v1beta1/tx";
/**
 * Hook for IBC transfers
 * TODO: this is only for simple IBC transfers, for anything that is more complex containing wasm and 
 * PFM module needs to be refined to accept from the Pathfinder RPC
 */
export const useIBCTransfer = () => {
    const { sendTransaction, getAddress } = useWallet();

    const transfer = useCallback(
        async (
            sourceChainId: string,
            destinationAddress: string,
            sourceChannel: string,
            token: { amount: string; denom: string },
            options?: {
                memo?: string;
                timeoutMinutes?: number;
                sourcePort?: string;
            }
        ): Promise<TransactionResult> => {
            const sender = getAddress(sourceChainId);
            if (!sender) {
                throw new Error(`Not connected to chain ${sourceChainId}`);
            }

            const {
                memo = "",
                timeoutMinutes = 10,
                sourcePort = "transfer",
            } = options || {};

            // Calculate timeout timestamp (current time + timeout minutes)
            const timeoutTimestamp = `${(Date.now() + timeoutMinutes * 60 * 1000).toString()}000000`;

            const msg: EncodeObject = {
                typeUrl: "/ibc.applications.transfer.v1.MsgTransfer",
                value: {
                    sourcePort,
                    sourceChannel,
                    token,
                    sender,
                    receiver: destinationAddress,
                    timeoutHeight: {
                        revisionHeight: "0",
                        revisionNumber: "0",
                    },
                    timeoutTimestamp,
                    memo,
                },
            };

            return sendTransaction(sourceChainId, [msg], { memo: "Spectra IBC Transfer", ...options });
        },
        [sendTransaction, getAddress]
    );

    return { transfer };
};

/**
 * Hook for Osmosis swaps
 */
export const useOsmosisSwap = () => {
    const { sendTransaction, getAddress, isConnectedToChain } = useWallet();

    const swapExactAmountIn = useCallback(
        async (
            tokenIn: { denom: string; amount: string },
            routes: SwapAmountInRoute[],
            tokenOutMinAmount: string,
            options?: TransactionOptions
        ): Promise<TransactionResult> => {
            const chainId = "osmosis-1";
            const sender = getAddress(chainId);
            
            if (!sender) {
                throw new Error("Not connected to Osmosis");
            }

            const msgDetails: MsgSwapExactAmountIn = {
                sender,
                tokenIn,
                routes,
                tokenOutMinAmount,
            };

            const msg: EncodeObject = {
                typeUrl: "/osmosis.poolmanager.v1beta1.MsgSwapExactAmountIn",
                value: msgDetails,
            };

            return sendTransaction(chainId, [msg], {
                memo: "Spectra Swap",
                ...options,
            });
        },
        [sendTransaction, getAddress]
    );

    const swapSplitRoute = useCallback(
        async (
            tokenIn: { denom: string; amount: string },
            routes: SwapAmountInSplitRoute[],
            tokenOutMinAmount: string,
            options?: TransactionOptions
        ): Promise<TransactionResult> => {
            const chainId = "osmosis-1";
            const sender = getAddress(chainId);
            
            if (!sender) {
                throw new Error("Not connected to Osmosis");
            }

            const msgDetails: MsgSplitRouteSwapExactAmountIn = {
                sender,
                routes,
                tokenInDenom: tokenIn.denom,
                tokenOutMinAmount,
            };

            const msg: EncodeObject = {
                typeUrl: "/osmosis.poolmanager.v1beta1.MsgSplitRouteSwapExactAmountIn",
                value: msgDetails,
            };

            return sendTransaction(chainId, [msg], {
                memo: "Spectra Split Route Swap",
                ...options,
            });
        },
        [sendTransaction, getAddress]
    );

    return { 
        swapExactAmountIn, 
        swapSplitRoute,
        isConnected: isConnectedToChain("osmosis-1"),
    };
};

/**
 * Hook for common Cosmos SDK transactions
 */
export const useCosmosTransactions = () => {
    const { sendTransaction, getAddress } = useWallet();

    const delegate = useCallback(
        async (
            chainId: string,
            validatorAddress: string,
            amount: string,
            denom: string,
            options?: TransactionOptions
        ): Promise<TransactionResult> => {
            const delegator = getAddress(chainId);
            if (!delegator) {
                throw new Error(`Not connected to chain ${chainId}`);
            }

            const msg: EncodeObject = {
                typeUrl: "/cosmos.staking.v1beta1.MsgDelegate",
                value: {
                    delegatorAddress: delegator,
                    validatorAddress,
                    amount: { denom, amount },
                },
            };

            return sendTransaction(chainId, [msg], options);
        },
        [sendTransaction, getAddress]
    );

    const undelegate = useCallback(
        async (
            chainId: string,
            validatorAddress: string,
            amount: string,
            denom: string,
            options?: TransactionOptions
        ): Promise<TransactionResult> => {
            const delegator = getAddress(chainId);
            if (!delegator) {
                throw new Error(`Not connected to chain ${chainId}`);
            }

            const msg: EncodeObject = {
                typeUrl: "/cosmos.staking.v1beta1.MsgUndelegate",
                value: {
                    delegatorAddress: delegator,
                    validatorAddress,
                    amount: { denom, amount },
                },
            };

            return sendTransaction(chainId, [msg], options);
        },
        [sendTransaction, getAddress]
    );

    const redelegate = useCallback(
        async (
            chainId: string,
            srcValidatorAddress: string,
            dstValidatorAddress: string,
            amount: string,
            denom: string,
            options?: TransactionOptions
        ): Promise<TransactionResult> => {
            const delegator = getAddress(chainId);
            if (!delegator) {
                throw new Error(`Not connected to chain ${chainId}`);
            }

            const msg: EncodeObject = {
                typeUrl: "/cosmos.staking.v1beta1.MsgBeginRedelegate",
                value: {
                    delegatorAddress: delegator,
                    validatorSrcAddress: srcValidatorAddress,
                    validatorDstAddress: dstValidatorAddress,
                    amount: { denom, amount },
                },
            };

            return sendTransaction(chainId, [msg], options);
        },
        [sendTransaction, getAddress]
    );

    const withdrawRewards = useCallback(
        async (
            chainId: string,
            validatorAddresses: string[],
            options?: TransactionOptions
        ): Promise<TransactionResult> => {
            const delegator = getAddress(chainId);
            if (!delegator) {
                throw new Error(`Not connected to chain ${chainId}`);
            }

            if (validatorAddresses.length === 0) {
                throw new Error("No validator addresses provided");
            }

            const msgs: EncodeObject[] = validatorAddresses.map((valAddr) => ({
                typeUrl: "/cosmos.distribution.v1beta1.MsgWithdrawDelegatorReward",
                value: {
                    delegatorAddress: delegator,
                    validatorAddress: valAddr,
                },
            }));

            return sendTransaction(chainId, msgs, options);
        },
        [sendTransaction, getAddress]
    );

    const vote = useCallback(
        async (
            chainId: string,
            proposalId: string,
            option: number,
            options?: TransactionOptions
        ): Promise<TransactionResult> => {
            const voter = getAddress(chainId);
            if (!voter) {
                throw new Error(`Not connected to chain ${chainId}`);
            }

            const msg: EncodeObject = {
                typeUrl: "/cosmos.gov.v1.MsgVote",
                value: {
                    proposalId: BigInt(proposalId),
                    voter,
                    option,
                },
            };

            return sendTransaction(chainId, [msg], options);
        },
        [sendTransaction, getAddress]
    );

    return {
        delegate,
        undelegate,
        redelegate,
        withdrawRewards,
        vote,
    };
};

/**
 * Hook for sending custom transactions
 * Use this when you need full control over message construction
 */
export const useCustomTransaction = () => {
    const { sendTransaction } = useWallet();

    const send = useCallback(
        async (
            chainId: string,
            messages: EncodeObject[],
            options?: TransactionOptions
        ): Promise<TransactionResult> => {
            return sendTransaction(chainId, messages, options);
        },
        [sendTransaction]
    );

    return { send };
};

