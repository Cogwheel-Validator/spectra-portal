"use client";

import { AlertCircle, ArrowDown, ArrowRight, CheckCircle2, Loader2 } from "lucide-react";
import { useRouter, useSearchParams } from "next/navigation";
import { useCallback, useEffect, useMemo, useState, useTransition } from "react";
import type { ClientChain, ClientConfig, ClientToken } from "@/components/modules/tomlTypes";
import AmountInput from "@/components/ui/send/amountInput";
import AssetDropdown from "@/components/ui/send/assetDropdown";
import ChainDropdown from "@/components/ui/send/chainDropdown";
import TransferModeToggle from "@/components/ui/send/transferModeToggle";
import WalletConnect from "@/components/ui/wallet/walletConnect";
import { type TransferMode, useTransfer } from "@/context/transferContext";
import { useWallet } from "@/context/walletContext";
import { useDebouncedCallback } from "@/hooks/useDebounce";
import {
    getRouteStepCount,
    routeSupportsPfm,
    usePathfinderQuery,
} from "@/hooks/usePathfinderQuery";
import { useGetAddressBalance } from "@/lib/apiQueries/fetchApiData";

interface SendUIProps {
    config: ClientConfig;
    sendChain?: string;
    receiveChain?: string;
    sendToken?: string;
    receiveToken?: string;
    amount?: string;
}

