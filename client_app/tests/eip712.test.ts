import { expect, test } from "vitest";
import { objectKeysToEip712Types } from "@/lib/injective/eip712";

test("objectKeysToEip712Types skips undefined-valued fields instead of throwing", () => {
    const aminoMsgTransferValue = {
        source_port: "transfer",
        source_channel: "channel-0",
        token: { denom: "uosmo", amount: "1000000" },
        sender: "osmo1sender",
        receiver: "cosmos1receiver",
        timeout_height: {},
        timeout_timestamp: undefined,
        memo: undefined,
    };

    const types = objectKeysToEip712Types({
        object: aminoMsgTransferValue,
        messageType: "cosmos-sdk/MsgTransfer",
    });

    const msgValueFields = types.get("MsgValue")?.map((f) => f.name);
    expect(msgValueFields).toContain("source_port");
    expect(msgValueFields).toContain("sender");
    expect(msgValueFields).not.toContain("memo");
    expect(msgValueFields).not.toContain("timeout_timestamp");
});

test("objectKeysToEip712Types includes memo as a string field when set", () => {
    const aminoMsgTransferValue = {
        source_port: "transfer",
        source_channel: "channel-0",
        token: { denom: "uosmo", amount: "1000000" },
        sender: "osmo1sender",
        receiver: "cosmos1receiver",
        timeout_height: {},
        timeout_timestamp: undefined,
        memo: "hello",
    };

    const types = objectKeysToEip712Types({
        object: aminoMsgTransferValue,
        messageType: "cosmos-sdk/MsgTransfer",
    });

    expect(types.get("MsgValue")).toContainEqual({ name: "memo", type: "string" });
});
