"use client";

import { CheckCircle2, Circle, Loader2, XCircle } from "lucide-react";
import type { TransferStep } from "@/context/transferContext";

interface StepIndicatorProps {
    step: TransferStep;
    index: number;
    isActive: boolean;
    isLast: boolean;
}

export default function StepIndicator({ step, index, isActive, isLast }: StepIndicatorProps) {
    const getStatusIcon = () => {
        switch (step.status) {
            case "completed":
                return <CheckCircle2 className="w-6 h-6 text-emerald-400" />;
            case "failed":
                return <XCircle className="w-6 h-6 text-red-400" />;
            case "signing":
            case "broadcasting":
            case "confirming":
                return <Loader2 className="w-6 h-6 text-teal-400 animate-spin" />;
            default:
                return (
                    <Circle
                        className={`w-6 h-6 ${isActive ? "text-teal-400" : "text-slate-500"}`}
                    />
                );
        }
    };

    const getStatusText = () => {
        switch (step.status) {
            case "pending":
                return "Pending";
            case "signing":
                return "Waiting for signature...";
            case "broadcasting":
                return "Broadcasting...";
            case "confirming":
                return "Confirming...";
            case "completed":
                return "Completed";
            case "failed":
                return "Failed";
            default:
                return step.status;
        }
    };

    const getStepTypeLabel = () => {
        switch (step.type) {
            case "ibc_transfer":
                return "IBC Transfer";
            case "pfm_transfer":
                return "PFM Transfer";
            case "swap":
                return "DEX Swap";
            case "wasm_execution":
                return "Smart Contract";
            default:
                return step.type;
        }
    };

    const statusColors = {
        pending: "bg-slate-700",
        signing: "bg-teal-500/20 border-teal-500",
        broadcasting: "bg-teal-500/20 border-teal-500",
        confirming: "bg-amber-500/20 border-amber-500",
        completed: "bg-emerald-500/10 border-emerald-500",
        failed: "bg-red-500/10 border-red-500",
    };

    return (
        <div className="flex items-start gap-4">
            {/* Step Number and Line */}
            <div className="flex flex-col items-center">
                <div
                    className={`
                        w-10 h-10 rounded-full flex items-center justify-center border-2
                        ${statusColors[step.status]}
                        ${isActive ? "ring-2 ring-teal-500/50" : ""}
                    `}
                >
                    {getStatusIcon()}
                </div>
                {!isLast && (
                    <div
                        className={`w-0.5 h-16 mt-2 ${
                            step.status === "completed" ? "bg-emerald-500" : "bg-slate-600"
                        }`}
                    />
                )}
            </div>

            {/* Step Content */}
            <div className="flex-1 pt-1">
                <div className="flex items-center gap-2">
                    <span className="text-sm font-medium text-slate-300">Step {index + 1}</span>
                    <span className="text-xs px-2 py-0.5 bg-slate-700 rounded text-slate-400">
                        {getStepTypeLabel()}
                    </span>
                </div>

                <p className="text-white font-medium mt-1">
                    {step.fromChain} → {step.toChain}
                </p>

                <p
                    className={`text-sm mt-1 ${
                        step.status === "failed"
                            ? "text-red-400"
                            : step.status === "completed"
                              ? "text-emerald-400"
                              : "text-slate-400"
                    }`}
                >
                    {getStatusText()}
                </p>

                {/* Transaction Hash */}
                {step.txHash && (
                    <p className="text-xs text-slate-500 mt-2 font-mono truncate">
                        Tx: {step.txHash}
                    </p>
                )}

                {/* Error Message */}
                {step.status === "failed" && step.error && (
                    <p className="text-xs text-red-400 mt-2">{step.error}</p>
                )}
            </div>
        </div>
    );
}
