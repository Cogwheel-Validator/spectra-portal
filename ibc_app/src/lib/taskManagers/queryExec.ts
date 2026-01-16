import {
    getAcknowledgePacketEvent,
    getRecvPacketEvent,
    type IbcDenomTraceResponse,
    type TransactionEvents,
    type TransactionResponse,
} from "@/components/modules/cosmosApiData";
import type { ClientChain } from "@/components/modules/tomlTypes";
import {
    getIbcDenomTrace,
    useGetTransactionByEvents,
    useGetTransactionByHash,
} from "../apiQueries/fetchApiData";

/**
 * Get the IBC event for the transaction
 * @param chainId - the chain id of the chain that the transaction is on
 * @param hash - the hash of the transaction
 * @param action - the action of the transaction, either "sending" or "receiving"
 * @returns the IBC event for the transaction, undefined if the transaction is not found
 */
export function useGetTxIbcData(
    chainId: string,
    hash: string,
    // should be mostly a sending in this usecase, but let's leave the option for receiving
    action: "sending" | "receiving",
): TransactionEvents | undefined {
    const response = useGetTransactionByHash(chainId, { hash: hash });
    if (response.isSuccess && action === "sending") {
        // get the IBC event for acknowledge_packet
        const event = getAcknowledgePacketEvent(response.data.tx_responses[0].events);
        return event;
    } else if (response.isSuccess && action === "receiving") {
        // get the IBC event for recv_packet
        const event = getRecvPacketEvent(response.data.tx_responses[0].events);
        return event;
    } else {
        return undefined;
    }
}

/**
 * Get the transaction response for the confirmation of the transaction
 * @param innerChainId - the chain id of the chain that the transaction started from
 * @param chainConfig - the chain config of the chain that the transaction is being confirmed on
 * @param sender - the sender of the transaction
 * @param amount - the amount of the transaction
 * @param receiver - the receiver of the transaction
 * @param tokenDenom - the denom of the token that the transaction is being confirmed on
 * @param timeoutHeight - the timeout height of the transaction
 * @returns the transaction response, undefined if the transaction is not found
 */
export function useGetConfirmationTx(
    // inner in a way tay the chain config used here is for the chain we want to confirm it, and this chain id is
    // from the chain the transaction started, hence the naming "inner"
    innerChainId: string,
    chainConfig: ClientChain,
    // this could be set up more nicely but better to be explicit on what is required here
    sender: string,
    amount: string,
    receiver: string,
    tokenDenom: string,
    timeoutHeight: {
        revisionHeight: string;
        revisionNumber: string;
    },
    limit: number = 1,
): TransactionResponse | undefined {
    // first get the IBC denom trace if the token denom starts with the ibc, else we assume it is a native token
    let denomTrace: IbcDenomTraceResponse | undefined;
    if (tokenDenom.startsWith("ibc/")) {
        const denomTraceResponse = getIbcDenomTrace(innerChainId, tokenDenom);
        if (denomTraceResponse.isSuccess) {
            denomTrace = denomTraceResponse.data;
        }
    }

    // Validate receiver address format
    const isValidReceiver = receiver.startsWith(chainConfig.bech32_prefix);

    // build the queries array
    const queries: string[] = isValidReceiver
        ? [
              `fungible_token_packet.sender='${sender}'`,
              `fungible_token_packet.amount='${amount}'`,
              `fungible_token_packet.receiver='${receiver}'`,
              `fungible_token_packet.denom='${denomTrace?.denom_trace.base_denom ?? tokenDenom}'`,
              `recv_packet.packet_timeout_height='${timeoutHeight.revisionNumber}-${timeoutHeight.revisionHeight}'`,
          ]
        : [];

    const response = useGetTransactionByEvents(chainConfig, { queries: queries, limit: limit });

    // Return early if validation failed
    if (!isValidReceiver) {
        return undefined;
    }

    if (response.isSuccess) {
        return response.data;
    } else {
        return undefined;
    }
}
