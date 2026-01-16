/**
 * This module is related to any fetch requests that client(browser) will need to fetch.
 * Will mostly be used for the tracking user assets accross different chains.
 */
"use client";
import { type UseQueryResult, useQuery } from "@tanstack/react-query";
import {
    type AddressSpendableBalanceResponse,
    AddressSpendableBalanceResponseSchema,
    type IbcDenomTraceResponse,
    IbcDenomTraceResponseSchema,
    type TransactionRequestByEvents,
    type TransactionRequestByHash,
    type TransactionResponse,
    TransactionResponseSchema,
} from "@/components/modules/cosmosApiData";
import type { ClientChain } from "@/components/modules/tomlTypes";
import { getRandomHealthyApi } from "@/lib/apiQueries/featchHealthyEndpoint";
import logger from "@/lib/clientLogger";

export function useGetAddressBalance(
    chainId: string,
    address: string,
    timeoutOption: number = 10 * 1000, // overwrite if needed but I think 10 seconds is okay
): UseQueryResult<AddressSpendableBalanceResponse, Error> {
    const abort = new AbortController();
    setTimeout(() => {
        abort.abort();
    }, timeoutOption);
    return useQuery({
        queryKey: ["addressBalance", chainId, address],
        queryFn: async () => {
            const apiUrl = getRandomHealthyApi(chainId);
            if (!apiUrl) {
                throw new Error(`No healthy API found for chain ${chainId}`);
            }
            const response = await fetch(`${apiUrl}/cosmos/bank/v1beta1/balances/${address}`, {
                signal: abort.signal,
            });
            if (!response.ok) {
                logger.error(
                    `Failed to get address balance for chain ${chainId} and address ${address}`,
                );
                throw new Error(
                    `Failed to get address balance for chain ${chainId} and address ${address}`,
                );
            }
            const data = await response.json();
            return AddressSpendableBalanceResponseSchema.parse(data);
        },
        enabled: !!chainId && !!address,
        staleTime: 15 * 1000, // 15 seconds
        gcTime: 2 * 60 * 1000, // 2 minutes
        refetchOnWindowFocus: true,
        retryDelay: 1000, // 1 second
        retryOnMount: true,
        retry: 3,
    });
}

export function useGetTransactionByHash(
    chainId: string,
    transactionRequest: TransactionRequestByHash,
    timeoutOption: number = 15 * 1000, // overwrite if needed but I think 15 seconds is okay
): UseQueryResult<TransactionResponse, Error> {
    const abort = new AbortController();
    setTimeout(() => {
        abort.abort();
    }, timeoutOption);
    const query = useQuery({
        queryKey: ["transaction-tx", chainId, transactionRequest],
        queryFn: async () => {
            const apiUrl = getRandomHealthyApi(chainId);
            if (!apiUrl) {
                throw new Error(`No healthy API found for chain ${chainId}`);
            }
            const response = await fetch(
                `${apiUrl}/cosmos/tx/v1beta1/txs/${transactionRequest.hash}`,
                {
                    signal: abort.signal,
                },
            );
            if (!response.ok) {
                throw new Error(
                    `Failed to get transaction for chain ${chainId} and hash ${transactionRequest.hash}`,
                );
            }
            const data = await response.json();
            return TransactionResponseSchema.parse(data);
        },
        enabled: !!chainId && !!transactionRequest,
        // set to 5 minutes, technically once accquired it shouldn't be modified in any way
        staleTime: 5 * 60 * 1000,
        gcTime: 10 * 60 * 1000, // 10 minutes
        refetchOnWindowFocus: false,
        retryDelay: 1000, // 1 second
        retryOnMount: true,
        retry: 3,
    });
    return query;
}

