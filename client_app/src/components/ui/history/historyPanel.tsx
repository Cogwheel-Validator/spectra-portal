"use client";

import { ChevronDown, ChevronUp, History, Loader2 } from "lucide-react";
import { useCallback, useState } from "react";
import type { ClientConfig } from "@/components/modules/tomlTypes";
import { type TransactionWithMeta, useTransactionHistory } from "@/hooks/useTransactionHistory";
import HistoryItem from "./historyItem";

interface HistoryPanelProps {
    config: ClientConfig;
    onResume?: (transaction: TransactionWithMeta) => void;
    onRetry?: (transaction: TransactionWithMeta) => void;
}

export default function HistoryPanel({ config, onResume, onRetry }: HistoryPanelProps) {
    const [isExpanded, setIsExpanded] = useState(false);
    const { transactions, isLoading, error, resumableCount, refresh } = useTransactionHistory(20);

    const handleToggle = useCallback(() => {
        setIsExpanded((prev) => !prev);
    }, []);

    // Don't render if no transactions
    if (!isLoading && transactions.length === 0) {
        return null;
    }

    const hasActionableItems = transactions.some((tx) => tx.isResumable || tx.isRetryable);

    return (
        <div className="bg-slate-800/30 rounded-xl border border-slate-700/50 overflow-hidden">
            {/* Header / Toggle Button */}
            <button
                type="button"
                onClick={handleToggle}
                className="w-full flex items-center justify-between px-6 py-4 hover:bg-slate-700/30 transition-colors"
            >
                <div className="flex items-center gap-3">
                    <History className="w-5 h-5 text-slate-400" />
                    <span className="font-medium text-white">Recent Transfers</span>

                    {/* Badge for resumable/actionable items */}
                    {!isLoading && (
                        <>
                            <span className="text-sm text-slate-400">({transactions.length})</span>
                            {resumableCount > 0 && (
                                <span className="px-2 py-0.5 text-xs font-medium bg-amber-500/20 text-amber-400 rounded-full">
                                    {resumableCount} resumable
                                </span>
                            )}
                        </>
                    )}

                    {isLoading && <Loader2 className="w-4 h-4 text-slate-400 animate-spin" />}
                </div>

                <div className="flex items-center gap-2">
                    {hasActionableItems && !isExpanded && (
                        <span className="text-xs text-amber-400">Action needed</span>
                    )}
                    {isExpanded ? (
                        <ChevronUp className="w-5 h-5 text-slate-400" />
                    ) : (
                        <ChevronDown className="w-5 h-5 text-slate-400" />
                    )}
                </div>
            </button>

            {/* Expandable Content */}
            {isExpanded && (
                <div className="border-t border-slate-700/50">
                    {isLoading ? (
                        <div className="flex items-center justify-center py-8">
                            <Loader2 className="w-6 h-6 text-slate-400 animate-spin" />
                        </div>
                    ) : error ? (
                        <div className="px-6 py-4 text-center">
                            <p className="text-red-400 text-sm">{error}</p>
                            <button
                                type="button"
                                onClick={refresh}
                                className="mt-2 text-xs text-teal-400 hover:text-teal-300"
                            >
                                Try again
                            </button>
                        </div>
                    ) : transactions.length === 0 ? (
                        <div className="px-6 py-8 text-center">
                            <p className="text-slate-400 text-sm">No recent transfers</p>
                        </div>
                    ) : (
                        <div className="p-4 space-y-3 max-h-96 overflow-y-auto">
                            {transactions.map((tx) => (
                                <HistoryItem
                                    key={tx.id}
                                    transaction={tx}
                                    config={config}
                                    onResume={onResume}
                                    onRetry={onRetry}
                                />
                            ))}
                        </div>
                    )}

                    {/* Footer with refresh button */}
                    {!isLoading && transactions.length > 0 && (
                        <div className="px-6 py-3 border-t border-slate-700/50 flex justify-end">
                            <button
                                type="button"
                                onClick={refresh}
                                className="text-xs text-slate-400 hover:text-white transition-colors"
                            >
                                Refresh
                            </button>
                        </div>
                    )}
                </div>
            )}
        </div>
    );
}
