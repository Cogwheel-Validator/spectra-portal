import type { ClientChain, ClientToken } from "@/components/modules/tomlTypes";
import AssetDropdown from "@/components/ui/send/assetDropdown";
import ChainDropdown from "@/components/ui/send/chainDropdown";

interface ToSectionProps {
    availableReceiveChains: ClientChain[];
    availableReceiveTokens: ClientToken[];
    receiveChain: string;
    receiveToken: string;
    sendChain: string;
    isPending: boolean;
    onReceiveChainChange: (chainId: string) => void;
    onReceiveTokenChange: (tokenSymbol: string) => void;
}

export default function ToSection({
    availableReceiveChains,
    availableReceiveTokens,
    receiveChain,
    receiveToken,
    sendChain,
    isPending,
    onReceiveChainChange,
    onReceiveTokenChange,
}: ToSectionProps) {
    return (
        <div className="flex-1 bg-slate-800/30 rounded-xl p-4 lg:p-5 border border-slate-700/50 space-y-3">
            <h2 className="text-base lg:text-lg font-semibold text-white flex items-center gap-2">
                <span className="w-5 h-5 lg:w-6 lg:h-6 rounded-full bg-teal-500 flex items-center justify-center text-xs lg:text-sm font-bold">
                    2
                </span>
                To
            </h2>

            <div className="space-y-3">
                <ChainDropdown
                    chains={availableReceiveChains}
                    selectedChainId={receiveChain}
                    onSelect={onReceiveChainChange}
                    placeholder="Select destination chain"
                    disabled={isPending || !sendChain}
                    label="Chain"
                    variant="to"
                />

                <AssetDropdown
                    tokens={availableReceiveTokens}
                    selectedSymbol={receiveToken}
                    onSelect={onReceiveTokenChange}
                    placeholder="Select asset to receive"
                    disabled={isPending || !receiveChain}
                    label="Asset (optional)"
                />
            </div>
        </div>
    );
}

