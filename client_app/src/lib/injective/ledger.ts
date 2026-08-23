import type { StdFee } from "@cosmjs/amino";
import { fromBase64 } from "@cosmjs/encoding";
import { type EncodeObject, makeAuthInfoBytes, type Registry } from "@cosmjs/proto-signing";
import {
    type AminoTypes,
    calculateFee,
    type DeliverTxResponse,
    type GasPrice,
    StargateClient,
} from "@cosmjs/stargate";
import { connectComet } from "@cosmjs/tendermint-rpc";
import type { Keplr } from "@keplr-wallet/types";
import { SignMode } from "cosmjs-types/cosmos/tx/signing/v1beta1/signing";
import { TxBody, TxRaw } from "cosmjs-types/cosmos/tx/v1beta1/tx";
import { Any } from "cosmjs-types/google/protobuf/any";
import { ExtensionOptionsWeb3Tx } from "@/lib/generated/injective/injective/types/v1beta1/tx_ext";
import { injectiveAccountParser } from "@/lib/injective/account";
import { getEip712TypedData } from "@/lib/injective/eip712";
import { encodeEthermintPubkeyAny, simulateEthermintTx } from "@/lib/injective/tx";

const EXTENSION_OPTIONS_WEB3_TX_TYPE_URL = "/injective.types.v1beta1.ExtensionOptionsWeb3Tx";

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
        messages,
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
