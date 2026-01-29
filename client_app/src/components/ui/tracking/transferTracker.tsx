"use client";

import {
    AlertTriangle,
    ArrowLeft,
    CheckCircle2,
    ExternalLink,
    Loader2,
    RotateCcw,
    Settings2,
    XCircle,
} from "lucide-react";
import { motion } from "motion/react";
import Link from "next/link";
import { useCallback, useEffect, useMemo, useState } from "react";
import type { ClientConfig } from "@/components/modules/tomlTypes";
import { useTaskProvider } from "@/context/taskProvider";
import { useTransfer } from "@/context/transferContext";
import { buildExplorerTxUrl, formatSlippageErrorMessage, parseSlippageError } from "@/lib/utils";
import ChainProgress from "./chainProgress";
import StepIndicator from "./stepIndicator";

interface TransferTrackerProps {
    config: ClientConfig;
    onBack: () => void;
}

export default function TransferTracker({ config, onBack }: TransferTrackerProps) {
    const transfer = useTransfer();
    const taskProvider = useTaskProvider();

    const { state } = transfer;
    const {
        phase,
        mode,
        steps,
        currentStepIndex,
        chainPath,
        error,
        pathfinderResponse,
        fromChainId,
        toChainId,
        amount,
        fromToken,
    } = state;

    // Calculate current chain index based on step progress
    const currentChainIndex = useMemo(() => {
        if (phase === "completed") return chainPath.length - 1;
        if (steps.length === 0) return 0;

        // Find which chain we're currently at based on step progress
        const completedSteps = steps.filter((s) => s.status === "completed").length;
        return Math.min(completedSteps, chainPath.length - 1);
    }, [phase, steps, chainPath]);

    // Get explorer URL for a chain
    const getExplorerUrl = useCallback(
        (chainId: string, txHash?: string) => {
            const chain = config.chains.find((c) => c.id === chainId);
            if (!chain) return null;

            if (txHash) {
                return buildExplorerTxUrl(
                    chain.explorer_details.base_url,
                    chain.explorer_details.transaction_path,
                    txHash,
                );
            }
            return chain.explorer_details.base_url;
        },
        [config],
    );

    // Start execution when in preparing phase
    useEffect(() => {
        if (phase === "preparing" && pathfinderResponse?.success) {
            taskProvider.executeTransfer(pathfinderResponse, mode, config);
        }
    }, [phase, pathfinderResponse, mode, config, taskProvider]);

    // Get phase status for visualization
    const visualStatus = useMemo(() => {
        switch (phase) {
            case "preparing":
            case "executing":
            case "tracking":
                return "executing";
            case "completed":
                return "completed";
            case "failed":
                return "failed";
            default:
                return "pending";
        }
    }, [phase]);

    // Handle retry
    const handleRetry = useCallback(() => {
        if (currentStepIndex < steps.length) {
            taskProvider.retryStep(currentStepIndex, config);
        }
    }, [currentStepIndex, steps.length, config, taskProvider]);

    // Check if current step is retryable
    const canRetry = useMemo(() => {
        if (phase !== "failed") return false;
        const currentStep = steps[currentStepIndex];
        return currentStep?.status === "failed";
    }, [phase, steps, currentStepIndex]);

    // Can go back only if completed or failed
    const canGoBack = phase === "completed" || phase === "failed";

    // Get from/to chain data
    const fromChain = config.chains.find((c) => c.id === fromChainId);
    const toChain = config.chains.find((c) => c.id === toChainId);

    // Handle adjusting slippage for retry
    const [suggestedSlippage, setSuggestedSlippage] = useState<number | null>(null);

    const handleAdjustSlippage = useCallback(
        (newSlippageBps: number) => {
            setSuggestedSlippage(newSlippageBps);
            transfer.setSlippage(newSlippageBps);
        },
        [transfer],
    );

    return (
        <div className="max-w-3xl mx-auto py-12 px-4 space-y-8">
            {/* Header */}
            <motion.div
                initial={{ opacity: 0, y: -20 }}
                animate={{ opacity: 1, y: 0 }}
                className="text-center"
            >
                <h1 className="text-3xl font-bold text-white">
                    {phase === "completed"
                        ? "Transfer Complete!"
                        : phase === "failed"
                          ? "Transfer Failed"
                          : "Transfer in Progress"}
                </h1>
                <p className="text-slate-400 mt-2">
                    {mode === "smart" ? "Smart Transfer" : "Manual Transfer"} • {steps.length} step
                    {steps.length !== 1 ? "s" : ""}
                </p>
            </motion.div>

            {/* Chain Progress Visualization */}
            <motion.div
                initial={{ opacity: 0, scale: 0.95 }}
                animate={{ opacity: 1, scale: 1 }}
                transition={{ delay: 0.1 }}
                className="bg-slate-800/30 rounded-xl p-8 border border-slate-700/50"
            >
                <ChainProgress
                    chainPath={chainPath}
                    currentChainIndex={currentChainIndex}
                    config={config}
                    status={visualStatus}
                />
            </motion.div>

            {/* Transfer Summary */}
            <motion.div
                initial={{ opacity: 0, y: 20 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: 0.2 }}
                className="bg-slate-800/30 rounded-xl p-6 border border-slate-700/50"
            >
                <h3 className="text-lg font-semibold text-white mb-4">Transfer Summary</h3>
                <div className="grid grid-cols-2 gap-4">
                    <div>
                        <span className="text-sm text-slate-400">From</span>
                        <p className="text-white font-medium">{fromChain?.name || fromChainId}</p>
                    </div>
                    <div>
                        <span className="text-sm text-slate-400">To</span>
                        <p className="text-white font-medium">{toChain?.name || toChainId}</p>
                    </div>
                    <div>
                        <span className="text-sm text-slate-400">Amount</span>
                        <p className="text-white font-medium">
                            {amount} {fromToken?.symbol || ""}
                        </p>
                    </div>
                    <div>
                        <span className="text-sm text-slate-400">Progress</span>
                        <p className="text-white font-medium">
                            {steps.filter((s) => s.status === "completed").length} / {steps.length}{" "}
                            steps
                        </p>
                    </div>
                </div>
            </motion.div>

            {/* Step Details */}
            <motion.div
                initial={{ opacity: 0, y: 20 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: 0.3 }}
                className="bg-slate-800/30 rounded-xl p-6 border border-slate-700/50"
            >
                <h3 className="text-lg font-semibold text-white mb-6">Step Details</h3>
                <div className="space-y-2">
                    {steps.map((step, index) => (
                        <StepIndicator
                            key={step.id}
                            step={step}
                            index={index}
                            isActive={index === currentStepIndex}
                            isLast={index === steps.length - 1}
                        />
                    ))}
                </div>
            </motion.div>

            {/* Error Message */}
            {error && (
                <ErrorDisplay
                    error={error}
                    toToken={state.toToken}
                    onAdjustSlippage={handleAdjustSlippage}
                />
            )}

            {/* Success Message */}
            {phase === "completed" && (
                <motion.div
                    initial={{ opacity: 0, scale: 0.95 }}
                    animate={{ opacity: 1, scale: 1 }}
                    className="bg-emerald-500/10 rounded-xl p-6 border border-emerald-500/30"
                >
                    <div className="flex items-center gap-3">
                        <CheckCircle2 className="w-6 h-6 text-emerald-400" />
                        <div>
                            <h4 className="text-emerald-400 font-semibold">Transfer Successful!</h4>
                            <p className="text-emerald-300 text-sm mt-1">
                                Your assets have been successfully transferred.
                            </p>
                        </div>
                    </div>
                </motion.div>
            )}

            {/* Transaction Links */}
            {steps.some((s) => s.txHash) && (
                <motion.div
                    initial={{ opacity: 0, y: 20 }}
                    animate={{ opacity: 1, y: 0 }}
                    transition={{ delay: 0.4 }}
                    className="bg-slate-800/30 rounded-xl p-6 border border-slate-700/50"
                >
                    <h3 className="text-lg font-semibold text-white mb-4">Transaction Hashes</h3>
                    <div className="space-y-2">
                        {steps
                            .filter((s) => s.txHash)
                            .map((step, index) => {
                                const explorerUrl = getExplorerUrl(step.fromChain, step.txHash);
                                return (
                                    <div
                                        key={step.id}
                                        className="flex items-center justify-between py-2 border-b border-slate-700/50 last:border-0"
                                    >
                                        <div>
                                            <span className="text-sm text-slate-400">
                                                Step {index + 1} ({step.fromChain})
                                            </span>
                                            <p className="text-xs text-slate-500 font-mono truncate max-w-xs">
                                                {step.txHash}
                                            </p>
                                        </div>
                                        {explorerUrl && (
                                            <Link
                                                href={explorerUrl}
                                                target="_blank"
                                                rel="noopener noreferrer"
                                                className="flex items-center gap-1 text-xs text-teal-400 hover:text-teal-300"
                                            >
                                                View <ExternalLink className="w-3 h-3" />
                                            </Link>
                                        )}
                                    </div>
                                );
                            })}
                    </div>
                </motion.div>
            )}

            {/* Action Buttons */}
            <motion.div
                initial={{ opacity: 0, y: 20 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: 0.5 }}
                className="flex justify-center gap-4"
            >
                {canRetry && (
                    <button
                        type="button"
                        onClick={handleRetry}
                        disabled={taskProvider.isExecuting}
                        className="flex items-center gap-2 px-6 py-3 bg-amber-500/20 text-amber-400 rounded-xl font-medium hover:bg-amber-500/30 transition-colors disabled:opacity-50"
                    >
                        {taskProvider.isExecuting ? (
                            <Loader2 className="w-5 h-5 animate-spin" />
                        ) : (
                            <RotateCcw className="w-5 h-5" />
                        )}
                        Retry Failed Step
                    </button>
                )}

                {canGoBack && (
                    <button
                        type="button"
                        onClick={onBack}
                        className="flex items-center gap-2 px-6 py-3 bg-slate-700 text-white rounded-xl font-medium hover:bg-slate-600 transition-colors"
                    >
                        <ArrowLeft className="w-5 h-5" />
                        New Transfer
                    </button>
                )}

                {!canGoBack && (
                    <div className="flex items-center gap-2 text-slate-400">
                        <Loader2 className="w-5 h-5 animate-spin" />
                        <span>Processing transfer...</span>
                    </div>
                )}
            </motion.div>
        </div>
    );
}

