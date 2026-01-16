"use client";

import Image from "next/image";
import Link from "next/link";
import { type ReactElement, useCallback, useMemo } from "react";
import { FaWpexplorer } from "react-icons/fa6";
import type { ClientChain, ClientConfig, ClientToken } from "@/components/modules/tomlTypes";
import AssetDropdown from "./assetDropdown";
import ChainDropdown from "./chainDropdown";

/**
 * Props for ChainSection component.
 * This component is kept for backwards compatibility but the main UI now uses
 * the individual dropdown components directly in senderUi.tsx
 */
interface ChainSelectionProps {
    config: ClientConfig;
    isPending: boolean;
    chainId: string;
    tokenSymbol: string;
    onChainChange: (chainId: string) => void;
    onTokenChange: (tokenSymbol: string) => void;
    isFromChain: boolean;
    otherChainId?: string;
    availableChains?: ClientChain[];
}

/**
 * ChainSection - A combined chain and asset selector
 *
 * This component wraps the ChainDropdown and AssetDropdown components
 * into a card layout. Used for backwards compatibility.
 */
export default function ChainSection(props: ChainSelectionProps): ReactElement {
    const {
        config,
        isPending,
        onChainChange,
        onTokenChange,
        isFromChain,
        chainId,
        tokenSymbol,
        otherChainId,
        availableChains,
    } = props;

    // Helper to get chain by ID
    const getChainById = useCallback(
        (id: string): ClientChain | undefined => {
            return config.chains.find((chain) => chain.id === id);
        },
        [config.chains],
    );

    // Helper to get tokens for a chain
    const getTokensForChain = useCallback(
        (id: string): ClientToken[] => {
            const chain = getChainById(id);
            if (!chain) return [];
            return [...chain.native_tokens, ...chain.ibc_tokens];
        },
        [getChainById],
    );

    // Helper to get sendable tokens between chains
    const getSendableTokens = useCallback(
        (fromId: string, toId: string): string[] => {
            const fromChain = getChainById(fromId);
            if (!fromChain) return [];
            const connectedChain = fromChain.connected_chains.find((chain) => chain.id === toId);
            return connectedChain?.sendable_tokens ?? [];
        },
        [getChainById],
    );

    // Derived data
    const chainData = useMemo(() => getChainById(chainId), [chainId, getChainById]);

    const allAvailableTokens = useMemo(
        () => getTokensForChain(chainId),
        [chainId, getTokensForChain],
    );

    const sendableTokens = useMemo(() => {
        if (!isFromChain || !otherChainId) return [];
        return getSendableTokens(chainId, otherChainId);
    }, [isFromChain, otherChainId, chainId, getSendableTokens]);

    const availableTokens = useMemo(() => {
        if (!isFromChain || sendableTokens.length === 0) return allAvailableTokens;
        return allAvailableTokens.filter((token) => sendableTokens.includes(token.symbol));
    }, [allAvailableTokens, sendableTokens, isFromChain]);

    // Determine which chains to show
    const chainsForSelection = useMemo(() => {
        return availableChains ?? config.chains;
    }, [availableChains, config.chains]);

    const title = isFromChain ? "From Chain" : "To Chain";
    const assetTitle = isFromChain ? "Send Asset" : "Receive Asset";

    // Disable to-chain selection if from-chain not selected
    const requireOtherChain = !isFromChain;
    const isChainDisabled = isPending || (requireOtherChain && !otherChainId);

    return (
        <div className="bg-slate-700/50 rounded-xl p-6 space-y-4">
            {/* Chain Header */}
            <div className="flex flex-row justify-between items-center">
                <h2 className="text-lg font-semibold text-white">{title}</h2>
                {chainData && (
                    <Link
                        href={chainData.explorer_details.base_url}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="tooltip text-slate-400 hover:text-white transition-colors"
                        data-tip="Chain Explorer"
                    >
                        <FaWpexplorer className="size-6" />
                    </Link>
                )}
            </div>

            {/* Chain Logo Display */}
            <div className="flex items-center justify-center py-2">
                <Image
                    src={chainData?.chain_logo || "/unknown.jpg"}
                    alt={chainData?.name || "No Chain Selected"}
                    width={80}
                    height={80}
                    className={`rounded-full size-20 bg-slate-900 
                        ${
                            chainData
                                ? isFromChain
                                    ? "border-[3px] border-orange-500 shadow-xl shadow-orange-800/70"
                                    : "border-[3px] border-teal-400 shadow-xl shadow-teal-700/70"
                                : "border-[3px] border-slate-600"
                        }`}
                />
            </div>

            {/* Chain Dropdown */}
            <ChainDropdown
                chains={chainsForSelection}
                selectedChainId={chainId}
                onSelect={onChainChange}
                placeholder={isFromChain ? "Select source chain" : "Select destination chain"}
                disabled={isChainDisabled}
                variant={isFromChain ? "from" : "to"}
            />

            {/* Asset Dropdown */}
            <div className="pt-4 border-t border-slate-600/50">
                <h3 className="text-sm font-medium text-slate-300 mb-2">{assetTitle}</h3>
                <AssetDropdown
                    tokens={availableTokens}
                    selectedSymbol={tokenSymbol}
                    onSelect={onTokenChange}
                    placeholder={
                        isFromChain ? "Select token to send" : "Select token to receive (optional)"
                    }
                    disabled={isPending || !chainId}
                />
            </div>
        </div>
    );
}
