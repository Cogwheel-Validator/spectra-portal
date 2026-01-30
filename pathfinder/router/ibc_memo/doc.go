/*
Package ibcmemo provides types and functions for building IBC memos for swap operations and for forwarding.

The current focus will be on the Osmosis chain and using the Skip Go ibc-hooks entry point contract.
Now there are some other chains that support IBC hooks, but we will focus on the Osmosis chain for now.

The key usage of the ibc-hooks is the idea that you can truncate the amount of transactions needed to execute
the intended action. This is very useful for the user experience.

There are a couple of different combinations of memos and how can should they look like.

1. Simple Forwarding ( or Multi-Hop Routing )

This process usually involves sending one token over at least 1 network and then forwarding it to the
destination chain. The key thing here is that the destination chain can reach the middle chain and that
middle chain can reach the source chain.

So some kind of formula would be something like this:

Source -> Middle -> Destination

The pathfinder will max out on 4 chains. So do not attempt for anything more. The memo for this type of
transfer looks something like this:

This example is for a transfer from Neutron, sending USDC to Osmosis:

	{
	  "forward": {
	    "channel": "channel-1",
	    "port": "transfer",
	    "receiver": "osmo1bech32address",
	    "retries": 2,
	    "timeout": 1769791113913992700
	  }
	}

The IBC data you insert should still be pointed to the Noble since this is the middle chain, we are just sending
info to redirect this transfer to the Osmosis chain.

2. Transfer and Swap

This is a simple IBC transfer combined on chain swap. The specific thing about this is that there is a need for
the IBC hook to be executed. This method is most common when you want to receive an asset on the Broker Chain.

This is how the JSON looks like for a transfer of ATOM token from the Cosmos Hub to the Osmosis Chain to
receive INJ token:

	{
	   "wasm":{
	      "contract":"osmo10a3k4hvk37cc4hnxctw4p95fhscd2z6h2rmx0aukc6rm8u9qqx9smfsh7u",
	      "msg":{
	         "swap_and_action":{
	            "user_swap":{
	               "swap_exact_asset_in":{
	                  "swap_venue_name":"osmosis-poolmanager",
	                  "operations":[
	                     {
	                        "pool":"1282",
	                        "denom_in":"ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2",
	                        "denom_out":"ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4"
	                     },
	                     {
	                        "pool":"1319",
	                        "denom_in":"ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4",
	                        "denom_out":"ibc/64BA6E31FE887D66C6F8F31C7B1A80C7CA179239677B4088BB55F5EA07DBE273"
	                     }
	                  ]
	               }
	            },
	            "min_asset":{
	               "native":{
	                  "denom":"ibc/64BA6E31FE887D66C6F8F31C7B1A80C7CA179239677B4088BB55F5EA07DBE273",
	                  "amount":"51724833532052439"
	               }
	            },
	            "timeout_timestamp":1769790211797082680,
	            "post_swap_action":{
	               "transfer":{
	                  "to_address":"osmo1bech32address"
	               }
	            },
	            "affiliates":[]
	         }
	      }
	   }
	}

The memo ALWAYS uses Skip Go ibc-hooks entry point contract. What pathfinder here collects and generates is
the Osmosis SQS route data, applies slippage protection and then generates the memo.
The msg.swap_and_action.post_swap_action.transfer.to_address is the address the user initiating the transfer
wants to receive the tokens on the Osmosis chain. In the IBC data the receiver address is the smart contract address!

3. Swap and Transfer

This is very similar to the example above, however usually the intended action starts from the Broker Chain,
Osmosis for example, and then you can send the assets traded in one transaction to the destination chain.
However this kind of transfer is usually only executable via execute smart contract command:

	{
	  "swap_and_action": {
	    "user_swap": {
	      "swap_exact_asset_in": {
	        "swap_venue_name": "osmosis-poolmanager",
	        "operations": [
	          {
	            "pool": "1567",
	            "denom_in": "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4",
	            "denom_out": "ibc/64BA6E31FE887D66C6F8F31C7B1A80C7CA179239677B4088BB55F5EA07DBE273"
	          }
	        ]
	      }
	    },
	    "min_asset": {
	      "native": {
	        "denom": "ibc/64BA6E31FE887D66C6F8F31C7B1A80C7CA179239677B4088BB55F5EA07DBE273",
	        "amount": "246852563031812464"
	      }
	    },
	    "timeout_timestamp": 1769791992029217000,
	    "post_swap_action": {
	      "ibc_transfer": {
	        "ibc_info": {
	          "source_channel": "channel-122",
	          "receiver": "inj1bech32address",
	          "memo": "",
	          "recover_address": "osmo1bech32address"
	        }
	      }
	    },
	    "affiliates": []
	  }
	}

This doesn't have anything with IBC memo, however this here is just to show you that, in this case,
IBC memo simply is not needed. You either use smart contract, or initiate 2 manual transactions.

4. Swap and Multi Hop

So this is similar to the option above, however you can extand the Osmosis smart contract to
send the assets over the IBC:

	{
	  "swap_and_action": {
	    "user_swap": {
	      "swap_exact_asset_in": {
	        "swap_venue_name": "osmosis-poolmanager",
	        "operations": [
	          {
	            "pool": "1464",
	            "denom_in": "uosmo",
	            "denom_out": "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4"
	          }
	        ]
	      }
	    },
	    "min_asset": {
	      "native": {
	        "denom": "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4",
	        "amount": "4584"
	      }
	    },
	    "timeout_timestamp": 1769794463350751700,
	    "post_swap_action": {
	      "ibc_transfer": {
	        "ibc_info": {
	          "source_channel": "channel-750",
	          "receiver": "noble1bech32address",
	          "memo": "{\"forward\":{\"channel\":\"channel-3\",\"port\":\"transfer\",\"receiver\":\"juno1bech32address\",\"retries\":2,\"timeout\":1769794463350749612}}",
	          "recover_address": "osmo1bech32address"
	        }
	      }
	    },
	    "affiliates": []
	  }
	}

In this case we embed the Simple Forwarding method within the post swap actions and place it there. So in this case
we initiate the smart contract on the Osmosis chain, and we instruct Noble chain when it receives the assets
send to Juno.

5. Transfer with multi-hop and swap

So there is a possibility that we might need to make a trasnfer first to the Broker Chain and from there send
the assets to the destination chain. There are some variations of how can this look like but to timplify it
treat the Broker chain as an anchor.

Inbound IBC Leg [ Max 2 Chains ] -> Broker Chain (Swap) -> Outbound IBC Leg [ Max 2 Chains ]

Each "leg" presents a chain transfer. Maximum of total 4 legs ( plus the broker chain )
is allowed at all time. There are 3 variations but they are all similar.

5.1. One Leg On Each Side of the Broker Chain

This is the most common variation. We have one leg on each side of the Broker Chain.

	{
	  "wasm": {
	    "contract": "osmo10a3k4hvk37cc4hnxctw4p95fhscd2z6h2rmx0aukc6rm8u9qqx9smfsh7u",
	    "msg": {
	      "swap_and_action": {
	        "user_swap": {
	          "swap_exact_asset_in": {
	            "swap_venue_name": "osmosis-poolmanager",
	            "operations": [
	              {
	                "pool": "1",
	                "denom_in": "ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2",
	                "denom_out": "uosmo"
	              },
	              {
	                "pool": "1464",
	                "denom_in": "uluna",
	                "denom_out": "ibc/2C962DAB9F57FE0921435426AE75196009FAA1981BF86991203C8411F8980FDB"
	              }
	            ]
	          }
	        },
	        "min_asset": {
	          "native": {
	            "denom": "ibc/2C962DAB9F57FE0921435426AE75196009FAA1981BF86991203C8411F8980FDB",
	            "amount": "210398"
	          }
	        },
	        "timeout_timestamp": 1769800008167480300,
	        "post_swap_action": {
	          "ibc_transfer": {
	            "ibc_info": {
	              "source_channel": "channel-253",
	              "receiver": "noble1bech32address",
	              "memo": "",
	              "recover_address": "osmo1bech32address"
	            }
	          }
	        },
	        "affiliates": []
	      }
	    }
	  }
	}

The method is identical to Transfer and Swap, however here in the post_swap_action instead of regular transfer,
this memo uses ibc_transfer. In this section receiver and recover address are the ones meant for the user
addresses. The receiver in this IBC transfer is actually the smart contract address.

5.2. Two Inbound Legs and One Outbound Leg

This is an example transfer from Neutron to Injective Chain.

	{
	   "forward":{
	      "channel":"channel-141",
	      "next":{
	         "wasm":{
	            "contract":"osmo10a3k4hvk37cc4hnxctw4p95fhscd2z6h2rmx0aukc6rm8u9qqx9smfsh7u",
	            "msg":{
	               "swap_and_action":{
	                  "affiliates":[],
	                  "min_asset":{
	                     "native":{
	                        "amount":"1549881842593500445",
	                        "denom":"ibc/64BA6E31FE887D66C6F8F31C7B1A80C7CA179239677B4088BB55F5EA07DBE273"
	                     }
	                  },
	                  "post_swap_action":{
	                     "ibc_transfer":{
	                        "ibc_info":{
	                           "memo":"",
	                           "receiver":"inj1bech32address",
	                           "recover_address":"osmo1bech32address",
	                           "source_channel":"channel-122"
	                        }
	                     }
	                  },
	                  "timeout_timestamp":1769614031777034000,
	                  "user_swap":{
	                     "swap_exact_asset_in":{
	                        "operations":[
	                           {
	                              "denom_in":"ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2",
	                              "denom_out":"uosmo",
	                              "pool":"1"
	                           },
	                           {
	                              "denom_in":"uosmo",
	                              "denom_out":"ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4",
	                              "pool":"1464"
	                           },
	                           {
	                              "denom_in":"ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4",
	                              "denom_out":"ibc/64BA6E31FE887D66C6F8F31C7B1A80C7CA179239677B4088BB55F5EA07DBE273",
	                              "pool":"1319"
	                           }
	                        ],
	                        "swap_venue_name":"osmosis-poolmanager"
	                     }
	                  }
	               }
	            }
	         }
	      },
	      "port":"transfer",
	      "receiver":"osmo10a3k4hvk37cc4hnxctw4p95fhscd2z6h2rmx0aukc6rm8u9qqx9smfsh7u",
	      "retries":2,
	      "timeout":1769614031777016644
	   }
	}

So in this example everything is wrapped into forward memo. The receiver address in the IBC transaction is the
users address ( in this case this is address on the Cosmos Hub ). To the forward memo, we add channel, port,
receiver ( constant address of the smart contract on the Osmosis chain ) and retries and timeout. Other parts
of the memo should be similar to what was already explained in Swap and Transfer.

5.3 One Inbound Leg and Two Outbound Legs

	{
	  "wasm": {
	    "contract": "osmo10a3k4hvk37cc4hnxctw4p95fhscd2z6h2rmx0aukc6rm8u9qqx9smfsh7u",
	    "msg": {
	      "swap_and_action": {
	        "user_swap": {
	          "swap_exact_asset_in": {
	            "swap_venue_name": "osmosis-poolmanager",
	            "operations": [
	              {
	                "pool": "1135",
	                "denom_in": "ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2",
	                "denom_out": "uosmo"
	              },
	              {
	                "pool": "1263",
	                "denom_in": "uosmo",
	                "denom_out": "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4"
	              }
	            ]
	          }
	        },
	        "min_asset": {
	          "native": {
	            "denom": "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4",
	            "amount": "207718"
	          }
	        },
	        "timeout_timestamp": 1769804258113751800,
	        "post_swap_action": {
	          "ibc_transfer": {
	            "ibc_info": {
	              "source_channel": "channel-750",
	              "receiver": "noble1bech32address",
	              "memo": "{\"forward\":{\"channel\":\"channel-3\",\"port\":\"transfer\",\"receiver\":\"juno1bech32address\",\"retries\":2,\"timeout\":1769804258113749879}}",
	              "recover_address": "osmo1bech32address"
	            }
	          }
	        },
	        "affiliates": []
	      }
	    }
	  }
	}

So here the ibc_transfer.ibc_info.memo is wrapped into the forward memo. The receiver address in the
IBC transaction is the smart contract address on the Osmosis chain.

5.4 Two Inbound Legs and Two Outbound Legs

	{
	  "forward": {
	    "channel": "channel-1",
	    "next": {
	      "wasm": {
	        "contract": "osmo10a3k4hvk37cc4hnxctw4p95fhscd2z6h2rmx0aukc6rm8u9qqx9smfsh7u",
	        "msg": {
	          "swap_and_action": {
	            "affiliates": [],
	            "min_asset": {
	              "native": {
	                "amount": "214252",
	                "denom": "ibc/B3792E4A62DF4A934EF2DF5968556DB56F5776ED25BDE11188A4F58A7DD406F0"
	              }
	            },
	            "post_swap_action": {
	              "ibc_transfer": {
	                "ibc_info": {
	                  "memo": "{\"forward\":{\"channel\":\"channel-3\",\"port\":\"transfer\",\"receiver\":\"juno1bech32address\",\"retries\":2,\"timeout\":1769803380957035530}}",
	                  "receiver": "noble1bech32address",
	                  "recover_address": "osmo1bech32address",
	                  "source_channel": "channel-132"
	                }
	              }
	            },
	            "timeout_timestamp": 1769803380957040000,
	            "user_swap": {
	              "swap_exact_asset_in": {
	                "operations": [
	                  {
	                    "denom_in": "ibc/C8A74ABBE2AF892E15680D916A7C22130585CE5704F9B17A10F184A90D53BECA",
	                    "denom_out": "uosmo",
	                    "pool": "2"
	                  },
	                  {
	                    "denom_in": "uosmo",
	                    "denom_out": "ibc/C559977F5797BDC1D74C0836A10C379C991D664166CB60D776A83029852431B4",
	                    "pool": "5"
	                  },
	                  {
	                    "denom_in": "ibc/C559977F5797BDC1D74C0836A10C379C991D664166CB60D776A83029852431B4",
	                    "denom_out": "ibc/B3792E4A62DF4A934EF2DF5968556DB56F5776ED25BDE11188A4F58A7DD406F0",
	                    "pool": "4"
	                  }
	                ],
	                "swap_venue_name": "osmosis-poolmanager"
	              }
	            }
	          }
	        }
	      }
	    },
	    "port": "transfer",
	    "receiver": "osmo10a3k4hvk37cc4hnxctw4p95fhscd2z6h2rmx0aukc6rm8u9qqx9smfsh7u",
	    "retries": 2,
	    "timeout": 1769803380957019000
	  }
	}

So this is combination of all things seen here before and it is the most complex variation.
*/
package ibcmemo
