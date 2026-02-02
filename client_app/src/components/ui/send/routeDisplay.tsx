import { AlertCircle, CheckCircle2, Loader2 } from "lucide-react";
import type { ClientToken } from "@/components/modules/tomlTypes";
import type { RouteInfo } from "@/hooks/useRouteInfo";

interface RouteDisplayProps {
    routeLoading: boolean;
    routePending: boolean;
    routeError: string | null;
    routeInfo: RouteInfo | null;
    routeIsStale: boolean;
    quoteAgeSeconds: number | null;
    selectedReceiveToken: ClientToken | null;
    amount: string;
}

export default function RouteDisplay({
    routeLoading,
    routePending,
    routeError,
    routeInfo,
    routeIsStale,
    quoteAgeSeconds,
    selectedReceiveToken,
    amount,
}: RouteDisplayProps) {
    if ((routeLoading || routePending) && amount && Number.parseFloat(amount) > 0) {
        return (
            <div className="bg-slate-800/30 rounded-xl p-4 border border-slate-700/50">
                <div className="flex items-center gap-3">
                    <Loader2 className="w-4 h-4 text-teal-400 animate-spin" />
                    <span className="text-slate-300 text-sm">Calculating best route...</span>
                </div>
            </div>
        );
    }

    if (routeError) {
        return (
            <div className="bg-red-500/10 rounded-xl p-4 border border-red-500/30">
                <div className="flex items-center gap-3">
                    <AlertCircle className="w-4 h-4 text-red-400" />
                    <span className="text-red-300 text-sm">{routeError}</span>
                </div>
            </div>
        );
    }

    if (!routeInfo || routeLoading || routePending) {
        return null;
    }

    return (
        <div
            className={`rounded-xl p-4 border ${
                routeIsStale
                    ? "bg-amber-500/10 border-amber-500/30"
                    : "bg-linear-to-r from-teal-500/10 to-emerald-500/10 border-teal-500/30"
            }`}
        >
            <div className="flex items-center justify-between mb-3">
                <h3 className="text-base font-semibold text-white">Route Summary</h3>
                <div className="flex items-center gap-2">
                    {quoteAgeSeconds !== null && (
                        <span
                            className={`text-xs ${routeIsStale ? "text-amber-400" : "text-slate-400"}`}
                        >
                            {quoteAgeSeconds}s ago
                        </span>
                    )}
                    {routeIsStale ? (
                        <AlertCircle className="w-4 h-4 text-amber-400" />
                    ) : (
                        <CheckCircle2 className="w-4 h-4 text-teal-400" />
                    )}
                </div>
            </div>

            {routeIsStale && (
                <div className="mb-3 p-2 bg-amber-500/10 rounded-lg border border-amber-500/20">
                    <p className="text-amber-300 text-xs">
                        Quote may be outdated. Price will be refreshed before execution.
                    </p>
                </div>
            )}

            <div className="grid grid-cols-2 lg:grid-cols-4 gap-3 text-sm">
                <div>
                    <span className="text-slate-400 text-xs">Route Type</span>
                    <p className="text-white font-medium">{routeInfo.routeType}</p>
                </div>
                <div>
                    <span className="text-slate-400 text-xs">Steps</span>
                    <p className="text-white font-medium">
                        {routeInfo.stepCount} tx{routeInfo.stepCount !== 1 ? "s" : ""}
                    </p>
                </div>
                {routeInfo.priceImpactBps !== 0 && Math.abs(routeInfo.priceImpactBps) > 1 && (
                    <div>
                        <span className="text-slate-400 text-xs">Price Impact</span>
                        <p className="font-medium" style={{ color: routeInfo.priceImpactColor }}>
                            {(routeInfo.priceImpact * 100).toFixed(2)}%
                        </p>
                    </div>
                )}
                {selectedReceiveToken && (
                    <div>
                        <span className="text-slate-400 text-xs">Expected Output</span>
                        <p className="text-white font-medium">
                            ~
                            {(
                                Number.parseFloat(routeInfo.expectedOutput) /
                                10 ** selectedReceiveToken.decimals
                            ).toLocaleString()}{" "}
                            {selectedReceiveToken.symbol}
                        </p>
                    </div>
                )}
            </div>
        </div>
    );
}

