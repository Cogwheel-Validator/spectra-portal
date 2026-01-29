import { z } from "zod";
import type { Coin } from "@/lib/generated/osmosis/cosmos/base/v1beta1/coin";
import type { SwapAmountInRoute } from "@/lib/generated/osmosis/osmosis/poolmanager/v1beta1/swap_route";

// ===================================================================
// All of the necessary requests and responses for the Cosmos REST API
// ===================================================================

// Address Spendable Balance Request and Response

// Address Spendable Balance Request and Response
export type AddressSpendableBalanceRequest = {
    address: string;
    offset: number;
    limit: number;
    countTotal: boolean;
};

// Address Spendable Balance Response Schema
export const AddressSpendableBalanceResponseSchema = z.object({
    balances: z.array(
        z.object({
            denom: z.string(),
            amount: z.string(),
        }),
    ),
    pagination: z.object({
        next_key: z.null().optional(),
        total: z.string(),
    }),
});

export type AddressSpendableBalanceResponse = z.infer<typeof AddressSpendableBalanceResponseSchema>;

// Transaction Response and Request

// Transaction Request by Hash and Events
export type TransactionRequestByHash = {
    hash: string;
};

// Transaction Request by Events
export type TransactionRequestByEvents = {
    // technically for sdk versions < v0.47 the term is events
    // for > v0.50 the term is queries but the functionality is the same
    // formating is only different
    queries: string[];
    limit: number;
};

// Transaction Events Schema
export const TransactionEventsSchema = z.object({
    type: z.string(),
    attributes: z.array(z.object({ key: z.string(), value: z.string(), index: z.boolean() })),
});

export type TransactionEvents = z.infer<typeof TransactionEventsSchema>;

// Transaction Message Schema
export const TransactionMessageSchema = z.looseObject({
    "@type": z.string(),
    // there are more data depending on which message type it is
    // will add more data if needed
});

export type TransactionMessage = z.infer<typeof TransactionMessageSchema>;

// Transaction Response Schema
export const TransactionResponseSchema = z.object({
    tx_response: z.looseObject({
        height: z.string(),
        txhash: z.string(),
        codespace: z.string(),
        code: z.number(),
        raw_log: z.string(),
        info: z.string(),
        gas_wanted: z.string(),
        gas_used: z.string(),
        tx: z.looseObject({
            "@type": z.string(),
            body: z.looseObject({
                messages: z.array(TransactionMessageSchema),
            }),
            auth_info: z.any().optional(),
            signatures: z.array(z.string()).optional(),
        }),
        timestamp: z.string(),
        events: z.array(TransactionEventsSchema),
    }),
});

export type TransactionResponse = z.infer<typeof TransactionResponseSchema>;

// Transaction Response Error Schema
export const TransactionResponseErrorSchema = z.object({
    code: z.number(),
    message: z.string(),
    details: z.array(z.string()),
});

export type TransactionResponseError = z.infer<typeof TransactionResponseErrorSchema>;

// Transaction Not Found Response - empty result transaction not yet indexed or invalid parameters
export const TransactionNotFoundResponseSchema = z.object({
    txs: z.array(z.unknown()).refine((arr) => arr.length === 0),
    tx_responses: z.array(z.unknown()).refine((arr) => arr.length === 0),
    pagination: z.null(),
    total: z.string().refine((s) => s === "0"),
});

export type TransactionNotFoundResponse = z.infer<typeof TransactionNotFoundResponseSchema>;

/**
 * Check if a response is a "transaction not found yet" response
 * This indicates the API is healthy but the transaction hasn't been indexed
 * @param data - The data to check
 * @returns true if the response indicates transaction not found yet
 */
export function isTransactionNotFound(data: unknown): data is TransactionNotFoundResponse {
    try {
        TransactionNotFoundResponseSchema.parse(data);
        return true;
    } catch {
        return false;
    }
}

/**
 * Type guard to check if a result is a TransactionResponseError
 * @param result - The result to check
 * @returns true if the result is an error response
 */
export function isTransactionResponseError(
    result: TransactionResponse | TransactionResponseError | null,
): result is TransactionResponseError {
    return result !== null && "code" in result && !("tx_response" in result);
}

/**
 * Type guard to check if a result is an EvTransactionResponse
 * @param result - The result to check
 * @returns true if the result is an EvTransactionResponse
 */
export function isEvTransactionResponse(
    result: TransactionResponse | EvTransactionResponse | null,
): result is EvTransactionResponse {
    return result !== null && "tx_responses" in result && result.tx_responses.length > 0;
}

export const IbcDenomTraceResponseSchema = z.object({
    denom_trace: z.object({
        path: z.string(),
        base_denom: z.string(),
    }),
});

