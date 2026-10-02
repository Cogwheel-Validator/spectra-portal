import type { StdFee } from "@cosmjs/amino";
import { fromBase64 } from "@cosmjs/encoding";
import { type EncodeObject, makeAuthInfoBytes, type Registry } from "@cosmjs/proto-signing";
import {
    type AminoTypes,
    calculateFee,
    type DeliverTxResponse,
    type GasPrice,
    isMsgTransferEncodeObject,
    QueryClient,
    StargateClient,
    setupIbcExtension,
} from "@cosmjs/stargate";
import { connectComet } from "@cosmjs/tendermint-rpc";
import type { Keplr } from "@keplr-wallet/types";
import { SignMode } from "cosmjs-types/cosmos/tx/signing/v1beta1/signing";
import { TxBody, TxRaw } from "cosmjs-types/cosmos/tx/v1beta1/tx";
import { Any } from "cosmjs-types/google/protobuf/any";
import type { Height } from "cosmjs-types/ibc/core/client/v1/client";
import { ClientState as TendermintClientState } from "cosmjs-types/ibc/lightclients/tendermint/v1/tendermint";
import { ExtensionOptionsWeb3Tx } from "@/lib/generated/injective/injective/types/v1beta1/tx_ext";
import { injectiveAccountParser } from "@/lib/injective/account";
import { getEip712TypedData } from "@/lib/injective/eip712";
import { encodeEthermintPubkeyAny, simulateEthermintTx } from "@/lib/injective/tx";

const EXTENSION_OPTIONS_WEB3_TX_TYPE_URL = "/injective.types.v1beta1.ExtensionOptionsWeb3Tx";

// Added to the counterparty client's latest height to form the MsgTransfer timeout
// height. Large on purpose: the timeout timestamp stays the real timeout, and a client
// lagging this far behind (~115 days at 1s blocks) is past any trusting period anyway.
const EIP712_TIMEOUT_HEIGHT_OFFSET = 10_000_000n;

const TENDERMINT_CLIENT_STATE_TYPE_URL = "/ibc.lightclients.tendermint.v1.ClientState";

/**
 * Replaces zero timeout heights on MsgTransfer messages with `latestHeight + offset`.
 *
 * Injective builds the EIP-712 types for a Ledger tx from the Go structs, which always
 * include `timeout_height.revision_number/revision_height` (uint64), but its Amino JSON
 * leaves zero values out. The chain then fails to hash its own typed data
 * ("invalid integer value <nil>/<nil> for type uint64"), so a timeout height can only
 * verify when *both* fields are non-zero.
 *
 * That includes the revision number. A counterparty whose chain id has no `-N` suffix
 * (e.g. `celestia`) is on revision 0, so its height is `{0, h}`. For those we use
 * revision 1 instead: IBC compares revision numbers first, so `{0, h}` never reaches
 * `{1, x}`. The height timeout then never fires and the timeout timestamp governs.
 * @param messages - tx messages
 * @param latestHeights - counterparty client latest height per `port/channel`
 * @returns messages with MsgTransfer timeout heights filled in
 */
export function withEip712TimeoutHeights(
    messages: EncodeObject[],
    latestHeights: Map<string, Height>,
): EncodeObject[] {
    return messages.map((message) => {
        if (!isMsgTransferEncodeObject(message)) {
            return message;
        }
        const { timeoutHeight, sourcePort, sourceChannel } = message.value;
        if (timeoutHeight && (timeoutHeight.revisionHeight || timeoutHeight.revisionNumber)) {
            if (!timeoutHeight.revisionHeight || !timeoutHeight.revisionNumber) {
                throw new Error(
                    "MsgTransfer timeout height needs a non-zero revision number and height to be signed with EIP-712",
                );
            }
            return message;
        }
        const latest = latestHeights.get(`${sourcePort}/${sourceChannel}`);
        if (!latest) {
            throw new Error(`No client height for ${sourcePort}/${sourceChannel}`);
        }
        return {
            ...message,
            value: {
                ...message.value,
                timeoutHeight: {
                    revisionNumber: latest.revisionNumber || 1n,
                    revisionHeight: latest.revisionHeight + EIP712_TIMEOUT_HEIGHT_OFFSET,
                },
            },
        };
    });
}

/**
 * Reads the latest height out of a channel's counterparty client state.
 * @param key - `port/channel`, for error messages
 * @param clientState - the client state Any from the channel client state query
 * @returns the client's latest height
 */
export function latestHeightFromClientState(key: string, clientState: Any | undefined): Height {
    if (!clientState) {
        throw new Error(`No client state found for ${key}`);
    }
    // Other client types (e.g. 08-wasm) would decode into garbage without an error.
    // TODO: reaserch about this behavior. Qodo did mention it could cause some problem
    // for now the solution is to limit it to tendermint only.
    if (clientState.typeUrl !== TENDERMINT_CLIENT_STATE_TYPE_URL) {
        throw new Error(
            `Unsupported light client ${clientState.typeUrl} for ${key}, can't build a Ledger timeout height`,
        );
    }
    const { latestHeight } = TendermintClientState.decode(clientState.value);
    if (!latestHeight?.revisionHeight) {
        throw new Error(`Client state for ${key} has no latest height`);
    }
    return latestHeight;
}

/**
 * Queries the latest height of the counterparty light client behind each MsgTransfer's
 * source channel.
 * @param rpcEndpoint - RPC endpoint of the sending chain
 * @param messages - tx messages
 * @returns latest height per `port/channel`
 */
