import { fromBase64 } from "@cosmjs/encoding";
import type { EncodeObject, OfflineDirectSigner } from "@cosmjs/proto-signing";
import { makeAuthInfoBytes, makeSignDoc, type Registry } from "@cosmjs/proto-signing";
import type { DeliverTxResponse, StdFee } from "@cosmjs/stargate";
import {
    createProtobufRpcClient,
    QueryClient,
    StargateClient,
    setupTxExtension,
} from "@cosmjs/stargate";
import { connectComet } from "@cosmjs/tendermint-rpc";
import { SignMode } from "cosmjs-types/cosmos/tx/signing/v1beta1/signing";
import { ServiceClientImpl, SimulateRequest } from "cosmjs-types/cosmos/tx/v1beta1/service";
import { AuthInfo, Fee, Tx, TxBody, TxRaw } from "cosmjs-types/cosmos/tx/v1beta1/tx";
import { Any } from "cosmjs-types/google/protobuf/any";
import { PubKey as EthSecp256k1PubKey } from "@/lib/generated/injective/injective/crypto/v1beta1/ethsecp256k1/keys";
import { injectiveAccountParser } from "@/lib/injective/account";

export const ETHSECP256K1_PUBKEY_TYPE_URL = "/injective.crypto.v1beta1.ethsecp256k1.PubKey";

// Encodes a raw 33-byte compressed eth_secp256k1 pubkey as an Injective-typed `Any`.
// @params pubkeyBytes - The raw 33-byte compressed eth_secp256k1 pubkey.
// @returns An `Any` object wrapping the encoded pubkey.
export function encodeEthermintPubkeyAny(pubkeyBytes: Uint8Array): Any {
    return Any.fromPartial({
        typeUrl: ETHSECP256K1_PUBKEY_TYPE_URL,
        value: EthSecp256k1PubKey.encode(
            EthSecp256k1PubKey.fromPartial({ key: pubkeyBytes }),
        ).finish(),
    });
}

interface EthermintTxContext {
    rpcEndpoint: string;
    registry: Registry;
    signerAddress: string;
    // Raw compressed pubkey bytes as returned by the offline signer
    pubkeyBytes: Uint8Array;
    messages: readonly EncodeObject[];
    memo: string;
}

/**
 * Gas-estimates a tx for an ethermint (Injective) account.
 * @param ctx - The transaction context.
 * @param sequence - The transaction sequence number.
 * @returns The estimated gas amount.
 */
export async function simulateEthermintTx(
    ctx: EthermintTxContext,
    sequence: number,
): Promise<number> {
    const cometClient = await connectComet(ctx.rpcEndpoint);
    try {
        const queryClient = QueryClient.withExtensions(cometClient, setupTxExtension);
        const rpc = createProtobufRpcClient(queryClient);
        const service = new ServiceClientImpl(rpc);

        const anyMsgs = ctx.messages.map((m) => ctx.registry.encodeAsAny(m));
        const pubkey = encodeEthermintPubkeyAny(ctx.pubkeyBytes);
        const tx = Tx.fromPartial({
            authInfo: AuthInfo.fromPartial({
                fee: Fee.fromPartial({}),
                signerInfos: [
                    {
                        publicKey: pubkey,
                        sequence: BigInt(sequence),
                        modeInfo: { single: { mode: SignMode.SIGN_MODE_UNSPECIFIED } },
                    },
                ],
            }),
            body: TxBody.fromPartial({ messages: Array.from(anyMsgs), memo: ctx.memo }),
            signatures: [new Uint8Array()],
        });
        const request = SimulateRequest.fromPartial({ txBytes: Tx.encode(tx).finish() });
        const response = await service.Simulate(request);
        if (!response.gasInfo) {
            throw new Error("Simulate response is missing gasInfo");
        }
        return Number(response.gasInfo.gasUsed);
    } finally {
        cometClient.disconnect();
    }
}

/**
 * Signs and broadcasts a direct-mode (SIGN_MODE_DIRECT) tx for an ethermint
 * (Injective) account, using the given offline signer's `signDirect`. This
 * should bypasses `SigningStargateClient.sign()` or `.signAndBroadcast()`
 * entirely.
 * @param ctx - The transaction context.
 * @returns {Promise<DeliverTxResponse>} The response from the transaction broadcast.
 */
export async function signAndBroadcastEthermintDirect(
    ctx: EthermintTxContext & {
        signer: OfflineDirectSigner;
        chainId: string;
        accountNumber: number;
        sequence: number;
        fee: StdFee;
    },
): Promise<DeliverTxResponse> {
    const { fee } = ctx;

    const bodyBytes = ctx.registry.encodeTxBody({ messages: ctx.messages, memo: ctx.memo });
    const authInfoBytes = makeAuthInfoBytes(
        [{ pubkey: encodeEthermintPubkeyAny(ctx.pubkeyBytes), sequence: ctx.sequence }],
        fee.amount,
        Number(fee.gas),
        undefined,
        undefined,
    );
    const signDoc = makeSignDoc(bodyBytes, authInfoBytes, ctx.chainId, ctx.accountNumber);
    const { signature, signed } = await ctx.signer.signDirect(ctx.signerAddress, signDoc);
    const txRaw = TxRaw.fromPartial({
        bodyBytes: signed.bodyBytes,
        authInfoBytes: signed.authInfoBytes,
        signatures: [fromBase64(signature.signature)],
    });

    const cometClient = await connectComet(ctx.rpcEndpoint);
    try {
        const client = StargateClient.create(cometClient, {
            accountParser: injectiveAccountParser,
        });
        return await client.broadcastTx(TxRaw.encode(txRaw).finish());
    } finally {
        cometClient.disconnect();
    }
}
