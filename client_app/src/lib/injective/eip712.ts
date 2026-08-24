// Adapted from injective-ts (Apache-2.0). See ./vendor/NOTICE.md and
// ./vendor/LICENSE-APACHE-2.0.md for the original copyright and license text.
// Source:
//   packages/sdk-ts/src/core/tx/eip712/{maps,utils,eip712}.ts
//   https://github.com/InjectiveLabs/injective-ts
//
// Changes from the original:
//   - `objectKeysToEip712Types` is copied close to verbatim (it's a generic
//     amino-object -> EIP712 type-schema mapper, not specific to any one
//     message type).
//   - `protoTypeToAminoType`, and the per-message `.toAmino()`/`.toEip712()`
//     methods it depended on, were dropped, because this app already produces the
//     same `{ type, value }` amino shape via `@cosmjs/stargate`'s
//     `AminoTypes` converter, which is used as the input.
//   - EVM chain id handling, snake_case conversion and default fee/domain
//     helpers were reimplemented locally to avoid pulling in
//     `@injectivelabs/utils`/`@injectivelabs/ts-types` for a handful helpers.

import type { StdFee } from "@cosmjs/amino";

export interface TypedDataField {
    name: string;
    type: string;
}

const numberFieldsWithStringValue = [
    "order_mask",
    "order_type",
    "oracle_type",
    "round",
    "expiration_timestamp",
    "settlement_timestamp",
    "oracle_scale_factor",
    "expiry",
    "option",
    "proposal_id",
    "creation_height",
    "contract_hook_max_gas",
    "expiration_block",
];
const stringFieldsWithNumberValue = ["timeout_timestamp", "revision_height", "revision_number"];
const stringFieldsToOmitIfEmpty = ["cid"];
const fieldsToOmitIfEmpty = ["admin_info", "order"];

function toSnakeCase(input: string): string {
    return input.replace(/[A-Z]/g, (letter) => `_${letter.toLowerCase()}`);
}

function keysToSnakeCase(object: Record<string, unknown>): Record<string, unknown> {
    const result: Record<string, unknown> = {};
    for (const key of Object.keys(object)) {
        result[toSnakeCase(key)] = object[key];
    }
    return result;
}

function snakeToPascal(input: string): string {
    return input
        .split("_")
        .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
        .join("");
}

function numberTypeToReflectionNumberType(property?: string): string {
    switch (property) {
        case "order_mask":
        case "order_type":
        case "oracle_type":
        case "option":
            return "int32";
        case "revision_number":
        case "revision_height":
        case "admin_permissions":
        case "oracle_scale_factor":
        case "exponent":
        case "round":
        case "proposal_id":
            return "uint64";
        case "expiry":
        case "creation_height":
        case "expiration_block":
            return "int64";
        default:
            return "uint64";
    }
}

function stringTypeToReflectionStringType(): string {
    return "uint64";
}

function appendTypePrefixToPropertyType(property: string, parentProperty = ""): string {
    const propertyWithoutTypePrefix = property.replace("Type", "");
    const parentPropertyWithoutTypePrefix =
        parentProperty === "MsgValue" ? "" : parentProperty.replace("Type", "");

    return `Type${parentPropertyWithoutTypePrefix + propertyWithoutTypePrefix}`;
}

/**
 * objectKeysToEip712Types infers an EIP-712 type schema from an
 * amino-JSON message value.
 * Injective's ante handler regenerates this same schema server-side
 * from the amino representation of the tx, so the shape produced here has
 * to match cosmos-sdk's amino encoding field-for-field, including the
 * numeric/string field overrides below (mainly IBC height/timeout fields).
 * @param object The amino-JSON message value to infer the schema from
 * @param primaryType The primary type of the message (default: "MsgValue")
 * @returns A Map of type names to their corresponding EIP-712 type fields
 */