async function queryCounterpartyHeights(
    rpcEndpoint: string,
    messages: EncodeObject[],
): Promise<Map<string, Height>> {
    const heights = new Map<string, Height>();
    const transfers = messages.filter(isMsgTransferEncodeObject);
    if (transfers.length === 0) {
        return heights;
    }

    const cometClient = await connectComet(rpcEndpoint);
    try {
        const queryClient = QueryClient.withExtensions(cometClient, setupIbcExtension);
        for (const { value } of transfers) {
            const port = value.sourcePort ?? "transfer";
            const channel = value.sourceChannel ?? "";
            const key = `${port}/${channel}`;
            if (heights.has(key)) {
                continue;
            }
            const response = await queryClient.ibc.channel.clientState(port, channel);
            heights.set(
                key,
                latestHeightFromClientState(key, response.identifiedClientState?.clientState),
            );
        }
    } finally {
        cometClient.disconnect();
    }
    return heights;
}

export interface EthermintLedgerTxParams {
    wallet: Keplr;
    chainId: string;
    address: string;
    pubkeyBytes: Uint8Array;
    evmChainId: number;
    messages: EncodeObject[];
    memo: string;
    registry: Registry;
    aminoTypes: AminoTypes;
    rpcEndpoint: string;
    fee: StdFee | "auto";
    gasAdjustment: number;
    gasPrice: GasPrice;
}

/**
 * Signs and broadcasts a tx for an ethermint (Injective) account connected
 * through a Ledger device via Keplr. Ledger's Ethereum app can't sign an
 * arbitrary protobuf digest, only raw messages or EIP-712 typed data.
 *
 * @param params - a EthermintLedgerTxParams interface containing all of the transaction parameters
 * @returns {Promise<DeliverTxResponse>} - response from the tx broadcast
 */
export async function sendEthermintLedgerTransaction(
    params: EthermintLedgerTxParams,
): Promise<DeliverTxResponse> {
    const {
        wallet,
        chainId,
        address,
        pubkeyBytes,
        evmChainId,
        memo,
        registry,
        aminoTypes,
        rpcEndpoint,
        fee,
        gasAdjustment,
        gasPrice,
    } = params;

    const queryClient = await StargateClient.connect(rpcEndpoint, {
        accountParser: injectiveAccountParser,
    });
    const account = await queryClient.getAccount(address);
    if (!account) {
        throw new Error("Could not retrieve account details for signing.");
    }

    const messages = withEip712TimeoutHeights(
        params.messages,
        await queryCounterpartyHeights(rpcEndpoint, params.messages),
    );
    const aminoMsgs = messages.map((message) => aminoTypes.toAmino(message));

    let finalFee: StdFee;
    if (fee === "auto") {
        const gasEstimated = await simulateEthermintTx(
            { rpcEndpoint, registry, signerAddress: address, pubkeyBytes, messages, memo },
            account.sequence,
        );
        finalFee = calculateFee(Math.round(gasEstimated * gasAdjustment), gasPrice);
    } else {
        finalFee = fee;
    }

    // Timeout by time, not by height. Still we need to specify it as 0.
    const timeoutHeight = 0;

    const eip712TypedData = getEip712TypedData({
        aminoMsgs,
        accountNumber: account.accountNumber,
        sequence: account.sequence,
        timeoutHeight,
        chainId,
        memo,
        fee: finalFee,
        evmChainId,
    });

    const stdSignDoc = {
        chain_id: chainId,
        timeout_height: timeoutHeight.toString(),
        account_number: account.accountNumber.toString(),
        sequence: account.sequence.toString(),
        fee: finalFee,
        msgs: aminoMsgs,
        memo: memo || "",
    };

    const { signed, signature } = await wallet.experimentalSignEIP712CosmosTx_v0(
        chainId,
        address,
        eip712TypedData,
        stdSignDoc,
    );

    const signedFee: StdFee = { amount: [...signed.fee.amount], gas: signed.fee.gas };
    const bodyBytes = registry.encodeTxBody({ messages, memo: signed.memo });
    const authInfoBytes = makeAuthInfoBytes(
        [{ pubkey: encodeEthermintPubkeyAny(pubkeyBytes), sequence: Number(signed.sequence) }],
        signedFee.amount,
        Number(signedFee.gas),
        undefined,
        undefined,
        SignMode.SIGN_MODE_LEGACY_AMINO_JSON,
    );

    const txBody = TxBody.decode(bodyBytes);
    txBody.extensionOptions = [
        Any.fromPartial({
            typeUrl: EXTENSION_OPTIONS_WEB3_TX_TYPE_URL,
            value: ExtensionOptionsWeb3Tx.encode(
                ExtensionOptionsWeb3Tx.fromPartial({ typedDataChainID: evmChainId }),
            ).finish(),
        }),
    ];

    const txRaw = TxRaw.fromPartial({
        bodyBytes: TxBody.encode(txBody).finish(),
        authInfoBytes,
        signatures: [fromBase64(signature.signature)],
    });

    const cometClient = await connectComet(rpcEndpoint);
    try {
        const client = StargateClient.create(cometClient, {
            accountParser: injectiveAccountParser,
        });
        return await client.broadcastTx(TxRaw.encode(txRaw).finish());
    } finally {
        cometClient.disconnect();
    }
}
