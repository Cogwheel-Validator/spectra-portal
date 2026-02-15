"use client";

import { Hand, Settings2, Zap } from "lucide-react";
import { useState } from "react";
import type { TransferMode } from "@/context/transferContext";

interface TransferModeToggleProps {
    mode: TransferMode;
    onModeChange: (mode: TransferMode) => void;
    slippageBps: number;
    onSlippageChange: (bps: number) => void;
    disabled?: boolean;
    supportsPfm?: boolean;
    supportsWasm?: boolean;
    isDirectRoute?: boolean;
}

const SLIPPAGE_PRESETS = [50, 100, 200, 500]; // 0.5%, 1%, 2%, 5%

export default function TransferModeToggle({
    mode,
    onModeChange,
    slippageBps,
    onSlippageChange,
    disabled = false,
    supportsPfm = false,
    supportsWasm = false,
    isDirectRoute = false,
}: TransferModeToggleProps) {
    const [showSlippageSettings, setShowSlippageSettings] = useState(false);
    const [customSlippage, setCustomSlippage] = useState("");

    const showSmartOption = !isDirectRoute;

    const handleCustomSlippageChange = (value: string) => {
        setCustomSlippage(value);
        const numValue = Number.parseFloat(value);
        if (!Number.isNaN(numValue) && numValue > 0 && numValue <= 50) {
            onSlippageChange(Math.round(numValue * 100)); // Convert percentage to bps
        }
    };

    const getSlippageDisplay = () => {
        return `${(slippageBps / 100).toFixed(slippageBps % 100 === 0 ? 0 : 1)}%`;
    };

    return (
        <div className="flex flex-col gap-3">
            {/* Transfer Mode Section */}
            <div className="flex flex-col gap-2">
                <div className="flex items-center justify-between">
                    <span className="text-sm font-medium text-gray-300">Transfer Mode</span>
                    {showSmartOption && (
                        <span className="text-xs text-teal-400">
                            {supportsWasm ? "WASM available" : "PFM available"}
                        </span>
                    )}
                    {isDirectRoute && !showSmartOption && (
                        <span className="text-xs text-slate-400">Direct route</span>
                    )}
                </div>

                <div className="flex gap-2 p-1 bg-slate-800/50 rounded-lg border border-slate-600/50">
                    {/* Smart Transfer Button */}
                    <button
                        type="button"
                        onClick={() => onModeChange("smart")}
                        disabled={disabled || !showSmartOption}
                        className={`
                            flex-1 flex items-center justify-center gap-2 px-3 py-2 rounded-md
                            transition-all duration-200 font-medium text-sm tooltip
                            ${
                                mode === "smart" && showSmartOption
                                    ? "bg-linear-to-r from-teal-500 to-teal-600 text-white shadow-lg shadow-teal-500/25"
                                    : "text-slate-400 hover:text-white hover:bg-slate-700/50"
                            }
                            ${disabled || !showSmartOption ? "opacity-50 cursor-not-allowed" : ""}
                        `}
                        data-tip={`Smart transfer gives user the power to execute multiple transaction at once
                            by leveraging Skip Go Smart Contracts or Packet Forwarding Middleware.`}
                    >
                        <Zap className="w-4 h-4" />
                        <span>Smart</span>
                    </button>

                    {/* Manual Transfer Button */}
                    <button
                        type="button"
                        onClick={() => onModeChange("manual")}
                        disabled={disabled}
                        className={`
                            flex-1 flex items-center justify-center gap-2 px-3 py-2 rounded-md
                            transition-all duration-200 font-medium text-sm tooltip
                            ${
                                mode === "manual" || (!showSmartOption && !disabled)
                                    ? "bg-linear-to-r from-orange-500 to-orange-600 text-white shadow-lg shadow-orange-500/25"
                                    : "text-slate-400 hover:text-white hover:bg-slate-700/50"
                            }
                            ${disabled ? "opacity-50 cursor-not-allowed" : ""}
                        `}
                        data-tip={`Manual transfer gives the user the most control over every transaction. 
                            However it also requires to have enough tokens to pay for the gas fees.`}
                    >
                        <Hand className="w-4 h-4" />
                        <span>Manual</span>
                    </button>
                </div>

                {/* Mode Description */}
                <p className="text-xs text-slate-500">
                    {mode === "smart" && showSmartOption ? (
                        <>
                            <span className="text-teal-400">Smart:</span> Execute in fewest
                            transactions using{" "}
                            {supportsWasm
                                ? "WASM contracts"
                                : supportsPfm
                                  ? "Packet Forwarding"
                                  : "Direct"}
                        </>
                    ) : isDirectRoute ? (
                        <>
                            <span className="text-orange-400">Direct:</span> Single IBC transfer to
                            destination chain
                        </>
                    ) : (
                        <>
                            <span className="text-orange-400">Manual:</span> Execute each step
                            individually for full control
                        </>
                    )}
                </p>
            </div>

            {/* Slippage Settings Section */}
            <div className="border-t border-slate-700/50 pt-3">
                <button
                    type="button"
                    onClick={() => setShowSlippageSettings(!showSlippageSettings)}
                    className="w-full flex items-center justify-between text-sm text-slate-400 hover:text-white transition-colors"
                >
                    <div className="flex items-center gap-2">
                        <Settings2 className="w-4 h-4" />
                        <span>Slippage Tolerance</span>
                    </div>
                    <span className="font-medium text-slate-300">{getSlippageDisplay()}</span>
                </button>

                {showSlippageSettings && (
                    <div className="mt-3 space-y-2">
                        {/* Preset Buttons */}
                        <div className="flex gap-2">
                            {SLIPPAGE_PRESETS.map((preset) => (
                                <button
                                    key={preset}
                                    type="button"
                                    onClick={() => {
                                        onSlippageChange(preset);
                                        setCustomSlippage("");
                                    }}
                                    className={`
                                        flex-1 py-1.5 rounded-md text-xs font-medium transition-colors
                                        ${
                                            slippageBps === preset
                                                ? "bg-teal-500/20 text-teal-400 border border-teal-500/50"
                                                : "bg-slate-700/50 text-slate-400 hover:text-white border border-slate-600/50"
                                        }
                                    `}
                                >
                                    {preset / 100}%
                                </button>
                            ))}
                        </div>

                        {/* Custom Input */}
                        <div className="flex items-center gap-2">
                            <input
                                type="text"
                                placeholder="Custom"
                                value={customSlippage}
                                onChange={(e) => handleCustomSlippageChange(e.target.value)}
                                className="flex-1 px-3 py-1.5 bg-slate-700/50 border border-slate-600/50 rounded-md text-sm text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-teal-500/50"
                            />
                            <span className="text-sm text-slate-400">%</span>
                        </div>

                        {/* Warning for high slippage */}
                        {slippageBps > 300 && (
                            <p className="text-xs text-yellow-400">
                                ⚠️ High slippage may result in unfavorable rates
                            </p>
                        )}
                    </div>
                )}
            </div>
        </div>
    );
}
