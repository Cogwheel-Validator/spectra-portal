"use client";

import { useCallback, useRef } from "react";
import {
    type EvTransactionResponse,
    getRecvPacketEvent,
    getSendPacketEvent,
    isEvTransactionResponse,
    type TransactionEvents,
    type TransactionResponse,
    type TransactionResponseError,
} from "@/components/modules/cosmosApiData";
import type { ClientChain } from "@/components/modules/tomlTypes";
import { getRandomHealthyApiImperative } from "@/lib/apiQueries/featchHealthyEndpoint";
import { fetchTransactionByEvents, fetchTransactionByHash } from "@/lib/apiQueries/fetchApiData";
import clientLogger from "@/lib/clientLogger";


// ============================================================================
// Types
// ============================================================================

export interface PacketData {
    packetDataHex: string;
    // packet timeout is technically a Unix timestamp but it is stored as a string
    packetTimeout: string;
    packetSequence: string;
    packetSrcPort: string;
    packetSrcChannel: string;
    packetDstPort: string;
    packetDstChannel: string;
    packetChannelOrdering: string;
    packetConnectionId: string;
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
 * Maximum safe query length (accounting for AND operators between queries)
 * Cosmos SDK appears to limit queries to ~500 chars total
 */
const MAX_QUERY_LENGTH = 450;

/**
 * Estimate the total length of a query array when joined with ' AND '
 */
function estimateQueryLength(queries: string[]): number {
    return queries.join(" AND ").length;
}

/**
 * Build tiered query strategies for finding IBC receive transaction
 * Returns queries in order of preference (most specific first)
 */
function buildQueryStrategies(packetData: PacketData): string[][] {
    const strategies: string[][] = [];

    // Strategy 1: packet_data_hex + sequence + src_channel (most specific)
    const strategy1 = [
        `recv_packet.packet_data_hex='${packetData.packetDataHex}'`,
        `recv_packet.packet_sequence='${packetData.packetSequence}'`,
        `recv_packet.packet_src_channel='${packetData.packetSrcChannel}'`,
    ];
    
    // Only use if under length limit
    if (estimateQueryLength(strategy1) <= MAX_QUERY_LENGTH) {
        strategies.push(strategy1);
        return strategies;
    }

    // Strategy 2: packet_data_hex + sequence (fallback if strategy 1 is too long)
    const strategy2 = [
        `recv_packet.packet_data_hex='${packetData.packetDataHex}'`,
        `recv_packet.packet_sequence='${packetData.packetSequence}'`,
    ];
    
    if (estimateQueryLength(strategy2) <= MAX_QUERY_LENGTH) {
        strategies.push(strategy2);
        return strategies;
    }

    // Strategy 3: sequence + channels + timeout (no hex, always fits)
    const strategy3 = [
        `recv_packet.packet_sequence='${packetData.packetSequence}'`,
        `recv_packet.packet_src_channel='${packetData.packetSrcChannel}'`,
        `recv_packet.packet_dst_channel='${packetData.packetDstChannel}'`,
        `recv_packet.packet_timeout_timestamp='${packetData.packetTimeout}'`,
    ];
    strategies.push(strategy3);

    return strategies;
}

function gatherEventPktData(event: TransactionEvents): PacketData | null {
    const getAttr = (key: string) =>
        event.attributes.find((a: { key: string }) => a.key === key)?.value || "";
    return {
        packetDataHex: getAttr("packet_data_hex"),
        packetChannelOrdering: getAttr("packet_channel_ordering"),
        packetTimeout: getAttr("packet_timeout_timestamp"),
        packetSequence: getAttr("packet_sequence"),
        packetSrcPort: getAttr("packet_src_port"),
        packetSrcChannel: getAttr("packet_src_channel"),
        packetDstPort: getAttr("packet_dst_port"),
        packetDstChannel: getAttr("packet_dst_channel"),
        packetConnectionId: getAttr("connection_id"),
    };
}

/**
 * Extract IBC packet data from a IBC MsgReceive transaction
 * @param txResponse - the transaction response to extract the IBC packet data from, specificaly the one from
 * the IBC MsgReceive, usually tied up with the IBC Client update transaction
 * @returns the IBC packet data, undefined if the IBC packet data is not found
 */
function extractIbcPacketData(txResponse: EvTransactionResponse): PacketData | null {
    if (!txResponse.tx_responses || txResponse.tx_responses.length === 0) {
        return null;
    }

    const events = txResponse.tx_responses[0].events;
    const packet = getRecvPacketEvent(events);

    if (!packet) {
        return null;
    }

    return gatherEventPktData(packet);
}

/**
 * Extract send packet data from a IBC MsgTransfer send transaction
 * @param txResponse - the transaction response to extract the send packet data from, specificaly the one from
 * the IBC MsgTransfer one
 * @returns the send packet data, undefined if the send packet data is not found
 */
function extractSendPacket(txResponse: TransactionResponse): PacketData | null {
    if (!txResponse.tx_response) {
        return null;
    }

    const events = txResponse.tx_response.events;
    const packet = getSendPacketEvent(events);

    if (!packet) {
        return null;
    }

    return gatherEventPktData(packet);
}

/**
 * Verify that receive transaction happened after source transaction
 * This prevents matching older duplicate transactions
 */
function verifyTransactionTimestamp(
    receiveTxTimestamp: string,
    sourceTxTimestamp: string,
): boolean {
    try {
        const receiveTime = new Date(receiveTxTimestamp).getTime();
        const sourceTime = new Date(sourceTxTimestamp).getTime();
        
        // Receive must happen after send (with small tolerance for clock skew)
        // Allow up to 1 second before source due to clock differences
        return receiveTime >= sourceTime - 1000;
    } catch {
        // If timestamp parsing fails, skip verification
        return true;
    }
}

/**
 * A function that allows to query transaction by events, in this case the ones from the IBC Message Transfer
 * Uses a tiered query strategy to handle query length limits
 * @param destChain - the destination chain to query the IBC receive transaction on
 * @param packetData - the packet data to query the IBC receive transaction on
 * @param sourceTxTimestamp - optional timestamp from source tx to verify we get the right one
 * @returns the IBC receive transaction response, null if not found yet, TransactionResponseError if error
 */
async function queryIbcReceive(
    destChain: ClientChain,
    packetData: PacketData,
    sourceTxTimestamp?: string,
): Promise<EvTransactionResponse | TransactionResponseError | null> {
    const apiUrl = await getRandomHealthyApiImperative(destChain.id);
    if (!apiUrl) {
        return {
            code: 400,
            message: "No healthy API found for chain",
            details: ["No healthy API found for chain"],
        };
    }

    // Build tiered query strategies (most specific first)
    const strategies = buildQueryStrategies(packetData);

    // Try each strategy until we find a match
    for (let i = 0; i < strategies.length; i++) {
        const queries = strategies[i];
        const queryLength = estimateQueryLength(queries);

        clientLogger.info("Trying query strategy", {
            strategyIndex: i + 1,
            totalStrategies: strategies.length,
            queryLength,
            queries,
            queriesJoined: queries.join(" AND "),
            lastQuery: queries[queries.length - 1],
            lastQueryEndsWithQuote: queries[queries.length - 1]?.endsWith("'"),
        });

        const result = await fetchTransactionByEvents(destChain, {
            queries: queries,
            limit: 1,
        });

        // If we got a valid result, verify it
        if (result && "tx_responses" in result && result.tx_responses) {
            // Verify packet data matches
            const receivedPacketData = extractIbcPacketData(result);
            if (!receivedPacketData || !verifyPacketData(receivedPacketData, packetData)) {
                clientLogger.warn("Packet data mismatch, trying next strategy");
                continue;
            }

            // Verify timestamp if provided (prevents matching old duplicate txs)
            if (sourceTxTimestamp) {
                if (!verifyTransactionTimestamp(result.tx_responses[0].timestamp, sourceTxTimestamp)) {
                    clientLogger.warn("Transaction timestamp is before source tx, skipping", {
                        receiveTxTime: result.tx_responses[0].timestamp,
                        sourceTxTime: sourceTxTimestamp,
                    });
                    // Return null to keep polling for the correct transaction
                    return null;
                }
            }

            // Found a valid match!
            return result;
        }

        // If null (not found) or error, try next strategy
        if (result === null) {
            // Transaction not found with this strategy, try next
            continue;
        }

        // If error and this is the last strategy, return the error
        if (i === strategies.length - 1) {
            return result;
        }
        return null;
    }

    // No strategy found a result
    return null;
}

/**
 * Verify packet data matches by comparing hex representation
 * This is more reliable than comparing all fields individually
 */
function verifyPacketData(
    receivedPacketData: PacketData,
    expectedPacketData: PacketData,
): boolean {
    // Primary verification: compare hex data (most reliable)
    if (receivedPacketData.packetDataHex !== expectedPacketData.packetDataHex) {
        return false;
    }

    // Secondary verification: check sequence and channels match
    return (
        receivedPacketData.packetSequence === expectedPacketData.packetSequence &&
        receivedPacketData.packetSrcChannel === expectedPacketData.packetSrcChannel &&
        receivedPacketData.packetDstChannel === expectedPacketData.packetDstChannel
    );
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
                const receiveResult = await queryIbcReceive(destChain, packetData, sourceTxTimestamp);

                clientLogger.info("receive IBC data confirmation", receiveResult);

                // Handle the three possible outcomes
                if (receiveResult === null) {
                    // Transaction not found yet - poll again
                    clientLogger.info("Transaction not found yet, will retry", { attempt, maxAttempts });
                } else if ("code" in receiveResult && "message" in receiveResult) {
                    // API error occurred (bad params, server error, etc.)
                    // Continue polling - might be temporary issue or transaction just not indexed
                    clientLogger.warn("Received error response, retrying", {
                        attempt,
                        maxAttempts,
                        code: receiveResult.code,
                        message: receiveResult.message,
                    });
                } else if (isEvTransactionResponse(receiveResult) && receiveResult.tx_responses.length > 0) {
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
