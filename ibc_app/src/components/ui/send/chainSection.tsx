"use client";
import Image from "next/image";
import Link from "next/link";
import { type ReactElement, useCallback, useMemo } from "react";
import { FaWpexplorer } from "react-icons/fa6";
import type { ClientChain, ClientConfig, ClientToken } from "@/components/modules/tomlTypes";

/**
 * Props for ChainSection component.
 */
interface ChainSelectionProps {
    // Static configuration
    config: ClientConfig;
    isPending: boolean;

    // Current state
    chainId: string;
    tokenSymbol: string;

    // Event handlers
    onChainChange: (chainId: string) => void;
    onTokenChange: (tokenSymbol: string) => void;

    // Behavior configuration
    // Whether this is the "From Chain" (true) or "To Chain" (false)
    isFromChain: boolean;
    // Other chain's ID - used for filtering tokens (from chain) or validation (to chain)
    otherChainId?: string;
    // Available chains for dropdown (if provided, limits options; otherwise uses all chains)
    availableChains?: ClientChain[];
}

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

    // Behavior based on isFromChain and availableChains
    // - If isFromChain: filter tokens by sendable_tokens when otherChainId is set
    // - If !isFromChain: require otherChainId to enable selection
    const filterBySendable = isFromChain;
    const requireOtherChain = !isFromChain;

    // Helper functions
    const getChainById = useCallback(
        (id: string): ClientChain | undefined => {
            return config.chains.find((chain) => chain.id === id);
        },
        [config],
    );

    const getTokensForChain = useCallback(
        (id: string): ClientToken[] => {
            const chain = getChainById(id);
            if (!chain) return [];
            return [...chain.native_tokens, ...chain.ibc_tokens];
        },
        [getChainById],
    );

    const getSendableTokens = useCallback(
        (fromId: string, toId: string): string[] => {
            const fromChain = getChainById(fromId);
            if (!fromChain) return [];

            const connectedChain = fromChain.connected_chains.find((chain) => chain.id === toId);
            if (!connectedChain) return [];

            return connectedChain.sendable_tokens;
        },
        [getChainById],
    );

    // Derived data computed within this component
    const chainData = useMemo(() => getChainById(chainId), [chainId, getChainById]);

    const allAvailableTokens = useMemo(
        () => getTokensForChain(chainId),
        [chainId, getTokensForChain],
    );

    const sendableTokens = useMemo(() => {
        if (!filterBySendable || !otherChainId) return [];
        return getSendableTokens(chainId, otherChainId);
    }, [filterBySendable, otherChainId, chainId, getSendableTokens]);

    const availableTokens = useMemo(() => {
        if (!filterBySendable || !sendableTokens.length) return allAvailableTokens;
        return allAvailableTokens.filter((token) => sendableTokens.includes(token.symbol));
    }, [allAvailableTokens, sendableTokens, filterBySendable]);

    const selectedToken = useMemo(
        () => availableTokens.find((t) => t.symbol === tokenSymbol),
        [availableTokens, tokenSymbol],
    );

    // Determine which chains to show in the dropdown
    const chainsForSelection = useMemo(() => {
        if (availableChains) return availableChains;
        return config.chains;
    }, [availableChains, config.chains]);

    const title: string = isFromChain ? "From Chain" : "To Chain";
    const assetTitle: string = isFromChain ? "Send Asset" : "Receive Asset";
    const chainSelectPlaceholder: string = isFromChain
        ? "Select source chain"
        : "Select destination chain";
    const tokenSelectPlaceholder: string = isFromChain
        ? "Select token to send"
        : "Select token to receive (optional)";

    return (
        <>
            {/* Chain Selection */}
            <div className="card bg-slate-700/50 shadow-xl text-base-content">
                <div className="card-body">
                    <div className="flex flex-row justify-between">
                        <h2 className="card-title">{title}</h2>
                        {chainData ? (
                            <Link
                                href={chainData.explorer_details.base_url}
                                target="_blank"
                                rel="noopener noreferrer"
                                className="tooltip"
                                data-tip="Chain Explorer"
                            >
                                <FaWpexplorer className="size-8" />
                            </Link>
                        ) : null}
                    </div>
                    {chainData ? (
                        <div className="flex items-center justify-center gap-2">
                            <Image
                                src={chainData.chain_logo || "/unknown.jpg"}
                                alt={chainData.name}
                                width={100}
                                height={100}
                                className={`rounded-full size-20 bg-slate-900 
                ${
                    isFromChain
                        ? "border-[3px] border-orange-500 shadow-xl shadow-orange-800/70"
                        : "border-[3px] border-teal-400 shadow-xl shadow-teal-700/70"
                }`}
                            />
                        </div>
                    ) : (
                        <div className="flex items-center  justify-center gap-2">
                            <Image
                                src={"/unknown.jpg"}
                                alt="No Chain Selected"
                                width={100}
                                height={100}
                                className="rounded-full bg-slate-900 size-20"
                            />
                        </div>
                    )}
                    <select
                        className="select select-bordered w-full"
                        value={chainId}
                        onChange={(e) => onChainChange(e.target.value)}
                        disabled={isPending || (requireOtherChain && !otherChainId)}
                    >
                        <option value="">{chainSelectPlaceholder}</option>
                        {chainsForSelection.map((chain) => (
                            <option key={chain.id} value={chain.id}>
                                {chain.name}
                            </option>
                        ))}
                    </select>
                </div>
                {/* Asset Selection */}
                <div className="card-body">
                    <h2 className="card-title">{assetTitle}</h2>
                    <select
                        className="select select-bordered w-full"
                        value={tokenSymbol}
                        onChange={(e) => onTokenChange(e.target.value)}
                        disabled={isPending || !chainId}
                    >
                        <option value="">{tokenSelectPlaceholder}</option>
                        {availableTokens.map((token) => (
                            <option key={token.denom} value={token.symbol}>
                                {token.symbol} - {token.name}
                            </option>
                        ))}
                    </select>
                </div>
            </div>
        </>
    );
}
