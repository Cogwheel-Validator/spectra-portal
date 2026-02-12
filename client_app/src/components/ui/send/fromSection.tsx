import { AlertCircle } from "lucide-react";
import type { ClientChain, ClientToken } from "@/components/modules/tomlTypes";
import AmountInput from "@/components/ui/send/amountInput";
import AssetDropdown from "@/components/ui/send/assetDropdown";
import ChainDropdown from "@/components/ui/send/chainDropdown";

interface FromSectionProps {
    chains: ClientChain[];
    availableSendTokens: ClientToken[];
    selectedSendToken: ClientToken | null;
    sendChain: string;
    sendToken: string;
    amount: string;
    senderAddress: string;
    receiveChain: string;
    receiveToken: string;
    formattedBalance: string | null;
    insufficientBalance: boolean;
    balanceLoading: boolean;
    isPending: boolean;
    onSendChainChange: (chainId: string) => void;
    onSendTokenChange: (tokenSymbol: string) => void;
    onAmountChange: (value: string) => void;
    senderBalance:
        | {
              balances: {
                  denom: string;
                  amount: string;
              }[];
              pagination: {
                  total: string;
                  next_key?: null | undefined;
              };
          }
        | undefined;
}

export default function FromSection({
    chains,
    availableSendTokens,
    selectedSendToken,
    sendChain,
    sendToken,
    amount,
    senderAddress,
    receiveChain,
    receiveToken,
    formattedBalance,
    insufficientBalance,
    balanceLoading,
    isPending,
    onSendChainChange,
    onSendTokenChange,
    onAmountChange,
    senderBalance,
}: FromSectionProps) {
    return (
        <div className="flex-1 bg-slate-800/30 rounded-xl p-4 lg:p-5 border border-slate-700/50 space-y-3">
            <h2 className="text-base lg:text-lg font-semibold text-white flex items-center gap-2">
                <span className="w-5 h-5 lg:w-6 lg:h-6 rounded-full bg-orange-500 flex items-center justify-center text-xs lg:text-sm font-bold">
                    1
                </span>
                From
            </h2>

            <div className="space-y-3">
                <ChainDropdown
                    chains={chains}
                    selectedChainId={sendChain}
                    onSelect={onSendChainChange}
                    placeholder="Select source chain"
                    disabled={isPending}
                    label="Chain"
                    variant="from"
                />

                <AssetDropdown
                    tokens={availableSendTokens}
                    selectedSymbol={sendToken}
                    onSelect={onSendTokenChange}
                    placeholder="Select asset to send"
                    disabled={isPending || !sendChain}
                    label="Asset"
                    senderBalance={senderBalance}
                />

                <div className="space-y-2">
                    <AmountInput
                        value={amount}
                        onChange={onAmountChange}
                        token={selectedSendToken}
                        disabled={isPending || !receiveChain || !receiveToken}
                        label="Amount to Send"
                    />

                    {selectedSendToken && senderAddress && (
                        <div className="flex justify-between text-xs">
                            <span className="text-slate-400">Available:</span>
                            {balanceLoading ? (
                                <span className="text-slate-400">Loading...</span>
                            ) : formattedBalance !== null ? (
                                <span
                                    className={`font-medium ${insufficientBalance ? "text-red-400" : "text-slate-300"}`}
                                >
                                    {formattedBalance} {selectedSendToken.symbol}
                                </span>
                            ) : (
                                <span className="text-slate-400">0 {selectedSendToken.symbol}</span>
                            )}
                        </div>
                    )}

                    {insufficientBalance && (
                        <div className="flex items-center gap-2 text-red-400 text-xs">
                            <AlertCircle className="w-3 h-3" />
                            <span>Insufficient balance</span>
                        </div>
                    )}
                </div>
            </div>
        </div>
    );
}
