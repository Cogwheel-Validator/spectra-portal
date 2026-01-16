"use client";

import { ArrowLeft, CheckCircle2, ExternalLink, Loader2, RotateCcw, XCircle } from "lucide-react";
import { motion } from "motion/react";
import Link from "next/link";
import { useCallback, useEffect, useMemo } from "react";
import type { ClientConfig } from "@/components/modules/tomlTypes";
import { useTaskProvider } from "@/context/taskProvider";
import { useTransfer } from "@/context/transferContext";
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
                return `${chain.explorer_details.base_url}${chain.explorer_details.transaction_path}/${txHash}`;
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
                <motion.div
                    initial={{ opacity: 0, scale: 0.95 }}
                    animate={{ opacity: 1, scale: 1 }}
                    className="bg-red-500/10 rounded-xl p-6 border border-red-500/30"
                >
                    <div className="flex items-start gap-3">
                        <XCircle className="w-6 h-6 text-red-400 shrink-0 mt-0.5" />
                        <div>
                            <h4 className="text-red-400 font-semibold">Error Occurred</h4>
                            <p className="text-red-300 text-sm mt-1">{error}</p>
                        </div>
                    </div>
                </motion.div>
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
