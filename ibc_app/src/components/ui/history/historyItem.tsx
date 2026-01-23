"use client";

import {
    AlertTriangle,
    ArrowRight,
    CheckCircle2,
    Clock,
    Play,
    RotateCcw,
    XCircle,
} from "lucide-react";
import Image from "next/image";
import type { ClientConfig } from "@/components/modules/tomlTypes";
import type { TransactionWithMeta } from "@/hooks/useTransactionHistory";

interface HistoryItemProps {
    transaction: TransactionWithMeta;
    config: ClientConfig;
    onResume?: (transaction: TransactionWithMeta) => void;
    onRetry?: (transaction: TransactionWithMeta) => void;
}

export default function HistoryItem({ transaction, config, onResume, onRetry }: HistoryItemProps) {
    const fromChain = config.chains.find((c) => c.id === transaction.fromChainId);
    const toChain = config.chains.find((c) => c.id === transaction.toChainId);

    // Get token info
    const fromToken = fromChain
        ? [...fromChain.native_tokens, ...fromChain.ibc_tokens].find(
              (t) => t.denom === transaction.tokenIn,
          )
        : null;

    // Status icon and color
    const StatusIcon = () => {
        switch (transaction.status) {
            case "success":
                return <CheckCircle2 className="w-5 h-5 text-emerald-400" />;
            case "failed":
                return <XCircle className="w-5 h-5 text-red-400" />;
            case "in-progress":
                return <Clock className="w-5 h-5 text-amber-400 animate-pulse" />;
            case "canceled":
                return <AlertTriangle className="w-5 h-5 text-slate-400" />;
            default:
                return null;
        }
    };

    const statusColor = {
        success: "border-emerald-500/30 bg-emerald-500/5",
        failed: "border-red-500/30 bg-red-500/5",
        "in-progress": "border-amber-500/30 bg-amber-500/5",
        canceled: "border-slate-500/30 bg-slate-500/5",
    }[transaction.status];

    // Format amount for display
    const formatAmount = (amount: string, decimals: number = 6) => {
        const num = Number.parseFloat(amount) / 10 ** decimals;
        return num.toFixed(4);
    };

    return (
        <div
            className={`rounded-lg border p-4 ${statusColor} transition-all hover:bg-slate-800/30`}
        >
            {/* Main Row */}
            <div className="flex items-center gap-4">
                {/* Status Icon */}
                <StatusIcon />

                {/* Time */}
                <span className="text-xs text-slate-400 w-16 shrink-0">
                    {transaction.formattedAge}
                </span>

                {/* Chain Path */}
                <div className="flex items-center gap-2 flex-1 min-w-0">
                    {/* From Chain */}
                    <div className="flex items-center gap-1.5 shrink-0">
                        <Image
                            src={fromChain?.chain_logo || "/unknown.jpg"}
                            alt={fromChain?.name || "Unknown"}
                            width={24}
                            height={24}
                            className="rounded-full"
                        />
                        <span className="text-sm text-white font-medium hidden sm:inline">
                            {fromChain?.name || "Unknown"}
                        </span>
                    </div>

                    {/* Trajectory */}
                    {transaction.trajectory && transaction.trajectory.length > 0 && (
                        <>
                            <ArrowRight className="w-4 h-4 text-slate-500 shrink-0" />
                            <div className="flex items-center gap-1">
                                {transaction.trajectory.map((chainId) => {
                                    const chain = config.chains.find((c) => c.id === chainId);
                                    return (
                                        <Image
                                            key={chainId}
                                            src={chain?.chain_logo || "/unknown.jpg"}
                                            alt={chain?.name || chainId}
                                            width={20}
                                            height={20}
                                            className="rounded-full opacity-60"
                                            title={chain?.name || chainId}
                                        />
                                    );
                                })}
                            </div>
                        </>
                    )}

                    <ArrowRight className="w-4 h-4 text-slate-500 shrink-0" />

                    {/* To Chain */}
                    <div className="flex items-center gap-1.5 shrink-0">
                        <Image
                            src={toChain?.chain_logo || "/unknown.jpg"}
                            alt={toChain?.name || "Unknown"}
                            width={24}
                            height={24}
                            className="rounded-full"
                        />
                        <span className="text-sm text-white font-medium hidden sm:inline">
                            {toChain?.name || "Unknown"}
                        </span>
                    </div>
                </div>

                {/* Amount */}
                <div className="text-right shrink-0">
                    <span className="text-sm text-white font-medium">
                        {formatAmount(transaction.amountIn, fromToken?.decimals)}
                    </span>
                    <span className="text-xs text-slate-400 ml-1">
                        {fromToken?.symbol || "???"}
                    </span>
                </div>

                {/* Progress (for in-progress) */}
                {transaction.status === "in-progress" && (
                    <div className="text-xs text-slate-400 shrink-0">
                        Step {transaction.currentStep}/{transaction.totalSteps}
                    </div>
                )}

                {/* Actions */}
                <div className="flex items-center gap-2 shrink-0">
                    {transaction.isResumable && onResume && (
                        <button
                            type="button"
                            onClick={() => onResume(transaction)}
                            className="flex items-center gap-1 px-2 py-1 text-xs font-medium text-amber-400 bg-amber-500/10 rounded hover:bg-amber-500/20 transition-colors"
                        >
                            <Play className="w-3 h-3" />
                            Resume
                        </button>
                    )}

                    {transaction.isRetryable && onRetry && (
                        <button
                            type="button"
                            onClick={() => onRetry(transaction)}
                            className="flex items-center gap-1 px-2 py-1 text-xs font-medium text-red-400 bg-red-500/10 rounded hover:bg-red-500/20 transition-colors"
                        >
                            <RotateCcw className="w-3 h-3" />
                            Retry
                        </button>
                    )}
                </div>
            </div>

            {/* Error Message (for failed transactions) */}
            {transaction.status === "failed" && transaction.error && (
                <div className="mt-3 pt-3 border-t border-slate-700/50">
                    <p className="text-xs text-red-400 flex items-start gap-2">
                        <AlertTriangle className="w-3 h-3 mt-0.5 shrink-0" />
                        <span className="line-clamp-2">{transaction.error}</span>
                    </p>
                </div>
            )}

            {/* Transfer Type Badge */}
            <div className="mt-2 flex items-center gap-2">
                <span
                    className={`text-xs px-1.5 py-0.5 rounded ${
                        transaction.typeOfTransfer === "smart"
                            ? "bg-teal-500/20 text-teal-400"
                            : "bg-orange-500/20 text-orange-400"
                    }`}
                >
                    {transaction.typeOfTransfer === "smart" ? "Smart" : "Manual"}
                </span>
            </div>
        </div>
    );
}
