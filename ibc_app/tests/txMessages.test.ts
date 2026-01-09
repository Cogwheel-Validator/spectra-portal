import { expect, test } from "bun:test"
import { 
    getSwapInMessage, 
    getSwapInMessageDetails, 
    getTxMessageTypes, 
    type TransactionMessage, 
    TransactionMessageSchema, 
    type TransactionResponse, 
    TransactionResponseSchema,
} from "@/components/modules/cosmosApiData"

// In realaty the transaction will never have this collectevly, this is just for testing
const messages: TransactionMessage[] = TransactionMessageSchema.array().parse([{
    "@type": "/osmosis.poolmanager.v1beta1.MsgSwapExactAmountIn",
    "sender": "osmo1zaw1hlngspfqkp68psuv4suxwjfxftken7rwm0",
    "routes": [
      {
        "pool_id": "1464",
        "token_out_denom": "uosmo"
      },
      {
        "pool_id": "1094",
        "token_out_denom": "ibc/903A61A498756EA560B85A85132D3AEE21B5DEDD41213725D22ABF276EA6945E"
      }
    ],
    "token_in": {
      "denom": "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4",
      "amount": "264432000"
    },
    "token_out_min_amount": "3640894367"
},
{
    "@type": "/ibc.core.client.v1.MsgUpdateClient"
},
{
    "@type": "/ibc.core.channel.v1.MsgRecvPacket"
}])

const txResponse: TransactionResponse = TransactionResponseSchema.parse({
    tx_responses: [{
        height: "123",
        txhash: "txhash",
        codespace: "codespace",
        code: 2,
        raw_log: "raw_log",
        info: "info",
        gas_wanted: "1000000",
        gas_used: "1000000",
        timestamp: "2021-01-01T00:00:00Z",
        events: [],
        tx: {
            type_url: "/cosmos.tx.v1beta1.Tx",
            body: {
                messages: messages
            }
        }
    }],
    pagination: {
        next_key: "next_key",
        total: "1"
    },
    total: "1"
})


test("getSwapInMessage", () => {
    const swapInMessage = getSwapInMessage(messages);
    expect(swapInMessage).toBeDefined();
    if (!swapInMessage) {
        throw new Error("Swap in message not found");
    }
    expect(swapInMessage["@type"]).toBe("/osmosis.poolmanager.v1beta1.MsgSwapExactAmountIn");
    expect(swapInMessage.sender).toBe("osmo1zaw1hlngspfqkp68psuv4suxwjfxftken7rwm0");
    expect(swapInMessage.routes).toBeDefined();
    expect(swapInMessage.token_in).toBeDefined();
    expect(swapInMessage.token_out_min_amount).toBeDefined();
});

test("get all message types in messages", () => {
    const allTypes = getTxMessageTypes(txResponse, "txhash");
    expect(allTypes).toBeDefined();
    expect(allTypes.length).toBe(3);
    expect(allTypes).toContain("/osmosis.poolmanager.v1beta1.MsgSwapExactAmountIn");
    expect(allTypes).toContain("/ibc.core.client.v1.MsgUpdateClient");
    expect(allTypes).toContain("/ibc.core.channel.v1.MsgRecvPacket");
});

test("getSwapInMessageDetails", () => {
    const swapInMessageDetails = getSwapInMessageDetails(messages);
    expect(swapInMessageDetails).toBeDefined();
    if (!swapInMessageDetails) {
        throw new Error("Swap in message details not found");
    }
    expect(swapInMessageDetails.sender).toBe("osmo1zaw1hlngspfqkp68psuv4suxwjfxftken7rwm0");
    expect(swapInMessageDetails.routes.length).toBe(2);
    expect(swapInMessageDetails.routes[0].poolId).toBe(1464);
    expect(swapInMessageDetails.routes[0].tokenOutDenom).toBe("uosmo");
    expect(swapInMessageDetails.routes[1].poolId).toBe(1094);
    expect(swapInMessageDetails.routes[1].tokenOutDenom).toBe("ibc/903A61A498756EA560B85A85132D3AEE21B5DEDD41213725D22ABF276EA6945E");
    expect(swapInMessageDetails.tokenIn).toBeDefined();
    expect(swapInMessageDetails.tokenIn.denom).toBe("ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4");
    expect(swapInMessageDetails.tokenIn.amount).toBe("264432000");
    expect(swapInMessageDetails.tokenOutMinAmount).toBe("3640894367");
});