export function useGetTransactionByEvents(
    chainConfig: ClientChain,
    transactionRequest: TransactionRequestByEvents,
    // overwrite if needed but I think 20 might too much but queries by events are a bit more complex
    timeoutOption: number = 20 * 1000,
): UseQueryResult<TransactionResponse, Error> {
    const abort = new AbortController();
    setTimeout(() => {
        abort.abort();
    }, timeoutOption);
    return useQuery({
        queryKey: ["transaction-ev", chainConfig.id, transactionRequest],
        queryFn: async () => {
            const apiUrl = getRandomHealthyApi(chainConfig.id);
            if (!apiUrl) {
                throw new Error(`No healthy API found for chain ${chainConfig.id}`);
            }

            // make the url for the transaction by events
            const responseUrl = makeUrlForTransactionByEvents(
                apiUrl,
                chainConfig,
                transactionRequest,
            );
            if (!responseUrl) {
                throw new Error(
                    `Failed to make url for transaction for chain ${chainConfig.id} and events ${transactionRequest.queries.join(",")}`,
                );
            }
            const response = await fetch(responseUrl, {
                signal: abort.signal,
            });
            if (!response.ok) {
                throw new Error(
                    `Failed to get transaction for chain ${chainConfig.id} and events ${transactionRequest.queries.join(",")}`,
                );
            }
            const data = await response.json();
            return TransactionResponseSchema.parse(data.tx_responses);
        },
        enabled: !!chainConfig.id && !!transactionRequest,
        // set to 5 minutes, technically once accquired it shouldn't be modified in any way
        staleTime: 5 * 60 * 1000,
        gcTime: 10 * 60 * 1000, // 10 minutes
        refetchOnWindowFocus: false,
        retryDelay: 1000, // 1 second
        retryOnMount: true,
        retry: 2, // because the timeout is a lot bigger here so we don't want to retry too often
    });
}

export function makeUrlForTransactionByEvents(
    apiUrl: string,
    chainConfig: ClientChain,
    transactionRequest: TransactionRequestByEvents,
): string {
    const queries = transactionRequest.queries;
    let path: string = "";

    if (chainConfig.cosmos_sdk_version < "v0.50.0") {
        // v0.47: uses "events" parameter with individual queries
        // should look like this:
        // https://atomone-api.cogwheel.zone/cosmos/tx/v1beta1/txs?order_by=ORDER_BY_UNSPECIFIED&limit=1&events=fungible_token_packet.sender%3D'atone16fxth82zn0zxr9mc2k6g9mc6fmv2ysf9kephw9'&events=fungible_token_packet.amount%3D'851000000'
        for (let index = 0; index < queries.length; index++) {
            const query = queries[index];
            path += `events=${encodeURIComponent(query)}`;
            if (index < queries.length - 1) {
                path += "&";
            }
        }
        path += `&order_by=${encodeURIComponent("ORDER_BY_UNSPECIFIED")}&limit=${transactionRequest.limit}`;
    } else {
        // v0.50+: uses "query" parameter with " AND " separator
        // example of the end result:
        // https://osmosis-api.polkachu.com/cosmos/tx/v1beta1/txs?order_by=ORDER_BY_UNSPECIFIED&limit=1&query=fungible_token_packet.sender%3D'atone16fxth82zn0zxr9mc2k6g9mc6fmv2ysf9kephw9'%20AND%20fungible_token_packet.amount%3D'851000000'
        const fullQuery = queries.join(" AND ");
        path = `order_by=${encodeURIComponent("ORDER_BY_UNSPECIFIED")}&limit=${transactionRequest.limit}&query=${encodeURIComponent(fullQuery)}`;
    }

    return `${apiUrl}/cosmos/tx/v1beta1/txs?${path}`;
}

export function getIbcDenomTrace(
    chainId: string,
    hash: string,
    timeoutOption: number = 5 * 1000, // overwrite if needed
): UseQueryResult<IbcDenomTraceResponse, Error> {
    // sanitize the hash input here by removing the "ibc/" prefix if it exists and also validate that it does have
    // the ibc/ prefix along with the correct length
    if (!hash.startsWith("ibc/")) {
        throw new Error(`Hash does not start with "ibc/"`);
    }
    if (hash.length !== 68) {
        throw new Error(`Hash does not have the correct length`);
    }
    const denomHash = hash.slice(4);
    if (denomHash.length !== 64) {
        throw new Error(`Denom hash does not have the correct length`);
    }

    const abort = new AbortController();
    setTimeout(() => {
        abort.abort();
    }, timeoutOption);
    return useQuery({
        queryKey: ["ibc-denom-trace", hash],
        queryFn: async () => {
            const apiUrl = getRandomHealthyApi(chainId);
            if (!apiUrl) {
                throw new Error(`No healthy API found for chain ${chainId}`);
            }
            const response = await fetch(`${apiUrl}/ibc/apps/transfer/v1/denom_traces/${hash}`, {
                signal: abort.signal,
            });
            if (!response.ok) {
                throw new Error(
                    `Failed to get IBC denom trace for chain ${chainId} and hash ${hash}`,
                );
            }
            const data = await response.json();
            return IbcDenomTraceResponseSchema.parse(data);
        },
        enabled: !!hash,
        staleTime: 5 * 60 * 1000,
        gcTime: 10 * 60 * 1000,
        refetchOnWindowFocus: false,
        retryDelay: 1000,
        retryOnMount: true,
        retry: 3,
    });
}