export type IbcDenomTraceResponse = z.infer<typeof IbcDenomTraceResponseSchema>;

/**
 * IBC message transfer details
 */
export type IbcMsgTransferDetails = {
    sourceChannel: string;
    sourcePort: string;
    sender: string;
    receiver: string;
    token: {
        amount: string;
        denom: string;
    };
    timeoutHeight: {
        revisionHeight: string;
        revisionNumber: string;
    };
};

// ===================================================================
// Utility functions to get the data from the responses
// ===================================================================

/**
 * Get the hashes of all transactions in the response
 * @param tx The transaction response
 * @returns The hashes of all transactions in the response
 */
export function getTxHashes(tx: TransactionResponse): string[] {
    return [tx.tx_response.txhash];
}

/**
 * Internal function to get the transaction data by hash
 * @param tx The transaction response
 * @param hash The hash of the transaction
 * @returns The transaction data for the transaction, undefined if the transaction is not found
 */
function getTransactionDataByHash(
    tx: TransactionResponse,
    hash: string,
): TransactionResponse["tx_response"] | undefined {
    return tx.tx_response.txhash === hash ? tx.tx_response : undefined;
}

/**
 * Get the messages for a specific transaction by hash
 * @param tx The transaction response
 * @param hash The hash of the transaction
 * @returns The messages array for the transaction, undefined if the transaction is not found
 */
export function getTxMessages(
    tx: TransactionResponse,
    hash: string,
): TransactionMessage[] | undefined {
    const txData = getTransactionDataByHash(tx, hash);
    if (!txData) {
        return undefined;
    }
    return txData.tx.body.messages as TransactionMessage[];
}

/**
 * Get the height of the transaction
 * @param tx The transaction response
 * @returns The height of the transaction, undefined if the transaction is not found
 */
export function getTxHeight(tx: TransactionResponse): string {
    // it should always be the same for all transactions in the response
    return tx.tx_response.height;
}

/**
 * Get the timestamp of the transaction
 * @param tx The transaction response
 * @returns The timestamp of the transaction, undefined if the transaction is not found
 */
export function getTxTimestamp(tx: TransactionResponse): string {
    // it should always be the same for all transactions in the response
    return tx.tx_response.timestamp;
}

/**
 * Get the events for a specific transaction by hash
 * @param tx The transaction response
 * @param hash The hash of the transaction
 * @returns The events array for the transaction, undefined if the transaction is not found
 */
export function getTxEvents(
    tx: TransactionResponse,
    hash: string,
): TransactionEvents[] | undefined {
    const txData = getTransactionDataByHash(tx, hash);
    if (!txData) {
        return undefined;
    }
    return txData.events as TransactionEvents[];
}

/**
 * Get the message types for a specific transaction by hash
 * @param tx The transaction response
 * @param hash The hash of the transaction
 * @returns The array containing the message types for the transaction, empty array if the transaction is not found
 */
export function getTxMessageTypes(tx: TransactionResponse, hash: string): string[] {
    const txData = getTxMessages(tx, hash);
    if (!txData) {
        return [];
    }
    return txData.map((message) => message["@type"]);
}

/**
 * Get the fungible token packet from the events
 * @param events The events array
 * @returns The fungible token packet, undefined if the fungible token packet is not found
 */
export function getFungibleTokenPacket(events: TransactionEvents[]): TransactionEvents | undefined {
    // fungible token packet is the event that contains the fungible token packet data
    return events.find((event) => event.type === "fungible_token_packet") as
        | TransactionEvents
        | undefined;
}

/**
 * Get the acknowledge packet event from the events
 *
 * Used only if the tx message contains /ibc.core.client.v1.MsgUpdateClient message type and
 * the /ibc.core.channel.v1.MsgAcknowledgement message type
 * @param events The events array
 * @returns The acknowledge packet event, undefined if the acknowledge packet event is not found
 */
export function getAcknowledgePacketEvent(
    events: TransactionEvents[],
): TransactionEvents | undefined {
    // acknowledge_packet is usually tied to the IBC message acknowledgment
    return events.find((event) => event.type === "acknowledge_packet") as
        | TransactionEvents
        | undefined;
}

/**
 * Get the recv packet event from the events
 * @param events The events array
 * @returns The recv packet event, undefined if the recv packet event is not found
 */
export function getRecvPacketEvent(events: TransactionEvents[]): TransactionEvents | undefined {
    // recv_packet is usually tied to the IBC message receive
    return events.find((event) => event.type === "recv_packet") as TransactionEvents | undefined;
}

/**
 * Get the fungible token packet event from the events
 * @param events The events array
 * @returns The fungible token packet event, undefined if the fungible token packet event is not found
 */
