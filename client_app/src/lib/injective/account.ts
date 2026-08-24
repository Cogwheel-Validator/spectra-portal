import { encodeEthSecp256k1Pubkey } from "@cosmjs/amino";
import type { Account, AccountParser } from "@cosmjs/stargate";
import { accountFromAny } from "@cosmjs/stargate";
import type { Any } from "cosmjs-types/google/protobuf/any";
import { PubKey as EthSecp256k1PubKey } from "@/lib/generated/injective/injective/crypto/v1beta1/ethsecp256k1/keys";
import { EthAccount } from "@/lib/generated/injective/injective/types/v1beta1/account";

export const ETH_ACCOUNT_TYPE_URL = "/injective.types.v1beta1.EthAccount";
export const ETHSECP256K1_PUBKEY_TYPE_URL = "/injective.crypto.v1beta1.ethsecp256k1.PubKey";

/**
 * cosmjs's default AccountParser can't parse Injective's EthAccount.
 * So client.getAccount()/getSequence() throw "Unsupported type" for it.
 * Unwrap it to a standard cosmjs Account here instead.
 * @param input The Any object to parse
 * @returns The parsed Account
 */
export const injectiveAccountParser: AccountParser = (input: Any): Account => {
    if (input.typeUrl !== ETH_ACCOUNT_TYPE_URL) {
        return accountFromAny(input);
    }

    const { baseAccount } = EthAccount.decode(input.value);
    if (!baseAccount) {
        throw new Error("EthAccount is missing base_account");
    }

    const pubkey =
        baseAccount.pubKey && baseAccount.pubKey.typeUrl === ETHSECP256K1_PUBKEY_TYPE_URL
            ? encodeEthSecp256k1Pubkey(EthSecp256k1PubKey.decode(baseAccount.pubKey.value).key)
            : null;

    return {
        address: baseAccount.address,
        pubkey,
        accountNumber: baseAccount.accountNumber,
        sequence: baseAccount.sequence,
    };
};