export function objectKeysToEip712Types({
    object,
    primaryType = "MsgValue",
}: {
    object: Record<string, unknown>;
    messageType?: string;
    primaryType?: string;
}): Map<string, TypedDataField[]> {
    const output = new Map<string, TypedDataField[]>();
    const types: TypedDataField[] = [];
    const snakeCased = keysToSnakeCase(object);

    for (const property of Object.keys(snakeCased)) {
        const propertyValue = snakeCased[property];

        if (property === "@type") {
            continue;
        }
        // Amino converters sometimes use `omitDefault` for optional fields,
        // which sets the key to `undefined` rather than deleting it.
        // JSON.stringify drops such keys when the sign doc is
        // actually serialized/hashed, so the type schema needs to match.
        if (propertyValue === undefined) {
            continue;
        }
        if (fieldsToOmitIfEmpty.includes(property) && !propertyValue) {
            continue;
        }

        const type = typeof propertyValue;

        if (type === "boolean") {
            types.push({ name: property, type: "bool" });
        } else if (
            type === "number" ||
            type === "bigint" ||
            numberFieldsWithStringValue.includes(property)
        ) {
            types.push({ name: property, type: numberTypeToReflectionNumberType(property) });
        } else if (type === "string") {
            if (stringFieldsToOmitIfEmpty.includes(property) && !propertyValue) {
                continue;
            }
            if (stringFieldsWithNumberValue.includes(property)) {
                types.push({ name: property, type: stringTypeToReflectionStringType() });
                continue;
            }
            types.push({ name: property, type: "string" });
        } else if (type === "object" && propertyValue !== null) {
            if (Array.isArray(propertyValue) && propertyValue.length === 0) {
                throw new Error(`Array with length 0 found for field "${property}"`);
            } else if (Array.isArray(propertyValue) && propertyValue.length > 0) {
                const arrayFirstType = typeof propertyValue[0];
                const isPrimitive =
                    arrayFirstType === "boolean" ||
                    arrayFirstType === "number" ||
                    arrayFirstType === "string";

                if (isPrimitive) {
                    if (arrayFirstType === "boolean") {
                        types.push({ name: property, type: "bool[]" });
                    } else if (arrayFirstType === "number") {
                        types.push({ name: property, type: "number[]" });
                    } else {
                        types.push({ name: property, type: "string[]" });
                    }
                } else if (arrayFirstType === "object") {
                    const propertyType = appendTypePrefixToPropertyType(
                        snakeToPascal(property),
                        primaryType,
                    );
                    const recursiveOutput = objectKeysToEip712Types({
                        object: propertyValue[0] as Record<string, unknown>,
                        primaryType: propertyType,
                    });
                    const recursiveTypes = recursiveOutput.get(propertyType);

                    types.push({ name: property, type: `${propertyType}[]` });
                    if (recursiveTypes) {
                        output.set(propertyType, recursiveTypes);
                    }
                    for (const [key, value] of recursiveOutput.entries()) {
                        if (key !== primaryType) {
                            output.set(key, value);
                        }
                    }
                } else {
                    throw new Error(
                        `Array with elements of unknown type found for field "${property}"`,
                    );
                }
            } else if (propertyValue instanceof Date) {
                types.push({ name: property, type: "string" });
            } else {
                const propertyType = appendTypePrefixToPropertyType(
                    snakeToPascal(property),
                    primaryType,
                );
                const recursiveOutput = objectKeysToEip712Types({
                    object: propertyValue as Record<string, unknown>,
                    primaryType: propertyType,
                });
                const recursiveTypes = recursiveOutput.get(propertyType);

                types.push({ name: property, type: propertyType });
                if (recursiveTypes) {
                    output.set(propertyType, recursiveTypes);
                }
                for (const [key, value] of recursiveOutput.entries()) {
                    if (key !== primaryType) {
                        output.set(key, value);
                    }
                }
            }
        } else {
            throw new Error(`Type of field "${property}" not supported for EIP712`);
        }
    }

    output.set(primaryType, types);
    return output;
}

/**
 * getEip712Domain returns a EIP712 domain object for the given chain id
 * @param evmChainId a evmChainId which need to match 1 on Injective
 * @returns a EIP712 domain object for the given chain id
 */