export function getFungibleTokenPacketEvent(
    events: TransactionEvents[],
): TransactionEvents | undefined {
    // fungible token packet is the event that contains the fungible token packet data
    return events.find((event) => event.type === "fungible_token_packet") as
        | TransactionEvents
        | undefined;
}

/**
 * Get the send packet event from the events
 * @param events The events array
 * @returns The send packet event, undefined if the send packet event is not found
 */
export function getSendPacketEvent(events: TransactionEvents[]): TransactionEvents | undefined {
    // send_packet is usually tied to the IBC message send
    return events.find((event) => event.type === "send_packet") as TransactionEvents | undefined;
}

/**
 * Get the swap in message from the messages array
 * @param messages The messages array
 * @returns The swap in message, undefined if the swap in message is not found
 */
export function getSwapInMessage(messages: TransactionMessage[]): TransactionMessage | undefined {
    return messages.find(
        (message) => message["@type"] === "/osmosis.poolmanager.v1beta1.MsgSwapExactAmountIn",
    ) as TransactionMessage | undefined;
}

/**
 * Swap in message details
 */
export type SwapInMessageDetails = {
    sender: string;
    routes: SwapAmountInRoute[];
    tokenIn: Coin;
    tokenOutMinAmount: string;
};

/**
 * Get the swap in message details from the messages array
 * @param messages The messages array
 * @returns The swap in message details, undefined if the swap in message details is not found
 */
export function getSwapInMessageDetails(
    messages: TransactionMessage[],
): SwapInMessageDetails | undefined {
    const swapInMessage = getSwapInMessage(messages);
    if (!swapInMessage) {
        return undefined;
    }

    const routes = swapInMessage.routes as { pool_id: string; token_out_denom: string }[];
    const tokenIn = swapInMessage.token_in as { denom: string; amount: string };
    return {
        sender: swapInMessage.sender,
        routes: routes.map((route) => ({
            poolId: Number(route.pool_id),
            tokenOutDenom: route.token_out_denom,
        })),
        tokenIn: {
            denom: tokenIn.denom,
            amount: tokenIn.amount,
        },
        tokenOutMinAmount: swapInMessage.token_out_min_amount,
    } as SwapInMessageDetails;
}

/**
 * Get the IBC message transfer details from the messages array
 * @param messages The messages array
 * @returns The IBC message transfer details, undefined if the IBC message transfer details is not found
 *
 * This transaction is usually first sent before the relayers acknowladge and receive the packets
 */
export function getIbcMsgTransferDetails(
    messages: TransactionMessage[],
): IbcMsgTransferDetails | undefined {
    const ibcMsgTransferMessage = messages.find(
        (message) => message["@type"] === "/ibc.applications.transfer.v1.MsgTransfer",
    ) as TransactionMessage | undefined;
    if (!ibcMsgTransferMessage) {
        return undefined;
    }
    const timeoutHeight = ibcMsgTransferMessage.timeout_height as {
        revision_height: string;
        revision_number: string;
    };
    return {
        sender: ibcMsgTransferMessage.sender as string,
        receiver: ibcMsgTransferMessage.receiver as string,
        sourceChannel: ibcMsgTransferMessage.source_channel as string,
        sourcePort: ibcMsgTransferMessage.source_port as string,
        token: {
            amount: ibcMsgTransferMessage.amount as string,
            denom: ibcMsgTransferMessage.denom as string,
        },
        timeoutHeight: {
            revisionHeight: timeoutHeight.revision_height as string,
            revisionNumber: timeoutHeight.revision_number as string,
        },
    } as IbcMsgTransferDetails;
}

// Transaction response schema gathered from REST API by using events for query
export const EvTransactionResponseSchema = z.looseObject({
    tx_responses: z.array(
        z.object({
            height: z.string(),
            txhash: z.string(),
            codespace: z.string(),
            code: z.number(),
            raw_log: z.string(),
            info: z.string(),
            gas_wanted: z.string(),
            gas_used: z.string(),
            tx: z.looseObject({
                "@type": z.string(),
                body: z.looseObject({ messages: z.array(TransactionMessageSchema) }),
            }),
            timestamp: z.string(),
            events: z.array(
                z.object({
                    type: z.string(),
                    attributes: z.array(
                        z.object({ key: z.string(), value: z.string(), index: z.boolean() }),
                    ),
                }),
            ),
        }),
    ),
    pagination: z.union([
        z.object({ next_key: z.string().nullable().optional(), total: z.string() }),
        z.null(),
    ]),
    total: z.string(),
});

export type EvTransactionResponse = z.infer<typeof EvTransactionResponseSchema>;
