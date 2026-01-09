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
        nextKey: z.string(),
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
            tx: z.object({
                type_url: z.string(),
                body: z.object({
                    messages: z.array(TransactionMessageSchema),
                }),
            }),
            timestamp: z.string(),
            events: z.array(TransactionEventsSchema),
        }),
    ),
    pagination: z.object({ next_key: z.string(), total: z.string() }),
    total: z.string(),
});

export type TransactionResponse = z.infer<typeof TransactionResponseSchema>;

// ===================================================================
// Utility functions to get the data from the responses
// ===================================================================

/**
 * Get the hashes of all transactions in the response
 * @param tx The transaction response
 * @returns The hashes of all transactions in the response
 */
export function getTxHashes(tx: TransactionResponse): string[] {
    return tx.tx_responses.map((tx) => tx.txhash);
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
): TransactionResponse["tx_responses"][number] | undefined {
    return tx.tx_responses.find((tx) => tx.txhash === hash) ?? undefined;
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
    return tx.tx_responses[0].height;
}

/**
 * Get the timestamp of the transaction
 * @param tx The transaction response
 * @returns The timestamp of the transaction, undefined if the transaction is not found
 */
export function getTxTimestamp(tx: TransactionResponse): string {
    // it should always be the same for all transactions in the response
    return tx.tx_responses[0].timestamp;
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
 * Get the swap in message from the messages array
 * @param messages The messages array
 * @returns The swap in message, undefined if the swap in message is not found
 */
export function getSwapInMessage(messages: TransactionMessage[]): TransactionMessage | undefined {
    return messages.find(
        (message) => message["@type"] === "/osmosis.poolmanager.v1beta1.MsgSwapExactAmountIn",
    ) as TransactionMessage | undefined;
}

export type SwapInMessageDetails = {
    sender: string;
    routes: SwapAmountInRoute[];
    tokenIn: Coin;
    tokenOutMinAmount: string;
};

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
