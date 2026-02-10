import { ArrowRight, Loader2 } from "lucide-react";
import type { ClientChain } from "@/components/modules/tomlTypes";
import type { TransferMode } from "@/context/transferContext";

interface TransferButtonProps {
    canSubmit: boolean;
    isPending: boolean;
    isRefreshing: boolean;
    isWalletReady: { ready: boolean; missingChains: ClientChain[]; multiHop: boolean };
    pathfinderSuccess: boolean;
    routeLoading: boolean;
    routePending: boolean;
    routeIsStale: boolean;
    mode: TransferMode;
    onSubmit: () => void;
}

export default function TransferButton({
    canSubmit,
    isPending,
    isRefreshing,
    isWalletReady,
    pathfinderSuccess,
    routeLoading,
    routePending,
    routeIsStale,
    mode,
    onSubmit,
}: TransferButtonProps) {
    const getButtonText = () => {
        if (!isWalletReady.ready) {
            const nMissing = isWalletReady.missingChains.length;
            if (nMissing === 1) {
                return `Connect Wallet to ${isWalletReady.missingChains[0].id}`;
            } else if (nMissing === 2 && !isWalletReady.multiHop) {
                return `Connect Wallet to Both Chains`;
            } else if (nMissing >= 1 && isWalletReady.multiHop) {
                // at this point we have more than 2 missing chains and it is a multi-hop route
                return `Connect Wallet to additional chains`;
            }
        }
        if (!pathfinderSuccess) {
            return "Enter Transfer Details";
        }
        if (routeLoading || routePending) {
            return (
                <>
                    <Loader2 className="w-5 h-5 animate-spin" />
                    Computing...
                </>
            );
        }
        if (isRefreshing) {
            return (
                <>
                    <Loader2 className="w-5 h-5 animate-spin" />
                    Refreshing quote...
                </>
            );
        }
        return (
            <>
                {mode === "smart" ? "Smart Transfer" : "Manual Transfer"}
                {routeIsStale && <span className="text-xs opacity-75">(will refresh)</span>}
                <ArrowRight className="w-5 h-5" />
            </>
        );
    };

    return (
        <button
            type="button"
            onClick={onSubmit}
            disabled={!canSubmit || isPending || isRefreshing}
            className={`
                w-full py-3 lg:py-4 px-6 rounded-xl font-bold text-base lg:text-lg
                transition-all duration-300 flex items-center justify-center gap-3
                ${
                    canSubmit && !isRefreshing
                        ? "bg-linear-to-r from-teal-500 to-emerald-500 text-white hover:from-teal-400 hover:to-emerald-400 shadow-lg shadow-teal-500/25"
                        : "bg-slate-700 text-slate-400 cursor-not-allowed"
                }
            `}
        >
            {getButtonText()}
        </button>
    );
}
