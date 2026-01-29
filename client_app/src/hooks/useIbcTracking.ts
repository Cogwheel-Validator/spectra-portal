"use client";

import { useCallback, useRef } from "react";
import { isEvTransactionResponse } from "@/components/modules/cosmosApiData";
import type { ClientChain } from "@/components/modules/tomlTypes";
import { fetchTransactionByHash } from "@/lib/apiQueries/fetchApiData";
import clientLogger from "@/lib/clientLogger";
import type { IbcTrackingResult, PacketData, TrackingOptions } from "./ibcTracking/types";
import { extractSendPacket, queryIbcReceive } from "./ibcTracking/util";

/**
 * Hook for tracking IBC transactions across chains
 * Returns functions for:
 * - Getting IBC packet data from a source transaction
 * - Polling for confirmation on destination chain
 */
export function useIbcTracking() {
    const abortControllerRef = useRef<AbortController | null>(null);

    /**
     * Get IBC packet data from a transaction on the source chain
     * Returns packet data and source transaction timestamp
     */
    const getPacketData = useCallback(
        async (
            sourceChainId: string,
            txHash: string,
        ): Promise<{ packetData: PacketData; timestamp: string } | null> => {
            const txResponse = await fetchTransactionByHash(sourceChainId, txHash);
            if (!txResponse || !txResponse.tx_response) return null;

            const packetData = extractSendPacket(txResponse);
            if (!packetData) return null;

            clientLogger.info("Source tx response", {
                hash: txHash,
                timestamp: txResponse.tx_response.timestamp,
            });

            return {
                packetData,
                timestamp: txResponse.tx_response.timestamp,
            };
        },
        [],
    );

    /**
     * Poll for IBC confirmation on destination chain
     * Returns when the receive transaction is found or timeout/max attempts reached
     */
    const waitForConfirmation = useCallback(
        async (
            destChain: ClientChain,
            packetData: PacketData,
            sourceTxTimestamp?: string,
            options: TrackingOptions = {},
        ): Promise<IbcTrackingResult> => {
            const {
                maxAttempts = 60,
                pollInterval = 10000, // 10 seconds
                timeout = 10 * 60 * 1000, // 10 minutes
                onProgress,
            } = options;

            // Create abort controller for this tracking session
            abortControllerRef.current = new AbortController();
            const startTime = Date.now();

            for (let attempt = 1; attempt <= maxAttempts; attempt++) {
                // Check if aborted
                if (abortControllerRef.current?.signal.aborted) {
                    return { success: false, error: "Tracking cancelled" };
                }

                // Check timeout
                if (Date.now() - startTime > timeout) {
                    return { success: false, error: "Tracking timeout exceeded" };
                }

                // Report progress
                onProgress?.(attempt, maxAttempts);

                // Try to find the receive transaction (with timestamp verification)
                const receiveResult = await queryIbcReceive(
                    destChain,
                    packetData,
                    sourceTxTimestamp,
                );

                clientLogger.info("receive IBC data confirmation", receiveResult);

                // Handle the three possible outcomes
                if (receiveResult === null) {
                    // Transaction not found yet - poll again
                    clientLogger.info("Transaction not found yet, will retry", {
                        attempt,
                        maxAttempts,
                    });
                } else if ("code" in receiveResult && "message" in receiveResult) {
                    // API error occurred (bad params, server error, etc.)
                    // Continue polling - might be temporary issue or transaction just not indexed
                    clientLogger.warn("Received error response, retrying", {
                        attempt,
                        maxAttempts,
                        code: receiveResult.code,
                        message: receiveResult.message,
                    });
                } else if (
                    isEvTransactionResponse(receiveResult) &&
                    receiveResult.tx_responses.length > 0
                ) {
                    // We got a valid transaction response (already verified in queryIbcReceive)
                    const txResponse = receiveResult.tx_responses[0];
                    if (txResponse.code === 0) {
                        // Transaction succeeded
                        return {
                            success: true,
                            txHash: txResponse.txhash,
                        };
                    } else {
                        // Transaction failed on destination chain
                        return {
                            success: false,
                            error: `Receive transaction failed with code ${txResponse.code}`,
                            txHash: txResponse.txhash,
                        };
                    }
                }

                // If this is not the last attempt, wait before next poll
                // This ensures we always respect the pollInterval
                if (attempt < maxAttempts) {
                    await new Promise<void>((resolve) => {
                        const timer = setTimeout(resolve, pollInterval);
                        // Allow abort to cancel the wait
                        abortControllerRef.current?.signal.addEventListener("abort", () => {
                            clearTimeout(timer);
                            resolve();
                        });
                    });
                }
            }

            return {
                success: false,
                error: `IBC confirmation not found after ${maxAttempts} attempts`,
            };
        },
        [],
    );

    /**
     * Cancel ongoing tracking
     */
    const cancelTracking = useCallback(() => {
        abortControllerRef.current?.abort();
    }, []);

    /**
     * Full tracking flow: get packet data from source tx, then wait for confirmation
     */
    const trackIbcTransfer = useCallback(
        async (
            sourceChainId: string,
            sourceTxHash: string,
            destChain: ClientChain,
            options: TrackingOptions = {},
        ): Promise<IbcTrackingResult> => {
            const {
                maxAttempts = 60,
                pollInterval = 10000, // 10 seconds
                timeout = 10 * 60 * 1000, // 10 minutes
            } = options;

            const startTime = Date.now();

            // Poll for packet data from source transaction
            // The transaction might not be indexed immediately after submission
            let packetDataResult: { packetData: PacketData; timestamp: string } | null = null;

            for (let attempt = 1; attempt <= maxAttempts; attempt++) {
                // Check timeout
                if (Date.now() - startTime > timeout) {
                    return { success: false, error: "Timeout while fetching source transaction" };
                }

                packetDataResult = await getPacketData(sourceChainId, sourceTxHash);

                clientLogger.info("packet data", { packetDataResult, attempt, maxAttempts });

                if (packetDataResult) {
                    // Successfully got packet data, break out of loop
                    break;
                }

                // If this is not the last attempt, wait before next poll
                // This ensures we always respect the pollInterval
                if (attempt < maxAttempts) {
                    await new Promise<void>((resolve) => {
                        const timer = setTimeout(resolve, pollInterval);
                        // Allow abort to cancel the wait
                        abortControllerRef.current?.signal.addEventListener("abort", () => {
                            clearTimeout(timer);
                            resolve();
                        });
                    });
                }
            }

            if (!packetDataResult) {
                return {
                    success: false,
                    error: "Could not extract IBC packet data from source transaction after multiple attempts",
                };
            }

            // Wait for confirmation on destination (with timestamp verification)
            return waitForConfirmation(
                destChain,
                packetDataResult.packetData,
                packetDataResult.timestamp,
                options,
            );
        },
        [getPacketData, waitForConfirmation],
    );

    return {
        getPacketData,
        waitForConfirmation,
        trackIbcTransfer,
        cancelTracking,
    };
}

export default useIbcTracking;