export function getEip712Domain(evmChainId: number) {
    return {
        domain: {
            name: "Injective Web3",
            version: "1.0.0",
            chainId: `0x${evmChainId.toString(16)}`,
            salt: "0",
            verifyingContract: "cosmos",
        },
    };
}

/**
 * getDefaultEip712Types returns the default EIP712 types registry
 * @returns registry of all EIP712 types
 */
export function getDefaultEip712Types() {
    return {
        types: {
            EIP712Domain: [
                { name: "name", type: "string" },
                { name: "version", type: "string" },
                { name: "chainId", type: "uint256" },
                { name: "verifyingContract", type: "string" },
                { name: "salt", type: "string" },
            ],
            Tx: [
                { name: "account_number", type: "string" },
                { name: "chain_id", type: "string" },
                { name: "fee", type: "Fee" },
                { name: "memo", type: "string" },
                { name: "msgs", type: "Msg[]" },
                { name: "sequence", type: "string" },
                { name: "timeout_height", type: "string" },
            ],
            Fee: [
                { name: "amount", type: "Coin[]" },
                { name: "gas", type: "string" },
            ],
            Coin: [
                { name: "denom", type: "string" },
                { name: "amount", type: "string" },
            ],
            Msg: [
                { name: "type", type: "string" },
                { name: "value", type: "MsgValue" },
            ],
        },
    };
}

export function getEip712Fee(fee: StdFee) {
    return {
        fee: {
            amount: fee.amount.map((c) => ({ denom: c.denom, amount: c.amount })),
            gas: fee.gas,
        },
    };
}

export function getEipTxDetails({
    accountNumber,
    sequence,
    timeoutHeight,
    chainId,
    memo,
}: {
    accountNumber: number;
    sequence: number;
    timeoutHeight: number;
    chainId: string;
    memo?: string;
}) {
    return {
        account_number: accountNumber.toString(),
        chain_id: chainId,
        timeout_height: timeoutHeight.toString(),
        memo: memo || "",
        sequence: sequence.toString(),
    };
}

/**
 * getEip712TypedData builds the EIP-712 typed data for a tx made up of
 * amino-converted messages.
 *
 * @param aminoMsgs - array of amino-converted messages
 * @param accountNumber - account number of the signer
 * @param sequence - sequence number of the signer
 * @param timeoutHeight - timeout height of the tx
 * @param chainId - chain ID of the tx
 * @param memo - memo of the tx
 * @param fee - fee of the tx
 * @param evmChainId - EVM chain ID
 * @returns EIP-712 typed data
 */
export function getEip712TypedData({
    aminoMsgs,
    accountNumber,
    sequence,
    timeoutHeight,
    chainId,
    memo,
    fee,
    evmChainId,
}: {
    aminoMsgs: { type: string; value: Record<string, unknown> }[];
    accountNumber: number;
    sequence: number;
    timeoutHeight: number;
    chainId: string;
    memo?: string;
    fee: StdFee;
    evmChainId: number;
}) {
    const eip712MessageTypes = aminoMsgs.reduce((acc, msg) => {
        const msgTypes = objectKeysToEip712Types({ object: msg.value, messageType: msg.type });
        for (const [key, value] of msgTypes.entries()) {
            acc.set(key, value);
        }
        return acc;
    }, new Map<string, TypedDataField[]>());

    const defaultTypes = getDefaultEip712Types();
    const types = {
        types: {
            ...defaultTypes.types,
            ...Object.fromEntries(eip712MessageTypes),
        },
    };

    return {
        ...types,
        primaryType: "Tx",
        ...getEip712Domain(evmChainId),
        message: {
            ...getEipTxDetails({ accountNumber, sequence, timeoutHeight, chainId, memo }),
            ...getEip712Fee(fee),
            msgs: aminoMsgs.map((msg) => ({ type: msg.type, value: msg.value })),
        },
    };
}
