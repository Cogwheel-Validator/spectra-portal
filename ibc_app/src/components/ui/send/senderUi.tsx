"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useCallback, useEffect, useMemo, useState, useTransition } from "react";
import type { ClientChain, ClientConfig, ClientToken } from "@/components/modules/tomlTypes";
import ChainSection from "@/components/ui/send/chainSection";
import { type SolveRouteResponse, solverClient } from "@/lib/solver-client";

interface SendUIOptimizedProps {
    // Pass config as prop from Server Component (pre-loaded at build time)
    config: ClientConfig;
    sendChain?: string;
    receiveChain?: string;
    sendToken?: string;
    receiveToken?: string;
    amount?: string;
}

/**
 * Optimized SendUI that separates concerns:
 * - Static config data (chains, tokens, metadata) passed as prop
 * - Dynamic solver calls only when needed (route calculation)
 */
export default function SendUIOptimized({
    config,
    sendChain: initialSendChain = "",
    receiveChain: initialReceiveChain = "",
    sendToken: initialSendToken = "",
    receiveToken: initialReceiveToken = "",
    amount: initialAmount = "",
}: SendUIOptimizedProps) {
    const router = useRouter();
    const searchParams = useSearchParams();
    const [isPending, startTransition] = useTransition();

    const [sendChain, setSendChain] = useState(initialSendChain);
    const [receiveChain, setReceiveChain] = useState(initialReceiveChain);
    const [sendToken, setSendToken] = useState(initialSendToken);
    const [receiveToken, setReceiveToken] = useState(initialReceiveToken);
    const [amount, setAmount] = useState(initialAmount);

    // Solver state - only called when we need routing info
    const [routeInfo, setRouteInfo] = useState<SolveRouteResponse | null>(null);
    const [routeLoading, setRouteLoading] = useState(false);
    const [routeError, setRouteError] = useState<string | null>(null);

    const getChainById = useCallback(
        (chainId: string): ClientChain | undefined => {
            return config.chains.find((chain) => chain.id === chainId);
        },
        [config],
    );

    const getTokensForChain = useCallback(
        (chainId: string): ClientToken[] => {
            const chain = getChainById(chainId);
            if (!chain) return [];
            return [...chain.native_tokens, ...chain.ibc_tokens];
        },
        [getChainById],
    );

    const getConnectedChains = useCallback(
        (chainId: string) => {
            const chain = getChainById(chainId);
            if (!chain) return [];
            return chain.connected_chains;
        },
        [getChainById],
    );

    // Memoized derived data for route calculation and available chains
    const sendChainData = useMemo(() => getChainById(sendChain), [sendChain, getChainById]);
    const receiveChainData = useMemo(
        () => getChainById(receiveChain),
        [receiveChain, getChainById],
    );

    // Convert ConnectedChainInfo[] to ClientChain[] by looking up full chain data
    const availableReceiveChains = useMemo(() => {
        const connectedChains = getConnectedChains(sendChain);
        return connectedChains
            .map((connectedChain) => getChainById(connectedChain.id))
            .filter((chain): chain is ClientChain => chain !== undefined);
    }, [sendChain, getConnectedChains, getChainById]);

    // Get selected token for route calculation
    const availableSendTokens = useMemo(
        () => getTokensForChain(sendChain),
        [sendChain, getTokensForChain],
    );
    const selectedSendToken = useMemo(
        () => availableSendTokens.find((t) => t.symbol === sendToken),
        [availableSendTokens, sendToken],
    );

    // Call solver RPC when we have all required info
    useEffect(() => {
        const fetchRoute = async () => {
            if (!sendChain || !receiveChain || !sendToken || !amount || !selectedSendToken) {
                setRouteInfo(null);
                return;
            }

            // Validate amount is positive
            const amountNum = parseFloat(amount);
            if (Number.isNaN(amountNum) || amountNum <= 0) {
                setRouteInfo(null);
                return;
            }

            setRouteLoading(true);
            setRouteError(null);

            try {
                const route = await solverClient.solveRoute({
                    from_chain_id: sendChain,
                    to_chain_id: receiveChain,
                    token_in: selectedSendToken.denom,
                    token_out: receiveToken ? selectedSendToken.denom : undefined, // Optional
                    amount_in: amount,
                });

                setRouteInfo(route);
            } catch (error) {
                console.error("Failed to fetch route:", error);
                setRouteError(error instanceof Error ? error.message : "Failed to calculate route");
                setRouteInfo(null);
            } finally {
                setRouteLoading(false);
            }
        };

        fetchRoute();
    }, [sendChain, receiveChain, sendToken, receiveToken, amount, selectedSendToken]);

    // URL update logic
    const updateURL = (
        updates: Partial<{
            from_chain: string;
            to_chain: string;
            send_asset: string;
            receive_asset: string;
            amount: string;
        }>,
    ) => {
        startTransition(() => {
            const params = new URLSearchParams(searchParams.toString());

            Object.entries(updates).forEach(([key, value]) => {
                if (value !== undefined) {
                    if (value) params.set(key, value);
                    else params.delete(key);
                }
            });

            router.push(`/transfer?${params.toString()}`, { scroll: false });
        });
    };

    const handleSendChainChange = (chainId: string) => {
        setSendChain(chainId);
        setSendToken("");
        setReceiveChain("");
        setReceiveToken("");
        setRouteInfo(null);
        updateURL({ from_chain: chainId, send_asset: "", to_chain: "", receive_asset: "" });
    };

    const handleReceiveChainChange = (chainId: string) => {
        setReceiveChain(chainId);
        setReceiveToken("");
        setRouteInfo(null);
        updateURL({ to_chain: chainId, receive_asset: "" });
    };

    const handleSendTokenChange = (tokenSymbol: string) => {
        setSendToken(tokenSymbol);
        setRouteInfo(null);
        updateURL({ send_asset: tokenSymbol });
    };

    const handleReceiveTokenChange = (tokenSymbol: string) => {
        setReceiveToken(tokenSymbol);
        setRouteInfo(null);
        updateURL({ receive_asset: tokenSymbol });
    };

    const handleAmountChange = (value: string) => {
        setAmount(value);
        updateURL({ amount: value });
    };

    const handleSignTransaction = () => {
        if (!routeInfo) {
            console.error("No route information available");
            return;
        }

        // TODO: Implement transaction signing with route info
        console.log("Sign transaction with route:", {
            sendChain,
            receiveChain,
            sendToken,
            receiveToken,
            amount,
            routeInfo,
        });
    };

    return (
        <div className="max-w-4xl mx-auto py-20 space-y-6">
            <h1 className="text-3xl font-bold text-center">Transfer Assets</h1>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                {/* From Chain Section with chain + token selection */}
                <ChainSection
                    config={config}
                    isPending={isPending}
                    chainId={sendChain}
                    tokenSymbol={sendToken}
                    onChainChange={handleSendChainChange}
                    onTokenChange={handleSendTokenChange}
                    isFromChain={true}
                    otherChainId={receiveChain}
                />

                {/* To Chain Section with chain + token selection */}
                <ChainSection
                    config={config}
                    isPending={isPending}
                    chainId={receiveChain}
                    tokenSymbol={receiveToken}
                    onChainChange={handleReceiveChainChange}
                    onTokenChange={handleReceiveTokenChange}
                    isFromChain={false}
                    otherChainId={sendChain}
                    availableChains={availableReceiveChains}
                />
            </div>

            {/* Amount Input */}
            <div className="card bg-base-200 shadow-xl">
                <div className="card-body">
                    <h2 className="card-title">Amount</h2>
                    <input
                        type="number"
                        placeholder="Enter amount"
                        className="input input-bordered w-full"
                        value={amount}
                        onChange={(e) => handleAmountChange(e.target.value)}
                        disabled={isPending || !sendToken}
                        step="any"
                        min="0"
                    />
                </div>
            </div>

            {/* Route Information from Solver */}
            {routeLoading && (
                <div className="card bg-base-300 shadow-xl">
                    <div className="card-body">
                        <h2 className="card-title">Calculating Route...</h2>
                        <div className="flex items-center justify-center py-4">
                            <span className="loading loading-spinner loading-lg"></span>
                        </div>
                    </div>
                </div>
            )}

            {routeError && (
                <div className="alert alert-error">
                    <svg
                        xmlns="http://www.w3.org/2000/svg"
                        className="stroke-current shrink-0 h-6 w-6"
                        fill="none"
                        viewBox="0 0 24 24"
                    >
                        <path
                            strokeLinecap="round"
                            strokeLinejoin="round"
                            strokeWidth="2"
                            d="M10 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2m7-2a9 9 0 11-18 0 9 9 0 0118 0z"
                        />
                    </svg>
                    <span>Route Error: {routeError}</span>
                </div>
            )}

            {/* Transaction Summary with Route Info */}
            {routeInfo && sendChain && receiveChain && sendToken && amount && (
                <div className="card bg-primary text-primary-content shadow-xl">
                    <div className="card-body">
                        <h2 className="card-title">Transaction Summary</h2>
                        <div className="space-y-3">
                            <p>
                                <strong>From:</strong> {sendChainData?.name} ({sendToken})
                            </p>
                            <p>
                                <strong>To:</strong> {receiveChainData?.name} (
                                {receiveToken || "Auto"})
                            </p>
                            <p>
                                <strong>Amount:</strong> {amount} {sendToken}
                            </p>

                            <div className="divider"></div>

                            <div className="bg-primary-content/20 p-4 rounded-lg">
                                <h3 className="font-bold mb-2">Route Information</h3>
                                <p>
                                    <strong>Route Type:</strong> {routeInfo.route_type}
                                </p>
                                <p>
                                    <strong>Steps:</strong> {routeInfo.steps.length}
                                </p>
                                {routeInfo.supports_pfm && (
                                    <p className="text-success">
                                        ✓ Supports PFM (Single Transaction)
                                    </p>
                                )}
                                {routeInfo.estimated_time_seconds && (
                                    <p>
                                        <strong>Est. Time:</strong> ~
                                        {routeInfo.estimated_time_seconds}s
                                    </p>
                                )}
                                {routeInfo.total_fees_estimate && (
                                    <p>
                                        <strong>Est. Fees:</strong> {routeInfo.total_fees_estimate}
                                    </p>
                                )}

                                {routeInfo.broker_swap && (
                                    <div className="mt-2">
                                        <p className="text-warning">
                                            ⚠ Requires DEX swap on{" "}
                                            {routeInfo.broker_swap.broker_chain_id}
                                        </p>
                                        <p className="text-sm">
                                            Output: ~{routeInfo.broker_swap.estimated_output}
                                        </p>
                                    </div>
                                )}
                            </div>
                        </div>
                        <div className="card-actions justify-end mt-4">
                            <button
                                type="button"
                                className="btn btn-secondary"
                                onClick={handleSignTransaction}
                                disabled={isPending || !routeInfo}
                            >
                                {isPending ? "Processing..." : "Sign Transaction"}
                            </button>
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
}
