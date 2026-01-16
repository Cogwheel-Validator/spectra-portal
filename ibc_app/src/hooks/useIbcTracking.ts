"use client";

import { useCallback, useRef } from "react";
import { getRecvPacketEvent, type TransactionResponse } from "@/components/modules/cosmosApiData";
import type { ClientChain } from "@/components/modules/tomlTypes";
import { getRandomHealthyApi } from "@/lib/apiQueries/featchHealthyEndpoint";

// ============================================================================
// Types
// ============================================================================

export interface IbcPacketData {
    packetSequence: string;
    sourceChannel: string;
    sourcePort: string;
    destChannel: string;
    destPort: string;
    timeoutHeight: {
        revisionHeight: string;
        revisionNumber: string;
    };
    timeoutTimestamp: string;
}

export interface IbcTrackingResult {
    success: boolean;
    txHash?: string;
    error?: string;
}

export interface TrackingOptions {
    maxAttempts?: number; // Maximum polling attempts (default: 60)
    pollInterval?: number; // Milliseconds between polls (default: 10000 = 10s)
    timeout?: number; // Total timeout in ms (default: 10 minutes)
    onProgress?: (attempt: number, maxAttempts: number) => void;
}

// ============================================================================
// Helper Functions
// ============================================================================

/**
 * Extract IBC packet data from a send transaction
 */
export function extractIbcPacketData(txResponse: TransactionResponse): IbcPacketData | null {
    if (!txResponse.tx_responses || txResponse.tx_responses.length === 0) {
        return null;
    }

    const events = txResponse.tx_responses[0].events;
    const packet = getRecvPacketEvent(events);

    if (!packet) {
        return null;
    }

    // Extract packet data from send_packet event
    const getAttr = (key: string) => packet.attributes.find((a) => a.key === key)?.value || "";

    return {
        packetSequence: getAttr("packet_sequence"),
        sourceChannel: getAttr("packet_src_channel"),
        sourcePort: getAttr("packet_src_port"),
        destChannel: getAttr("packet_dst_channel"),
        destPort: getAttr("packet_dst_port"),
        timeoutHeight: {
            revisionHeight: getAttr("packet_timeout_height").split("-")[1] || "0",
            revisionNumber: getAttr("packet_timeout_height").split("-")[0] || "0",
        },
        timeoutTimestamp: getAttr("packet_timeout_timestamp"),
    };
}

/**
 * Fetch transaction by hash (non-hook version for imperative use)
 */
async function fetchTransactionByHash(
    chainId: string,
    hash: string,
): Promise<TransactionResponse | null> {
    const apiUrl = getRandomHealthyApi(chainId);
    if (!apiUrl) return null;

    try {
        const response = await fetch(`${apiUrl}/cosmos/tx/v1beta1/txs/${hash}`);
        if (!response.ok) return null;
        return await response.json();
    } catch {
        return null;
    }
}

/**
 * Query for IBC receive transaction on destination chain
 */
async function queryIbcReceive(
    destChain: ClientChain,
    packetData: IbcPacketData,
): Promise<TransactionResponse | null> {
    const apiUrl = getRandomHealthyApi(destChain.id);
    if (!apiUrl) return null;

    // Build query for recv_packet event
    const queries = [
        `recv_packet.packet_sequence='${packetData.packetSequence}'`,
        `recv_packet.packet_src_channel='${packetData.sourceChannel}'`,
        `recv_packet.packet_dst_channel='${packetData.destChannel}'`,
    ];

    const queryString = queries.map((q) => `events=${encodeURIComponent(q)}`).join("&");

    try {
        const response = await fetch(
            `${apiUrl}/cosmos/tx/v1beta1/txs?${queryString}&pagination.limit=1`,
        );
        if (!response.ok) return null;
        const data = await response.json();
        return data.tx_responses?.length > 0 ? data : null;
    } catch {
        return null;
    }
}

// ============================================================================
// Hook
// ============================================================================

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
     */
    const getPacketData = useCallback(
        async (sourceChainId: string, txHash: string): Promise<IbcPacketData | null> => {
            const txResponse = await fetchTransactionByHash(sourceChainId, txHash);
            if (!txResponse) return null;
            return extractIbcPacketData(txResponse);
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
            packetData: IbcPacketData,
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

                // Try to find the receive transaction
                const receiveResult = await queryIbcReceive(destChain, packetData);

                if (receiveResult?.tx_responses?.[0]) {
                    const receiveTx = receiveResult.tx_responses[0];

                    // Check if transaction was successful
                    if (receiveTx.code === 0) {
                        return {
                            success: true,
                            txHash: receiveTx.txhash,
                        };
                    } else {
                        return {
                            success: false,
                            error: `Receive transaction failed with code ${receiveTx.code}`,
                            txHash: receiveTx.txhash,
                        };
                    }
                }

                // Wait before next attempt
                await new Promise<void>((resolve) => {
                    const timer = setTimeout(resolve, pollInterval);
                    // Allow abort to cancel the wait
                    abortControllerRef.current?.signal.addEventListener("abort", () => {
                        clearTimeout(timer);
                        resolve();
                    });
                });
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
            // Get packet data from source transaction
            const packetData = await getPacketData(sourceChainId, sourceTxHash);

            if (!packetData) {
                return {
                    success: false,
                    error: "Could not extract IBC packet data from source transaction",
                };
            }

            // Wait for confirmation on destination
            return waitForConfirmation(destChain, packetData, options);
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
