"use client";

import { ChevronDown, Coins } from "lucide-react";
import Image from "next/image";
import { useCallback, useEffect, useRef, useState } from "react";
import type { ClientToken } from "@/components/modules/tomlTypes";

interface AssetDropdownProps {
    tokens: ClientToken[];
    selectedSymbol: string;
    onSelect: (symbol: string) => void;
    placeholder?: string;
    disabled?: boolean;
    label?: string;
    showIcon?: boolean;
}

export default function AssetDropdown({
    tokens,
    selectedSymbol,
    onSelect,
    placeholder = "Select asset",
    disabled = false,
    label,
    showIcon = true,
}: AssetDropdownProps) {
    const [isOpen, setIsOpen] = useState(false);
    const [searchQuery, setSearchQuery] = useState("");
    const dropdownRef = useRef<HTMLDivElement>(null);
    const inputRef = useRef<HTMLInputElement>(null);

    const selectedToken = tokens.find((t) => t.symbol === selectedSymbol);

    // Filter tokens based on search query
    const filteredTokens = tokens.filter(
        (token) =>
            token.symbol.toLowerCase().includes(searchQuery.toLowerCase()) ||
            token.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
            token.denom.toLowerCase().includes(searchQuery.toLowerCase()),
    );

    // Group tokens by native vs IBC
    const nativeTokens = filteredTokens.filter((t) => t.is_native);
    const ibcTokens = filteredTokens.filter((t) => !t.is_native);

    // Close dropdown when clicking outside
    useEffect(() => {
        const handleClickOutside = (event: MouseEvent) => {
            if (dropdownRef.current && !dropdownRef.current.contains(event.target as Node)) {
                setIsOpen(false);
                setSearchQuery("");
            }
        };

        document.addEventListener("mousedown", handleClickOutside);
        return () => document.removeEventListener("mousedown", handleClickOutside);
    }, []);

    // Focus search input when dropdown opens
    useEffect(() => {
        if (isOpen && inputRef.current) {
            inputRef.current.focus();
        }
    }, [isOpen]);

    const handleSelect = useCallback(
        (symbol: string) => {
            onSelect(symbol);
            setIsOpen(false);
            setSearchQuery("");
        },
        [onSelect],
    );

    const handleKeyDown = useCallback(
        (e: React.KeyboardEvent) => {
            if (e.key === "Escape") {
                setIsOpen(false);
                setSearchQuery("");
            } else if (e.key === "Enter" && filteredTokens.length === 1) {
                handleSelect(filteredTokens[0].symbol);
            }
        },
        [filteredTokens, handleSelect],
    );

    const TokenIcon = ({ token }: { token: ClientToken }) => {
        if (token.icon) {
            return (
                <Image
                    src={token.icon}
                    alt={token.symbol}
                    width={28}
                    height={28}
                    className="rounded-full"
                />
            );
        }
        return (
            <div className="w-7 h-7 rounded-full bg-linear-to-br from-slate-600 to-slate-700 flex items-center justify-center">
                <Coins className="w-4 h-4 text-slate-300" />
            </div>
        );
    };

    const TokenRow = ({ token, isSelected }: { token: ClientToken; isSelected: boolean }) => (
        <button
            key={token.denom}
            type="button"
            onClick={() => handleSelect(token.symbol)}
            className={`
                w-full flex items-center gap-3 px-4 py-3 text-left
                transition-colors duration-150
                ${isSelected ? "bg-slate-700/50" : "hover:bg-slate-700/30"}
            `}
        >
            {showIcon && <TokenIcon token={token} />}
            <div className="flex flex-col flex-1 min-w-0">
                <div className="flex items-center gap-2">
                    <span className="font-medium text-white">{token.symbol}</span>
                    {!token.is_native && (
                        <span className="text-xs px-1.5 py-0.5 bg-slate-600/50 rounded text-slate-400">
                            IBC
                        </span>
                    )}
                </div>
                <span className="text-xs text-slate-400 truncate">{token.name}</span>
            </div>
            {token.origin_chain_name && !token.is_native && (
                <span className="text-xs text-slate-500">from {token.origin_chain_name}</span>
            )}
            {isSelected && <span className="text-teal-400">✓</span>}
        </button>
    );

    return (
        <div className="relative" ref={dropdownRef}>
            {label && <span className="text-sm font-medium text-gray-300 mb-1 block">{label}</span>}

            {/* Trigger Button */}
            <button
                type="button"
                onClick={() => !disabled && setIsOpen(!isOpen)}
                disabled={disabled}
                className={`
                    w-full flex items-center justify-between gap-3 px-4 py-3
                    bg-slate-800/50 rounded-lg border border-slate-600/50
                    transition-all duration-200
                    ${disabled ? "opacity-50 cursor-not-allowed" : "hover:bg-slate-700/50 hover:border-slate-500 cursor-pointer"}
                    ${isOpen ? "ring-2 ring-slate-500" : ""}
                `}
            >
                <div className="flex items-center gap-3">
                    {selectedToken ? (
                        <>
                            {showIcon && <TokenIcon token={selectedToken} />}
                            <div className="flex flex-col items-start">
                                <span className="font-medium text-white">
                                    {selectedToken.symbol}
                                </span>
                                <span className="text-xs text-slate-400">{selectedToken.name}</span>
                            </div>
                        </>
                    ) : (
                        <>
                            <div className="w-7 h-7 rounded-full bg-slate-700 flex items-center justify-center">
                                <Coins className="w-4 h-4 text-slate-400" />
                            </div>
                            <span className="text-slate-400">{placeholder}</span>
                        </>
                    )}
                </div>
                <ChevronDown
                    className={`w-5 h-5 text-slate-400 transition-transform duration-200 ${isOpen ? "rotate-180" : ""}`}
                />
            </button>

            {/* Dropdown Menu */}
            {isOpen && (
                <div className="absolute z-50 w-full mt-2 bg-slate-800 border border-slate-600 rounded-lg shadow-xl overflow-hidden">
                    {/* Search Input */}
                    <div className="p-2 border-b border-slate-700">
                        <input
                            ref={inputRef}
                            type="text"
                            placeholder="Search assets..."
                            value={searchQuery}
                            onChange={(e) => setSearchQuery(e.target.value)}
                            onKeyDown={handleKeyDown}
                            className="w-full px-3 py-2 bg-slate-900/50 border border-slate-700 rounded-md text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-slate-500"
                        />
                    </div>

                    {/* Token List */}
                    <div className="max-h-72 overflow-y-auto">
                        {filteredTokens.length === 0 ? (
                            <div className="px-4 py-3 text-slate-400 text-center">
                                No assets found
                            </div>
                        ) : (
                            <>
                                {/* Native Tokens Section */}
                                {nativeTokens.length > 0 && (
                                    <>
                                        <div className="px-4 py-2 bg-slate-900/50 text-xs font-medium text-slate-400 uppercase tracking-wider">
                                            Native Assets
                                        </div>
                                        {nativeTokens.map((token) => (
                                            <TokenRow
                                                key={token.denom}
                                                token={token}
                                                isSelected={token.symbol === selectedSymbol}
                                            />
                                        ))}
                                    </>
                                )}

                                {/* IBC Tokens Section */}
                                {ibcTokens.length > 0 && (
                                    <>
                                        <div className="px-4 py-2 bg-slate-900/50 text-xs font-medium text-slate-400 uppercase tracking-wider">
                                            IBC Assets
                                        </div>
                                        {ibcTokens.map((token) => (
                                            <TokenRow
                                                key={token.denom}
                                                token={token}
                                                isSelected={token.symbol === selectedSymbol}
                                            />
                                        ))}
                                    </>
                                )}
                            </>
                        )}
                    </div>
                </div>
            )}
        </div>
    );
}
