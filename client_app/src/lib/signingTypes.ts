import { createWasmAminoConverters, wasmTypes } from "@cosmjs/cosmwasm-stargate";
import { type GeneratedType, Registry } from "@cosmjs/proto-signing";
import { AminoTypes, createDefaultAminoConverters, defaultRegistryTypes } from "@cosmjs/stargate";
import {
    MsgSplitRouteSwapExactAmountIn,
    MsgSwapExactAmountIn,
} from "@/lib/generated/osmosis/osmosis/poolmanager/v1beta1/tx";
import { createOsmosisAminoConverters } from "@/lib/osmosis/amino";

/**
 * Builds the protobuf registry and Amino converters for every message type the app
 * signs. Each type needs both: protobuf for SIGN_MODE_DIRECT, Amino for Ledger
 * (SIGN_MODE_LEGACY_AMINO_JSON, and EIP-712 on Injective).
 * @returns the registry and Amino types to hand to the signing client
 */
export function createSigningTypes(): { registry: Registry; aminoTypes: AminoTypes } {
    const registry = new Registry([...defaultRegistryTypes, ...wasmTypes]);
    registry.register(
        "/osmosis.poolmanager.v1beta1.MsgSwapExactAmountIn",
        MsgSwapExactAmountIn as GeneratedType,
    );
    registry.register(
        "/osmosis.poolmanager.v1beta1.MsgSplitRouteSwapExactAmountIn",
        MsgSplitRouteSwapExactAmountIn as GeneratedType,
    );

    const aminoTypes = new AminoTypes({
        ...createDefaultAminoConverters(),
        ...createWasmAminoConverters(),
        ...createOsmosisAminoConverters(),
    });

    return { registry, aminoTypes };
}
