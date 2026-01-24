"use client";

import { ChevronDown } from "lucide-react";
import Image from "next/image";
import { useCallback, useEffect, useRef, useState } from "react";
import type { ClientChain } from "@/components/modules/tomlTypes";

interface ChainDropdownProps {
    chains: ClientChain[];
    selectedChainId: string;
    onSelect: (chainId: string) => void;
    placeholder?: string;
    disabled?: boolean;
    label?: string;
    variant?: "from" | "to";
}

export default function ChainDropdown({
    chains,
    selectedChainId,
    onSelect,
    placeholder = "Select chain",
    disabled = false,
    label,
    variant = "from",
}: ChainDropdownProps) {
    const [isOpen, setIsOpen] = useState(false);
    const [searchQuery, setSearchQuery] = useState("");
    const dropdownRef = useRef<HTMLDivElement>(null);
    const inputRef = useRef<HTMLInputElement>(null);

    const selectedChain = chains.find((c) => c.id === selectedChainId);

    // Filter chains based on search query
    const filteredChains = chains.filter(
        (chain) =>
            chain.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
            chain.id.toLowerCase().includes(searchQuery.toLowerCase()),
    );

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
        (chainId: string) => {
            onSelect(chainId);
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
            } else if (e.key === "Enter" && filteredChains.length === 1) {
                handleSelect(filteredChains[0].id);
            }
        },
        [filteredChains, handleSelect],
    );

    const borderColor = variant === "from" ? "border-orange-500" : "border-teal-400";
    const shadowColor = variant === "from" ? "shadow-orange-800/70" : "shadow-teal-700/70";
    const hoverBg = variant === "from" ? "hover:bg-orange-500/20" : "hover:bg-teal-400/20";

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
                    {selectedChain ? (
                        <>
                            <Image
                                src={selectedChain.chain_logo || "/unknown.jpg"}
                                alt={selectedChain.name}
                                width={32}
                                height={32}
                                className={`rounded-full border-2 ${borderColor} shadow-lg ${shadowColor}`}
                            />
                            <span className="font-medium text-white">{selectedChain.name}</span>
                        </>
                    ) : (
                        <>
                            <div className="w-8 h-8 rounded-full bg-slate-700 flex items-center justify-center">
                                <span className="text-slate-400 text-sm">?</span>
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
                            placeholder="Search chains..."
                            value={searchQuery}
                            onChange={(e) => setSearchQuery(e.target.value)}
                            onKeyDown={handleKeyDown}
                            className="w-full px-3 py-2 bg-slate-900/50 border border-slate-700 rounded-md text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-slate-500"
                        />
                    </div>

                    {/* Chain List */}
                    <div className="max-h-64 overflow-y-auto">
                        {filteredChains.length === 0 ? (
                            <div className="px-4 py-3 text-slate-400 text-center">
                                No chains found
                            </div>
                        ) : (
                            filteredChains.map((chain) => (
                                <button
                                    key={chain.id}
                                    type="button"
                                    onClick={() => handleSelect(chain.id)}
                                    className={`
                                        w-full flex items-center gap-3 px-4 py-3 text-left
                                        transition-colors duration-150
                                        ${chain.id === selectedChainId ? "bg-slate-700/50" : hoverBg}
                                    `}
                                >
                                    <Image
                                        src={chain.chain_logo || "/unknown.jpg"}
                                        alt={chain.name}
                                        width={28}
                                        height={28}
                                        className="rounded-full"
                                    />
                                    <div className="flex flex-col">
                                        <span className="font-medium text-white">{chain.name}</span>
                                        <span className="text-xs text-slate-400">{chain.id}</span>
                                    </div>
                                    {chain.id === selectedChainId && (
                                        <span className="ml-auto text-teal-400">✓</span>
                                    )}
                                </button>
                            ))
                        )}
                    </div>
                </div>
            )}
        </div>
    );
}
