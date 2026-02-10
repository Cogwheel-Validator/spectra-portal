import type {
    EvTransactionResponse,
    TransactionEvents,
    TransactionResponse,
    TransactionResponseError,
} from "@/components/modules/cosmosApiData";
import { getRecvPacketEvent, getSendPacketEvent } from "@/components/modules/cosmosApiData";
import type { ClientChain } from "@/components/modules/tomlTypes";
import { getRandomHealthyApiImperative } from "@/lib/apiQueries/featchHealthyEndpoint";
import { fetchTransactionByEvents } from "@/lib/apiQueries/fetchApiData";
import clientLogger from "@/lib/clientLogger";
import type { PacketData } from "./types";

/**
 * Maximum safe query length (accounting for AND operators between queries)
 * Cosmos SDK appears to limit queries to ~500 chars total
 */
const MAX_QUERY_LENGTH = 450;

/**
 * Estimate the total length of a query array when joined with ' AND '
 */
export function estimateQueryLength(queries: string[]): number {
    return queries.join(" AND ").length;
}

/**
 * Build tiered query strategies for finding IBC receive transaction
 * Returns queries in order of preference (most specific first)
 */
export function buildQueryStrategies(packetData: PacketData): string[][] {
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

export function gatherEventPktData(event: TransactionEvents): PacketData | null {
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
export function extractIbcPacketData(txResponse: EvTransactionResponse): PacketData | null {
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
export function extractSendPacket(txResponse: TransactionResponse): PacketData | null {
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
export function verifyTransactionTimestamp(
    receiveTxTimestamp: string,
    sourceTxTimestamp: string,
): boolean {
    try {
        const receiveTime = new Date(receiveTxTimestamp).getTime();
        const sourceTime = new Date(sourceTxTimestamp).getTime();

        // Receive must happen after send (with small tolerance for clock skew)
        // Allow up to 10 second before source due to clock differences
        return receiveTime >= sourceTime - 10000;
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
export async function queryIbcReceive(
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
                if (
                    !verifyTransactionTimestamp(result.tx_responses[0].timestamp, sourceTxTimestamp)
                ) {
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
export function verifyPacketData(
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
