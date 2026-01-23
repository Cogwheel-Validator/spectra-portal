"use client";

import { CheckCircle2, Circle, Loader2 } from "lucide-react";
import { motion } from "motion/react";
import Image from "next/image";
import type { ClientConfig } from "@/components/modules/tomlTypes";

interface ChainProgressProps {
    chainPath: string[];
    currentChainIndex: number;
    config: ClientConfig;
    status: "pending" | "executing" | "completed" | "failed";
}

export default function ChainProgress({
    chainPath,
    currentChainIndex,
    config,
    status,
}: ChainProgressProps) {
    const getChainData = (chainId: string) => {
        return config.chains.find((c) => c.id === chainId);
    };

    const getChainStatus = (index: number) => {
        if (status === "completed") return "completed";
        if (status === "failed" && index === currentChainIndex) return "failed";
        if (index < currentChainIndex) return "completed";
        if (index === currentChainIndex) return "active";
        return "pending";
    };

    return (
        <div className="relative">
            {/* Progress Line Container */}
            <div className="absolute top-8 left-0 right-0 flex items-center justify-center pointer-events-none">
                <div className="relative w-full mx-12">
                    {/* Progress Line Background */}
                    <div className="absolute inset-0 h-1 bg-slate-700 rounded-full" />

                    {/* Animated Progress Line */}
                    <motion.div
                        className="absolute left-0 top-0 h-1 bg-linear-to-r from-teal-500 to-emerald-500 rounded-full z-10"
                        initial={{ width: "0%" }}
                        animate={{
                            width:
                                status === "completed"
                                    ? "100%"
                                    : `${(currentChainIndex / (chainPath.length - 1)) * 100}%`,
                        }}
                        transition={{ duration: 0.5, ease: "easeOut" }}
                    />
                </div>
            </div>

            {/* Chain Nodes */}
            <div className="flex justify-between items-start relative z-20">
                {chainPath.map((chainId, index) => {
                    const chain = getChainData(chainId);
                    const chainStatus = getChainStatus(index);

                    return (
                        <div
                            key={chainId}
                            className="flex flex-col items-center"
                            style={{ width: `${100 / chainPath.length}%` }}
                        >
                            {/* Chain Icon Container */}
                            <motion.div
                                className={`
                                    relative w-16 h-16 rounded-full flex items-center justify-center
                                    ${
                                        chainStatus === "active"
                                            ? "ring-4 ring-teal-500/50"
                                            : chainStatus === "completed"
                                              ? "ring-4 ring-emerald-500/30"
                                              : chainStatus === "failed"
                                                ? "ring-4 ring-red-500/50"
                                                : ""
                                    }
                                `}
                                initial={{ scale: 1 }}
                                animate={{
                                    scale: chainStatus === "active" ? [1, 1.05, 1] : 1,
                                }}
                                transition={{
                                    duration: 1.5,
                                    repeat: chainStatus === "active" ? Infinity : 0,
                                    ease: "easeInOut",
                                }}
                            >
                                {/* Chain Logo */}
                                <Image
                                    src={chain?.chain_logo || "/unknown.jpg"}
                                    alt={chain?.name || chainId}
                                    width={64}
                                    height={64}
                                    className={`
                                        rounded-full border-4
                                        ${
                                            chainStatus === "completed"
                                                ? "border-emerald-500"
                                                : chainStatus === "active"
                                                  ? "border-teal-500"
                                                  : chainStatus === "failed"
                                                    ? "border-red-500"
                                                    : "border-slate-600 opacity-50"
                                        }
                                    `}
                                />

                                {/* Status Indicator Overlay */}
                                <div className="absolute -bottom-1 -right-1">
                                    {chainStatus === "completed" && (
                                        <motion.div
                                            initial={{ scale: 0 }}
                                            animate={{ scale: 1 }}
                                            className="w-6 h-6 bg-slate-900 rounded-full flex items-center justify-center"
                                        >
                                            <CheckCircle2 className="w-5 h-5 text-emerald-400" />
                                        </motion.div>
                                    )}
                                    {chainStatus === "active" && (
                                        <div className="w-6 h-6 bg-slate-900 rounded-full flex items-center justify-center">
                                            <Loader2 className="w-5 h-5 text-teal-400 animate-spin" />
                                        </div>
                                    )}
                                    {chainStatus === "failed" && (
                                        <div className="w-6 h-6 bg-slate-900 rounded-full flex items-center justify-center">
                                            <Circle className="w-5 h-5 text-red-400" />
                                        </div>
                                    )}
                                </div>

                                {/* Animated Pulse for Active Chain */}
                                {chainStatus === "active" && (
                                    <motion.div
                                        className="absolute inset-0 rounded-full bg-teal-500/20"
                                        animate={{
                                            scale: [1, 1.3, 1],
                                            opacity: [0.5, 0, 0.5],
                                        }}
                                        transition={{
                                            duration: 2,
                                            repeat: Infinity,
                                            ease: "easeOut",
                                        }}
                                    />
                                )}
                            </motion.div>

                            {/* Chain Name */}
                            <p
                                className={`
                                    mt-3 text-sm font-medium text-center
                                    ${
                                        chainStatus === "completed"
                                            ? "text-emerald-400"
                                            : chainStatus === "active"
                                              ? "text-teal-400"
                                              : chainStatus === "failed"
                                                ? "text-red-400"
                                                : "text-slate-500"
                                    }
                                `}
                            >
                                {chain?.name || chainId}
                            </p>

                            {/* Step Label */}
                            <p className="text-xs text-slate-500 mt-1">
                                {index === 0
                                    ? "Source"
                                    : index === chainPath.length - 1
                                      ? "Destination"
                                      : `Step ${index}`}
                            </p>
                        </div>
                    );
                })}
            </div>
        </div>
    );
}