export default function SendUI({
    config,
    sendChain: initialSendChain = "",
    receiveChain: initialReceiveChain = "",
    sendToken: initialSendToken = "",
    receiveToken: initialReceiveToken = "",
    amount: initialAmount = "",
}: SendUIProps) {
    const router = useRouter();
    const searchParams = useSearchParams();
    const [isPending, startTransition] = useTransition();
    const { isConnectedToChain, getAddress } = useWallet();
    const transfer = useTransfer();

    // Local state for form inputs
    const [sendChain, setSendChain] = useState(initialSendChain);
    const [receiveChain, setReceiveChain] = useState(initialReceiveChain);
    const [sendToken, setSendToken] = useState(initialSendToken);
    const [receiveToken, setReceiveToken] = useState(initialReceiveToken);
    const [amount, setAmount] = useState(initialAmount);
    const [mode, setMode] = useState<TransferMode>("manual");

    // Helper functions
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
        (chainId: string): ClientChain[] => {
            const chain = getChainById(chainId);
            if (!chain) return [];
            return chain.connected_chains
                .map((cc) => getChainById(cc.id))
                .filter((c): c is ClientChain => c !== undefined);
        },
        [getChainById],
    );

    const getSendableTokens = useCallback(
        (fromChainId: string, toChainId: string): string[] => {
            const chain = getChainById(fromChainId);
            if (!chain) return [];
            const connected = chain.connected_chains.find((cc) => cc.id === toChainId);
            return connected?.sendable_tokens ?? [];
        },
        [getChainById],
    );

    // Derived data
    const sendChainData = useMemo(() => getChainById(sendChain), [sendChain, getChainById]);
    const receiveChainData = useMemo(
        () => getChainById(receiveChain),
        [receiveChain, getChainById],
    );

    // Find broker chains (DEX chains like Osmosis that can swap and route)
    const brokerChains = useMemo(() => {
        return config.chains.filter((c) => c.is_dex);
    }, [config.chains]);

    // Get all chains reachable from source chain (directly or via broker)
    const availableReceiveChains = useMemo(() => {
        if (!sendChain) return [];

        const directlyConnected = getConnectedChains(sendChain);
        const directIds = new Set(directlyConnected.map((c) => c.id));

        // Add chains reachable via broker chains
        const viabroker: ClientChain[] = [];
        for (const broker of brokerChains) {
            // Check if source chain can reach the broker
            const sourceChain = getChainById(sendChain);
            const canReachBroker = sourceChain?.connected_chains.some((cc) => cc.id === broker.id);

            if (canReachBroker || sendChain === broker.id) {
                // Add all chains connected to the broker that aren't already in the list
                for (const connected of broker.connected_chains) {
                    if (!directIds.has(connected.id) && connected.id !== sendChain) {
                        const chain = getChainById(connected.id);
                        if (chain && !viabroker.some((c) => c.id === chain.id)) {
                            viabroker.push(chain);
                        }
                    }
                }
            }
        }

        return [...directlyConnected, ...viabroker];
    }, [sendChain, getConnectedChains, brokerChains, getChainById]);

    const sendableTokenSymbols = useMemo(() => {
        if (!sendChain || !receiveChain) return [];
        return getSendableTokens(sendChain, receiveChain);
    }, [sendChain, receiveChain, getSendableTokens]);

    // Base tokens available on chains (without balance sorting)
    const baseSendTokens = useMemo(() => {
        return getTokensForChain(sendChain);
    }, [sendChain, getTokensForChain]);

    const availableReceiveTokens = useMemo(() => {
        return getTokensForChain(receiveChain);
    }, [receiveChain, getTokensForChain]);

    // Wallet addresses
    const senderAddress = useMemo(() => getAddress(sendChain) ?? "", [sendChain, getAddress]);
    const receiverAddress = useMemo(
        () => getAddress(receiveChain) ?? "",
        [receiveChain, getAddress],
    );

    // Fetch sender's balance on the source chain
    const { data: senderBalance, isLoading: balanceLoading } = useGetAddressBalance(
        sendChain,
        senderAddress,
    );

    // All tokens available on source chain, sorted by user's balance
    const availableSendTokens = useMemo(() => {
        // If we have balance data, sort by balance (highest first)
        if (senderBalance?.balances) {
            const balanceMap = new Map(
                senderBalance.balances.map((b) => [b.denom, BigInt(b.amount)]),
            );

            return [...baseSendTokens].sort((a, b) => {
                const balanceA = balanceMap.get(a.denom) ?? BigInt(0);
                const balanceB = balanceMap.get(b.denom) ?? BigInt(0);
                if (balanceB > balanceA) return 1;
                if (balanceB < balanceA) return -1;
                return 0;
            });
        }

        return baseSendTokens;
    }, [baseSendTokens, senderBalance]);

    const selectedSendToken = useMemo(
        () => availableSendTokens.find((t) => t.symbol === sendToken) ?? null,
        [availableSendTokens, sendToken],
    );

    const selectedReceiveToken = useMemo(
        () => availableReceiveTokens.find((t) => t.symbol === receiveToken) ?? null,
        [availableReceiveTokens, receiveToken],
    );

    // Check if user has sufficient balance for the selected token
    const tokenBalance = useMemo(() => {
        if (!senderBalance || !selectedSendToken) return null;
        const balance = senderBalance.balances.find((b) => b.denom === selectedSendToken.denom);
        return balance ? balance.amount : "0";
    }, [senderBalance, selectedSendToken]);

    // Format balance for display
    const formattedBalance = useMemo(() => {
        if (!tokenBalance || !selectedSendToken) return null;
        const decimals = selectedSendToken.decimals ?? 6;
        const value = Number(tokenBalance) / 10 ** decimals;
        return value.toLocaleString(undefined, {
            minimumFractionDigits: 0,
            // even if the token has more decimals than 6, we only want to display 6 decimal places
            maximumFractionDigits: decimals > 6 ? 6 : decimals,
        });
    }, [tokenBalance, selectedSendToken]);

    // Check if user has enough balance
    const insufficientBalance = useMemo(() => {
        if (!tokenBalance || !amount || !selectedSendToken) return false;
        const decimals = selectedSendToken.decimals ?? 6;
        const amountInSmallestUnit = BigInt(Math.floor(Number(amount) * 10 ** decimals));
        const balanceInSmallestUnit = BigInt(tokenBalance);
        return amountInSmallestUnit > balanceInSmallestUnit;
    }, [tokenBalance, amount, selectedSendToken]);

    // Convert amount to base units (with decimals) for pathfinder
    const amountInBaseUnits = useMemo(() => {
        if (!amount || !selectedSendToken) return "";
        const decimals = selectedSendToken.decimals ?? 6;
        try {
            // Convert from human-readable format (e.g., "1.5") to base units
            // Using BigInt to handle tokens with high decimals (like EVM tokens with 18)
            const parts = amount.split(".");
            const integerPart = parts[0] || "0";
            const decimalPart = (parts[1] || "").padEnd(decimals, "0").slice(0, decimals);
            
            // Combine to get the full amount in base units
            const fullAmount = integerPart + decimalPart;
            // Remove leading zeros but keep at least one digit
            return BigInt(fullAmount).toString();
        } catch {
            return "";
        }
    }, [amount, selectedSendToken]);

    // Pathfinder query with debounce
    const pathfinderParams = useMemo(() => {
        if (!sendChain || !receiveChain || !sendToken || !senderAddress || !receiverAddress) {
            return null;
        }
        return {
            chainFrom: sendChain,
            tokenFromDenom: selectedSendToken?.denom ?? "",
            amountIn: amountInBaseUnits,
            chainTo: receiveChain,
            tokenToDenom: selectedReceiveToken?.denom ?? "",
            senderAddress,
            receiverAddress,
            singleRoute: true,
            slippageBps: 100,
        };
    }, [
        sendChain,
        receiveChain,
        sendToken,
        amountInBaseUnits,
        senderAddress,
        receiverAddress,
        selectedSendToken,
        selectedReceiveToken,
    ]);

    const isReadyToQuery = !!(
        sendChain &&
        receiveChain &&
        sendToken &&
        amount &&
        Number.parseFloat(amount) > 0 &&
        senderAddress &&
        receiverAddress
    );

    const {
        data: pathfinderResponse,
        isLoading: routeLoading,
        isPending: routePending,
        error: routeError,
    } = usePathfinderQuery(pathfinderParams, isReadyToQuery, 2000);

    // Route analysis
    const supportsPfm = useMemo(() => {
        return pathfinderResponse ? routeSupportsPfm(pathfinderResponse) : false;
    }, [pathfinderResponse]);

    const supportsWasm = useMemo(() => {
        if (!pathfinderResponse?.success) return false;
        if (pathfinderResponse.route.case === "brokerSwap") {
            return pathfinderResponse.route.value.execution?.usesWasm ?? false;
        }
        return false;
    }, [pathfinderResponse]);

    const stepCount = useMemo(() => {
        return pathfinderResponse ? getRouteStepCount(pathfinderResponse, mode) : 0;
    }, [pathfinderResponse, mode]);

    // Extract stable setters from transfer context to avoid dependency issues
    const {
        setPathfinderResponse,
        setFromChain,
        setToChain,
        setFromToken,
        setToToken,
        setAmount: setTransferAmount,
        setSenderAddress,
        setReceiverAddress,
        setMode: setTransferMode,
        setSlippage,
        state: transferState,
    } = transfer;

    // Slippage from context
    const slippageBps = transferState.slippageBps;

    // Determine if this is a direct route (no PFM or WASM available)
    const isDirectRoute = useMemo(() => {
        if (!pathfinderResponse?.success) return false;
        return pathfinderResponse.route.case === "direct";
    }, [pathfinderResponse]);

    // Update transfer context when pathfinder response changes
    useEffect(() => {
        setPathfinderResponse(pathfinderResponse);
    }, [pathfinderResponse, setPathfinderResponse]);

    // Sync transfer context with local state
    useEffect(() => {
        setFromChain(sendChain);
        setToChain(receiveChain);
        setFromToken(selectedSendToken);
        setToToken(selectedReceiveToken);
        setTransferAmount(amount);
        setSenderAddress(senderAddress);
        setReceiverAddress(receiverAddress);
        setTransferMode(mode);
    }, [
        sendChain,
        receiveChain,
        selectedSendToken,
        selectedReceiveToken,
        amount,
        senderAddress,
        receiverAddress,
        mode,
        setFromChain,
        setToChain,
        setFromToken,
        setToToken,
        setTransferAmount,
        setSenderAddress,
        setReceiverAddress,
        setTransferMode,
    ]);

    // URL update logic
    const updateURL = useCallback(
        (
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
        },
        [router, searchParams],
    );

    // Event handlers
    const handleSendChainChange = useCallback(
        (chainId: string) => {
            setSendChain(chainId);
            setSendToken("");
            setReceiveChain("");
            setReceiveToken("");
            setAmount("");
            updateURL({
                from_chain: chainId,
                send_asset: "",
                to_chain: "",
                receive_asset: "",
                amount: "",
            });
        },
        [updateURL],
    );

    const handleReceiveChainChange = useCallback(
        (chainId: string) => {
            setReceiveChain(chainId);
            setReceiveToken("");
            updateURL({ to_chain: chainId, receive_asset: "" });
        },
        [updateURL],
    );

    const handleSendTokenChange = useCallback(
        (tokenSymbol: string) => {
            setSendToken(tokenSymbol);
            updateURL({ send_asset: tokenSymbol });
        },
        [updateURL],
    );

    const handleReceiveTokenChange = useCallback(
        (tokenSymbol: string) => {
            setReceiveToken(tokenSymbol);
            updateURL({ receive_asset: tokenSymbol });
        },
        [updateURL],
    );

    // Debounced URL update for amount using callback hook
    const debouncedUpdateURL = useDebouncedCallback(
        (value: string) => {
            updateURL({ amount: value });
        },
        2000,
    );

    const handleAmountChange = useCallback(
        (value: string) => {
            setAmount(value);
            debouncedUpdateURL(value);
        },
        [debouncedUpdateURL],
    );

    // Validation
    const isWalletReady = useMemo(() => {
        if (!sendChain || !receiveChain) return false;
        return isConnectedToChain(sendChain) && isConnectedToChain(receiveChain);
    }, [sendChain, receiveChain, isConnectedToChain]);

    const canSubmit = useMemo(() => {
        return (
            isWalletReady &&
            pathfinderResponse?.success === true &&
            !routeLoading &&
            !routePending &&
            Number.parseFloat(amount) > 0 &&
            !insufficientBalance &&
            !balanceLoading
        );
    }, [
        isWalletReady,
        pathfinderResponse,
        routeLoading,
        routePending,
        amount,
        insufficientBalance,
        balanceLoading,
    ]);

    // Required chains for wallet connection
    const requiredChains = useMemo(() => {
        const chains: ClientChain[] = [];
        if (sendChainData) chains.push(sendChainData);
        if (receiveChainData && receiveChainData.id !== sendChainData?.id) {
            chains.push(receiveChainData);
        }
        return chains;
    }, [sendChainData, receiveChainData]);

    // Handle transfer submission
    const { startPreparing } = transfer;
    const handleSubmit = useCallback(() => {
        if (!canSubmit || !pathfinderResponse) return;
        // Trigger the transfer flow - TaskProvider will handle execution
        startPreparing();
    }, [canSubmit, pathfinderResponse, startPreparing]);

    // Extract route info for display
    const routeInfo = useMemo(() => {
        if (!pathfinderResponse?.success) return null;

        let routeType = "";
        let expectedOutput = "";
        let priceImpact = "";

        switch (pathfinderResponse.route.case) {
            case "direct":
                routeType = "Direct Transfer";
                expectedOutput = pathfinderResponse.route.value.transfer?.amount ?? "0";
                break;
            case "indirect": {
                routeType = "Multi-hop Transfer";
                const legs = pathfinderResponse.route.value.legs;
                expectedOutput = legs[legs.length - 1]?.amount ?? "0";
                break;
            }
            case "brokerSwap":
                routeType = "Swap & Transfer";
                expectedOutput = pathfinderResponse.route.value.swap?.amountOut ?? "0";
                priceImpact = pathfinderResponse.route.value.swap?.priceImpact ?? "0";
                break;
        }

        return { routeType, expectedOutput, priceImpact, stepCount };
    }, [pathfinderResponse, stepCount]);

    return (
        <div className="space-y-4 lg:space-y-5">
            {/* Header */}
            <div className="flex justify-between items-center">
                <h1 className="text-2xl lg:text-3xl font-bold text-white">Transfer Assets</h1>
                <WalletConnect requiredChains={requiredChains} availableChains={config.chains} />
            </div>

            {/* Main Transfer Section - Horizontal on PC, Vertical on Mobile */}
            <div className="flex flex-col lg:flex-row lg:items-stretch gap-4 lg:gap-6">
                {/* From Section */}
                <div className="flex-1 bg-slate-800/30 rounded-xl p-4 lg:p-5 border border-slate-700/50 space-y-3">
                    <h2 className="text-base lg:text-lg font-semibold text-white flex items-center gap-2">
                        <span className="w-5 h-5 lg:w-6 lg:h-6 rounded-full bg-orange-500 flex items-center justify-center text-xs lg:text-sm font-bold">
                            1
                        </span>
                        From
                    </h2>

                    <div className="space-y-3">
                        <ChainDropdown
                            chains={config.chains}
                            selectedChainId={sendChain}
                            onSelect={handleSendChainChange}
                            placeholder="Select source chain"
                            disabled={isPending}
                            label="Chain"
                            variant="from"
                        />

                        <AssetDropdown
                            tokens={availableSendTokens}
                            selectedSymbol={sendToken}
                            onSelect={handleSendTokenChange}
                            placeholder="Select asset to send"
                            disabled={isPending || !sendChain}
                            label="Asset"
                        />

                        {/* Amount Input - under From section */}
                        <div className="space-y-2">
                            <AmountInput
                                value={amount}
                                onChange={handleAmountChange}
                                token={selectedSendToken}
                                disabled={isPending || !receiveChain || !receiveToken}
                                isLoading={routeLoading || routePending}
                                label="Amount to Send"
                            />

                            {/* Balance Display */}
                            {selectedSendToken && senderAddress && (
                                <div className="flex justify-between text-xs">
                                    <span className="text-slate-400">Available:</span>
                                    {balanceLoading ? (
                                        <span className="text-slate-400">Loading...</span>
                                    ) : formattedBalance !== null ? (
                                        <span
                                            className={`font-medium ${insufficientBalance ? "text-red-400" : "text-slate-300"}`}
                                        >
                                            {formattedBalance} {selectedSendToken.symbol}
                                        </span>
                                    ) : (
                                        <span className="text-slate-400">
                                            0 {selectedSendToken.symbol}
                                        </span>
                                    )}
                                </div>
                            )}

                            {/* Insufficient Balance Warning */}
                            {insufficientBalance && (
                                <div className="flex items-center gap-2 text-red-400 text-xs">
                                    <AlertCircle className="w-3 h-3" />
                                    <span>Insufficient balance</span>
                                </div>
                            )}
                        </div>
                    </div>
                </div>

                {/* Arrow Separator - Down on mobile, Right on PC */}
                <div className="flex justify-center items-center lg:self-center">
                    <div className="p-2 lg:p-3 bg-slate-700/50 rounded-full">
                        <ArrowDown className="w-5 h-5 lg:hidden text-slate-400" />
                        <ArrowRight className="w-5 h-5 hidden lg:block text-slate-400" />
                    </div>
                </div>

                {/* To Section */}
                <div className="flex-1 bg-slate-800/30 rounded-xl p-4 lg:p-5 border border-slate-700/50 space-y-3">
                    <h2 className="text-base lg:text-lg font-semibold text-white flex items-center gap-2">
                        <span className="w-5 h-5 lg:w-6 lg:h-6 rounded-full bg-teal-500 flex items-center justify-center text-xs lg:text-sm font-bold">
                            2
                        </span>
                        To
                    </h2>

                    <div className="space-y-3">
                        <ChainDropdown
                            chains={availableReceiveChains}
                            selectedChainId={receiveChain}
                            onSelect={handleReceiveChainChange}
                            placeholder="Select destination chain"
                            disabled={isPending || !sendChain}
                            label="Chain"
                            variant="to"
                        />

                        <AssetDropdown
                            tokens={availableReceiveTokens}
                            selectedSymbol={receiveToken}
                            onSelect={handleReceiveTokenChange}
                            placeholder="Select asset to receive"
                            disabled={isPending || !receiveChain}
                            label="Asset (optional)"
                        />
                    </div>
                </div>
            </div>

            {/* Route Info + Mode + Submit - Compact on PC */}
            <div className="space-y-3 lg:space-y-4">
                {/* Transfer Mode Toggle - Always show when pathfinder response is available */}
                {pathfinderResponse?.success && (
                    <div className="bg-slate-800/30 rounded-xl p-4 lg:p-5 border border-slate-700/50">
                        <TransferModeToggle
                            mode={mode}
                            onModeChange={setMode}
                            slippageBps={slippageBps}
                            onSlippageChange={setSlippage}
                            supportsPfm={supportsPfm}
                            supportsWasm={supportsWasm}
                            isDirectRoute={isDirectRoute}
                            disabled={isPending}
                        />
                    </div>
                )}

                {/* Route Information */}
                {(routeLoading || routePending) && amount && Number.parseFloat(amount) > 0 && (
                    <div className="bg-slate-800/30 rounded-xl p-4 border border-slate-700/50">
                        <div className="flex items-center gap-3">
                            <Loader2 className="w-4 h-4 text-teal-400 animate-spin" />
                            <span className="text-slate-300 text-sm">
                                Calculating best route...
                            </span>
                        </div>
                    </div>
                )}

                {routeError && (
                    <div className="bg-red-500/10 rounded-xl p-4 border border-red-500/30">
                        <div className="flex items-center gap-3">
                            <AlertCircle className="w-4 h-4 text-red-400" />
                            <span className="text-red-300 text-sm">{routeError}</span>
                        </div>
                    </div>
                )}

                {routeInfo && !routeLoading && !routePending && (
                    <div className="bg-linear-to-r from-teal-500/10 to-emerald-500/10 rounded-xl p-4 border border-teal-500/30">
                        <div className="flex items-center justify-between mb-3">
                            <h3 className="text-base font-semibold text-white">Route Summary</h3>
                            <CheckCircle2 className="w-4 h-4 text-teal-400" />
                        </div>

                        <div className="grid grid-cols-2 lg:grid-cols-4 gap-3 text-sm">
                            <div>
                                <span className="text-slate-400 text-xs">Route Type</span>
                                <p className="text-white font-medium">{routeInfo.routeType}</p>
                            </div>
                            <div>
                                <span className="text-slate-400 text-xs">Steps</span>
                                <p className="text-white font-medium">
                                    {routeInfo.stepCount} tx{routeInfo.stepCount !== 1 ? "s" : ""}
                                </p>
                            </div>
                            {routeInfo.priceImpact &&
                                Number.parseFloat(routeInfo.priceImpact) !== 0 && (
                                    <div>
                                        <span className="text-slate-400 text-xs">Price Impact</span>
                                        <p
                                            className={`font-medium ${Number.parseFloat(routeInfo.priceImpact) < -0.01 ? "text-yellow-400" : "text-white"}`}
                                        >
                                            {(
                                                Number.parseFloat(routeInfo.priceImpact) * 100
                                            ).toFixed(2)}
                                            %
                                        </p>
                                    </div>
                                )}
                            {selectedReceiveToken && (
                                <div>
                                    <span className="text-slate-400 text-xs">Expected Output</span>
                                    <p className="text-white font-medium">
                                        ~
                                        {(
                                            Number.parseFloat(routeInfo.expectedOutput) /
                                            10 ** selectedReceiveToken.decimals
                                        ).toLocaleString()}{" "}
                                        {selectedReceiveToken.symbol}
                                    </p>
                                </div>
                            )}
                        </div>
                    </div>
                )}

                {/* Submit Button */}
                <button
                    type="button"
                    onClick={handleSubmit}
                    disabled={!canSubmit || isPending}
                    className={`
                        w-full py-3 lg:py-4 px-6 rounded-xl font-bold text-base lg:text-lg
                        transition-all duration-300 flex items-center justify-center gap-3
                        ${
                            canSubmit
                                ? "bg-linear-to-r from-teal-500 to-emerald-500 text-white hover:from-teal-400 hover:to-emerald-400 shadow-lg shadow-teal-500/25"
                                : "bg-slate-700 text-slate-400 cursor-not-allowed"
                        }
                    `}
                >
                    {!isWalletReady ? (
                        "Connect Wallet to Both Chains"
                    ) : !pathfinderResponse?.success ? (
                        "Enter Transfer Details"
                    ) : routeLoading || routePending ? (
                        <>
                            <Loader2 className="w-5 h-5 animate-spin" />
                            Computing...
                        </>
                    ) : (
                        <>
                            {mode === "smart" ? "Smart Transfer" : "Manual Transfer"}
                            <ArrowRight className="w-5 h-5" />
                        </>
                    )}
                </button>
            </div>
        </div>
    );
}
