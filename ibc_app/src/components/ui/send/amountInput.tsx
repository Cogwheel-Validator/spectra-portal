"use client";

import { useCallback, useState } from "react";
import type { ClientToken } from "@/components/modules/tomlTypes";

interface AmountInputProps {
    value: string;
    onChange: (value: string) => void;
    token: ClientToken | null;
    disabled?: boolean;
    isLoading?: boolean;
    balance?: string;
    label?: string;
}

export default function AmountInput({
    value,
    onChange,
    token,
    disabled = false,
    isLoading = false,
    balance,
    label = "Amount",
}: AmountInputProps) {
    const [isFocused, setIsFocused] = useState(false);

    const handleChange = useCallback(
        (e: React.ChangeEvent<HTMLInputElement>) => {
            const newValue = e.target.value;
            // Allow empty string, numbers, and decimals
            if (newValue === "" || /^\d*\.?\d*$/.test(newValue)) {
                onChange(newValue);
            }
        },
        [onChange],
    );

    const handleMaxClick = useCallback(() => {
        if (balance) {
            onChange(balance);
        }
    }, [balance, onChange]);

    const handleHalfClick = useCallback(() => {
        if (balance) {
            const halfBalance = (Number.parseFloat(balance) / 2).toString();
            onChange(halfBalance);
        }
    }, [balance, onChange]);

    const isDisabled = disabled || !token;

    return (
        <div className="space-y-2">
            <div className="flex justify-between items-center">
                <span className="text-sm font-medium text-gray-300">{label}</span>
                {balance && token && (
                    <span className="text-xs text-slate-400">
                        Balance: {balance} {token.symbol}
                    </span>
                )}
            </div>

            <div
                className={`
                    relative flex items-center
                    bg-slate-800/50 rounded-lg border
                    transition-all duration-200
                    ${isDisabled ? "border-slate-700 opacity-50" : isFocused ? "border-slate-400 ring-2 ring-slate-500/50" : "border-slate-600/50 hover:border-slate-500"}
                `}
            >
                <input
                    type="text"
                    inputMode="decimal"
                    placeholder={isDisabled ? "Select destination asset first" : "0.00"}
                    value={value}
                    onChange={handleChange}
                    onFocus={() => setIsFocused(true)}
                    onBlur={() => setIsFocused(false)}
                    disabled={isDisabled}
                    className={`
                        flex-1 px-4 py-3 bg-transparent text-white text-lg font-medium
                        placeholder-slate-500 focus:outline-none
                        ${isDisabled ? "cursor-not-allowed" : ""}
                    `}
                />

                {/* Token Symbol Badge */}
                {token && (
                    <div className="flex items-center gap-2 pr-4">
                        {/* Quick Amount Buttons */}
                        {balance && !isDisabled && (
                            <div className="flex gap-1">
                                <button
                                    type="button"
                                    onClick={handleHalfClick}
                                    className="px-2 py-1 text-xs font-medium text-slate-400 bg-slate-700/50 rounded hover:bg-slate-700 hover:text-white transition-colors"
                                >
                                    50%
                                </button>
                                <button
                                    type="button"
                                    onClick={handleMaxClick}
                                    className="px-2 py-1 text-xs font-medium text-slate-400 bg-slate-700/50 rounded hover:bg-slate-700 hover:text-white transition-colors"
                                >
                                    MAX
                                </button>
                            </div>
                        )}
                        <span className="text-slate-300 font-medium">{token.symbol}</span>
                    </div>
                )}

                {/* Loading Indicator */}
                {isLoading && (
                    <div className="absolute right-4 top-1/2 -translate-y-1/2">
                        <div className="w-5 h-5 border-2 border-slate-600 border-t-teal-400 rounded-full animate-spin" />
                    </div>
                )}
            </div>

            {/* Validation Message */}
            {value && token && Number.parseFloat(value) > 0 && (
                <div className="text-xs text-slate-400">
                    {Number.parseFloat(value).toLocaleString()} {token.symbol}
                    {balance && Number.parseFloat(value) > Number.parseFloat(balance) && (
                        <span className="text-red-400 ml-2">Exceeds balance</span>
                    )}
                </div>
            )}
        </div>
    );
}