// ============================================================================
// Error Display Component
// ============================================================================

interface ErrorDisplayProps {
    error: string;
    toToken: { symbol: string; decimals: number } | null;
    onAdjustSlippage: (newSlippageBps: number) => void;
}

function ErrorDisplay({ error, toToken, onAdjustSlippage }: ErrorDisplayProps) {
    const slippageInfo = useMemo(() => parseSlippageError(error), [error]);
    const [slippageApplied, setSlippageApplied] = useState(false);

    // Format the error message
    const formattedMessage = useMemo(() => {
        if (slippageInfo.isSlippageError) {
            return formatSlippageErrorMessage(
                slippageInfo,
                toToken?.symbol,
                toToken?.decimals ?? 6,
            );
        }
        return error;
    }, [slippageInfo, error, toToken]);

    const handleApplySuggestedSlippage = () => {
        if (slippageInfo.suggestedSlippageBps) {
            onAdjustSlippage(slippageInfo.suggestedSlippageBps);
            setSlippageApplied(true);
        }
    };

    return (
        <motion.div
            initial={{ opacity: 0, scale: 0.95 }}
            animate={{ opacity: 1, scale: 1 }}
            className={`rounded-xl p-6 border ${
                slippageInfo.isSlippageError
                    ? "bg-amber-500/10 border-amber-500/30"
                    : "bg-red-500/10 border-red-500/30"
            }`}
        >
            <div className="flex items-start gap-3">
                {slippageInfo.isSlippageError ? (
                    <AlertTriangle className="w-6 h-6 text-amber-400 shrink-0 mt-0.5" />
                ) : (
                    <XCircle className="w-6 h-6 text-red-400 shrink-0 mt-0.5" />
                )}
                <div className="flex-1">
                    <h4
                        className={`font-semibold ${
                            slippageInfo.isSlippageError ? "text-amber-400" : "text-red-400"
                        }`}
                    >
                        {slippageInfo.isSlippageError ? "Price Moved Too Much" : "Error Occurred"}
                    </h4>
                    <p
                        className={`text-sm mt-1 ${
                            slippageInfo.isSlippageError ? "text-amber-300" : "text-red-300"
                        }`}
                    >
                        {formattedMessage}
                    </p>

                    {/* Slippage adjustment suggestion */}
                    {slippageInfo.isSlippageError && slippageInfo.suggestedSlippageBps && (
                        <div className="mt-4 p-3 bg-slate-800/50 rounded-lg border border-slate-700/50">
                            <div className="flex items-center justify-between">
                                <div className="flex items-center gap-2">
                                    <Settings2 className="w-4 h-4 text-amber-400" />
                                    <span className="text-sm text-slate-300">
                                        Suggested slippage:{" "}
                                        <span className="font-medium text-amber-400">
                                            {(slippageInfo.suggestedSlippageBps / 100).toFixed(1)}%
                                        </span>
                                    </span>
                                </div>
                                {!slippageApplied ? (
                                    <button
                                        type="button"
                                        onClick={handleApplySuggestedSlippage}
                                        className="px-3 py-1 text-xs font-medium bg-amber-500/20 text-amber-400 rounded hover:bg-amber-500/30 transition-colors"
                                    >
                                        Apply & Retry
                                    </button>
                                ) : (
                                    <span className="text-xs text-emerald-400 flex items-center gap-1">
                                        <CheckCircle2 className="w-3 h-3" />
                                        Applied
                                    </span>
                                )}
                            </div>
                            <p className="text-xs text-slate-400 mt-2">
                                Click &quot;Apply & Retry&quot; then use the &quot;Retry Failed
                                Step&quot; button below to try again with higher slippage tolerance.
                            </p>
                        </div>
                    )}
                </div>
            </div>

            {/* Show raw error for debugging in non-slippage cases */}
            {!slippageInfo.isSlippageError && error !== formattedMessage && (
                <details className="mt-3">
                    <summary className="text-xs text-slate-500 cursor-pointer hover:text-slate-400">
                        Technical details
                    </summary>
                    <pre className="mt-2 text-xs text-slate-500 overflow-x-auto whitespace-pre-wrap break-all">
                        {error}
                    </pre>
                </details>
            )}
        </motion.div>
    );
}
