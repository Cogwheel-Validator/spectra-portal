/**
 * This module is related to any fetch requests that client(browser) will need to fetch.
 * Will mostly be used for the tracking user assets accross different chains.
 */
"use client";
import { type UseQueryResult, useQuery } from "@tanstack/react-query";
import {
    type AddressSpendableBalanceResponse,
    AddressSpendableBalanceResponseSchema,
    type EvTransactionResponse,
    EvTransactionResponseSchema,
    type IbcDenomTraceResponse,
    IbcDenomTraceResponseSchema,
    isTransactionNotFound,
    type TransactionRequestByEvents,
    type TransactionRequestByHash,
    type TransactionResponse,
    type TransactionResponseError,
    TransactionResponseErrorSchema,
    TransactionResponseSchema,
} from "@/components/modules/cosmosApiData";
import type { ClientChain } from "@/components/modules/tomlTypes";
import { getRandomHealthyApiImperative } from "@/lib/apiQueries/featchHealthyEndpoint";
import logger from "@/lib/clientLogger";

/**
 * Hook to query the address balance
 * @param chainId chain id of the chain that is about to be queried
 * @param address address of the account that is about to be queried
 * @param timeoutOption timeout option for the query
 * @returns
 */
export function useGetAddressBalance(
    chainId: string,
    address: string,
    timeoutOption: number = 10 * 1000, // overwrite if needed but I think 10 seconds is okay
): UseQueryResult<AddressSpendableBalanceResponse, Error> {
    return useQuery({
        queryKey: ["addressBalance", chainId, address],
        queryFn: async () => {
            const apiUrl = await getRandomHealthyApiImperative(chainId);
            if (!apiUrl) {
                throw new Error(`No healthy API found for chain ${chainId}`);
            }

            const abort = new AbortController();
            const timeoutId = setTimeout(() => {
                abort.abort();
            }, timeoutOption);

            try {
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
                const parsedData = AddressSpendableBalanceResponseSchema.parse(data);
                return parsedData;
            } finally {
                clearTimeout(timeoutId);
            }
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

/**
 * Hook to query the transaction by hash
 * @param chainId chain id of the chain that is about to be queried
 * @param transactionRequest request of the transaction that is about to be queried
 * @param timeoutOption timeout option for the query
 * @returns
 */
export function useGetTransactionByHash(
    chainId: string,
    transactionRequest: TransactionRequestByHash,
    timeoutOption: number = 15 * 1000, // overwrite if needed but I think 15 seconds is okay
): UseQueryResult<TransactionResponse, Error> {
    const query = useQuery({
        queryKey: ["transaction-tx", chainId, transactionRequest],
        queryFn: async () => {
            const apiUrl = await getRandomHealthyApiImperative(chainId);
            if (!apiUrl) {
                throw new Error(`No healthy API found for chain ${chainId}`);
            }

            const abort = new AbortController();
            const timeoutId = setTimeout(() => {
                abort.abort();
            }, timeoutOption);

            try {
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
            } finally {
                clearTimeout(timeoutId);
            }
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

/**
 * Imperative option to query the transaction by tx hash
 * @param chainId chaind id of the chain that is about to be queried
 * @param transactionHash hash of the transaction that is about to be queried
 * @param timeoutOption timeout option for the query
 * @returns
 */
export async function fetchTransactionByHash(
    chainId: string,
    transactionHash: string,
    timeoutOption: number = 15 * 1000, // overwrite if needed but I think 15 seconds is okay
): Promise<TransactionResponse | null> {
    const apiUrl = await getRandomHealthyApiImperative(chainId);
    if (!apiUrl) return null;
    const abort = new AbortController();
    const timeoutId = setTimeout(() => {
        abort.abort();
    }, timeoutOption);
    try {
        const response = await fetch(`${apiUrl}/cosmos/tx/v1beta1/txs/${transactionHash}`, {
            signal: abort.signal,
        });
        if (!response.ok) return null;
        return TransactionResponseSchema.parse(await response.json());
    } catch (error) {
        logger.error(
            `Failed to get transaction for chain ${chainId} and hash ${transactionHash}`,
            error,
        );
        return null;
    } finally {
        clearTimeout(timeoutId);
    }
}

/**
 * Hook to query the transaction by events
 * @param chainConfig chain config of the chain that is about to be queried
 * @param transactionRequest request of the transaction that is about to be queried
 * @param timeoutOption timeout option for the query
 * @returns
 */
export function useGetTransactionByEvents(
    chainConfig: ClientChain,
    transactionRequest: TransactionRequestByEvents,
    // overwrite if needed but I think 20 might too much but queries by events are a bit more complex
    timeoutOption: number = 20 * 1000,
): UseQueryResult<TransactionResponse, Error> {
    return useQuery({
        queryKey: ["transaction-ev", chainConfig.id, transactionRequest],
        queryFn: async () => {
            const apiUrl = await getRandomHealthyApiImperative(chainConfig.id);
            if (!apiUrl) {
                throw new Error(`No healthy API found for chain ${chainConfig.id}`);
            }

            const abort = new AbortController();
            const timeoutId = setTimeout(() => {
                abort.abort();
            }, timeoutOption);

            try {
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
            } finally {
                clearTimeout(timeoutId);
            }
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

/**
 * Imperative option to query the transaction by events
 * @param chainConfig chain config of the chain that is about to be queried
 * @param transactionRequest request of the transaction that is about to be queried
 * @param timeoutOption timeout option for the query
 * @returns TransactionResponse if found, TransactionResponseError if error occurred, null if not found yet (empty result)
 */
export async function fetchTransactionByEvents(
    chainConfig: ClientChain,
    transactionRequest: TransactionRequestByEvents,
    timeoutOption: number = 20 * 1000,
): Promise<EvTransactionResponse | TransactionResponseError | null> {
    const apiUrl = await getRandomHealthyApiImperative(chainConfig.id);
    if (!apiUrl) {
        return {
            code: 400,
            message: "No healthy API found for chain",
            details: ["No healthy API found for chain"],
        };
    }

    const abort = new AbortController();
    const timeoutId = setTimeout(() => {
        abort.abort();
    }, timeoutOption);

    const fetchUrl = makeUrlForTransactionByEvents(apiUrl, chainConfig, transactionRequest);

    logger.info("fetch url", {
        url: fetchUrl,
        urlLength: fetchUrl.length,
        queries: transactionRequest.queries,
        queriesJoined: transactionRequest.queries.join(" AND "),
    });

    try {
        const response = await fetch(fetchUrl, {
            signal: abort.signal,
        });

        if (!response.ok) {
            return TransactionResponseErrorSchema.parse({
                code: response.status,
                message: response.statusText,
                details: [response.statusText],
            });
        }

        const data = await response.json();

        // Check if this is an empty response (transaction not yet indexed)
        if (isTransactionNotFound(data)) {
            return null; // Signal that transaction not found yet - should retry
        }

        // Try to parse as successful response
        try {
            return EvTransactionResponseSchema.parse(data);
        } catch (successParseError) {
            // If it doesn't match successful response schema, try error schema
            try {
                return TransactionResponseErrorSchema.parse(data);
            } catch (errorParseError) {
                // Neither schema matched - log the actual response for debugging
                logger.error("Response doesn't match any expected schema", {
                    data,
                    successParseError,
                    errorParseError,
                });
                // Return a generic error
                return {
                    code: 500,
                    message: "Unexpected response format from API",
                    details: [JSON.stringify(data)],
                };
            }
        }
    } catch (error) {
        logger.error(
            `Failed to get transaction for chain ${chainConfig.id} and events ${transactionRequest.queries.join(",")}`,
            error,
        );
        return {
            code: 500,
            message: `Failed to get transaction for chain ${chainConfig.id} and events ${transactionRequest.queries.join(",")}`,
            details: [String(error)],
        };
    } finally {
        clearTimeout(timeoutId);
    }
}

/**
 * Helper function to make the url for the transaction by events
 * @param apiUrl api url of the chain that is about to be queried
 * @param chainConfig chain config of the chain that is about to be queried
 * @param transactionRequest request of the transaction that is about to be queried
 * @returns
 */
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
        const encodedQuery = encodeURIComponent(fullQuery);

        logger.info("Building v0.50+ query URL", {
            queriesArray: queries,
            fullQueryBeforeEncode: fullQuery,
            fullQueryLength: fullQuery.length,
            encodedQuery: encodedQuery,
            encodedQueryLength: encodedQuery.length,
            lastCharBeforeEncode: fullQuery.charAt(fullQuery.length - 1),
            lastCharAfterEncode: encodedQuery.slice(-3), // last 3 chars to see if %27 is there
        });

        path = `order_by=${encodeURIComponent("ORDER_BY_UNSPECIFIED")}&limit=${transactionRequest.limit}&query=${encodedQuery}`;
    }

    const finalUrl = `${apiUrl}/cosmos/tx/v1beta1/txs?${path}`;

    logger.info("Final URL construction", {
        finalUrl,
        finalUrlLength: finalUrl.length,
        endsWithQuote: finalUrl.includes("%27") && finalUrl.lastIndexOf("%27"),
    });

    return finalUrl;
}

export function useGetIbcDenomTrace(
    chainId: string,
    hash: string,
    timeoutOption: number = 5 * 1000, // overwrite if needed
): UseQueryResult<IbcDenomTraceResponse, Error> {
    // Validate only if hash is provided (non-empty)
    if (hash) {
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
    }

    return useQuery({
        queryKey: ["ibc-denom-trace", hash],
        queryFn: async () => {
            const apiUrl = await getRandomHealthyApiImperative(chainId);
            if (!apiUrl) {
                throw new Error(`No healthy API found for chain ${chainId}`);
            }

            const abort = new AbortController();
            const timeoutId = setTimeout(() => {
                abort.abort();
            }, timeoutOption);

            try {
                const response = await fetch(
                    `${apiUrl}/ibc/apps/transfer/v1/denom_traces/${hash}`,
                    {
                        signal: abort.signal,
                    },
                );
                if (!response.ok) {
                    throw new Error(
                        `Failed to get IBC denom trace for chain ${chainId} and hash ${hash}`,
                    );
                }
                const data = await response.json();
                return IbcDenomTraceResponseSchema.parse(data);
            } finally {
                clearTimeout(timeoutId);
            }
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
