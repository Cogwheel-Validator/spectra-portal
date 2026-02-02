"use client";

import { ArrowDown, ArrowRight } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import type { ClientChain, ClientConfig, ClientToken } from "@/components/modules/tomlTypes";
import FromSection from "@/components/ui/send/fromSection";
import RouteDisplay from "@/components/ui/send/routeDisplay";
import ToSection from "@/components/ui/send/toSection";
import TransferButton from "@/components/ui/send/transferButton";
import TransferModeToggle from "@/components/ui/send/transferModeToggle";
import WalletConnect from "@/components/ui/wallet/walletConnect";
import { useTransfer } from "@/context/transferContext";
import { useWallet } from "@/context/walletContext";
import { useBalanceValidation } from "@/hooks/useBalanceValidation";
import { useDebouncedCallback } from "@/hooks/useDebounce";
import { usePathfinderQuery } from "@/hooks/usePathfinderQuery";
import { useRouteInfo } from "@/hooks/useRouteInfo";
import { useTransferFormState } from "@/hooks/useTransferFormState";
import { humanToBaseUnits } from "@/lib/utils";

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
    const { isConnectedToChain, getAddress } = useWallet();
    const transfer = useTransfer();

    // Form state management
    const {
        sendChain,
        receiveChain,
        sendToken,
        receiveToken,
        amount,
        mode,
        isPending,
        setMode,
        handleSendChainChange,
        handleReceiveChainChange,
        handleSendTokenChange,
        handleReceiveTokenChange,
        handleAmountChange,
    } = useTransferFormState({
        initialSendChain,
        initialReceiveChain,
        initialSendToken,
        initialReceiveToken,
        initialAmount,
    });

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
        const viaBroker: ClientChain[] = [];
        for (const broker of brokerChains) {
            const sourceChain = getChainById(sendChain);
            const canReachBroker = sourceChain?.connected_chains.some((cc) => cc.id === broker.id);

            if (canReachBroker || sendChain === broker.id) {
                for (const connected of broker.connected_chains) {
                    if (!directIds.has(connected.id) && connected.id !== sendChain) {
                        const chain = getChainById(connected.id);
                        if (chain && !viaBroker.some((c) => c.id === chain.id)) {
                            viaBroker.push(chain);
                        }
                    }
                }
            }
        }

        return [...directlyConnected, ...viaBroker];
    }, [sendChain, getConnectedChains, brokerChains, getChainById]);

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

    // Balance validation hook - get sender balance for sorting tokens
    const { senderBalance } = useBalanceValidation(sendChain, senderAddress, null, amount);

    // All tokens available on source chain, sorted by user's balance
    const availableSendTokens = useMemo(() => {
        if (senderBalance?.balances) {
            const balanceMap = new Map(
                senderBalance.balances.map((b: { denom: string; amount: string }) => [
                    b.denom,
                    BigInt(b.amount),
                ]),
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

    // Recompute balance validation with selected token
    const {
        formattedBalance: finalFormattedBalance,
        insufficientBalance: finalInsufficientBalance,
        balanceLoading: finalBalanceLoading,
    } = useBalanceValidation(sendChain, senderAddress, selectedSendToken, amount);

    // Convert amount to base units (with decimals) for pathfinder
    const amountInBaseUnits = useMemo(() => {
        return humanToBaseUnits(amount, selectedSendToken?.decimals ?? 6);
    }, [amount, selectedSendToken]);

    const slippageBps = transfer.state.slippageBps;

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
            smartRoute: false,
            slippageBps: slippageBps,
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
        slippageBps,
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
        isStale: routeIsStale,
        quoteAgeSeconds,
        refetchFresh,
    } = usePathfinderQuery(pathfinderParams, isReadyToQuery, {
        debounceMs: 2000,
        autoRefreshMs: 30000,
        staleAfterMs: 15000,
    });

    // Route information hook
    const { routeInfo, supportsPfm, supportsWasm, isDirectRoute, intermediateChainIds } =
        useRouteInfo(pathfinderResponse, mode);

    // Extract stable setters from transfer context
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
    } = transfer;

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

    // Required chains for wallet connection (including intermediate chains)
    const requiredChains = useMemo(() => {
        const chains: ClientChain[] = [];
        const addedIds = new Set<string>();

        if (sendChainData) {
            chains.push(sendChainData);
            addedIds.add(sendChainData.id);
        }

        for (const chainId of intermediateChainIds) {
            if (!addedIds.has(chainId)) {
                const chain = getChainById(chainId);
                if (chain) {
                    chains.push(chain);
                    addedIds.add(chainId);
                }
            }
        }

        if (receiveChainData && !addedIds.has(receiveChainData.id)) {
            chains.push(receiveChainData);
        }

        return chains;
    }, [sendChainData, receiveChainData, intermediateChainIds, getChainById]);

    // Validation
    const isWalletReady = useMemo(() => {
        if (!sendChain || !receiveChain) return false;
        return requiredChains.every((chain) => isConnectedToChain(chain.id));
    }, [sendChain, receiveChain, requiredChains, isConnectedToChain]);

    const canSubmit = useMemo(() => {
        return (
            isWalletReady &&
            pathfinderResponse?.success === true &&
            !routeLoading &&
            !routePending &&
            Number.parseFloat(amount) > 0 &&
            !finalInsufficientBalance &&
            !finalBalanceLoading
        );
    }, [
        isWalletReady,
        pathfinderResponse,
        routeLoading,
        routePending,
        amount,
        finalInsufficientBalance,
        finalBalanceLoading,
    ]);

    const { startPreparing } = transfer;
    const [isRefreshing, setIsRefreshing] = useState(false);

    const handleSubmit = useCallback(async () => {
        if (!canSubmit || !pathfinderResponse) return;

        // If quote is stale, refresh it first
        if (routeIsStale) {
            setIsRefreshing(true);
            try {
                const freshResponse = await refetchFresh();
                if (!freshResponse?.success) {
                    setIsRefreshing(false);
                    return;
                }
                setPathfinderResponse(freshResponse);
            } catch {
                setIsRefreshing(false);
                return;
            }
            setIsRefreshing(false);
        }

        startPreparing();
    }, [
        canSubmit,
        pathfinderResponse,
        routeIsStale,
        refetchFresh,
        setPathfinderResponse,
        startPreparing,
    ]);

    // Debounced URL update for amount
    const debouncedUpdateURL = useDebouncedCallback((_value: string) => {
        // This is handled inside useTransferFormState now
    }, 2000);

    const wrappedHandleAmountChange = useCallback(
        (value: string) => {
            handleAmountChange(value, debouncedUpdateURL);
        },
        [handleAmountChange, debouncedUpdateURL],
    );

    return (
        <div className="space-y-4 lg:space-y-5">
            {/* Header */}
            <div className="flex justify-between items-center">
                <h1 className="text-2xl lg:text-3xl font-bold text-white">Transfer Assets</h1>
                <WalletConnect requiredChains={requiredChains} availableChains={config.chains} />
            </div>

            {/* Main Transfer Section */}
            <div className="flex flex-col lg:flex-row lg:items-stretch gap-4 lg:gap-6">
                <FromSection
                    chains={config.chains}
                    availableSendTokens={availableSendTokens}
                    selectedSendToken={selectedSendToken}
                    sendChain={sendChain}
                    sendToken={sendToken}
                    amount={amount}
                    senderAddress={senderAddress}
                    receiveChain={receiveChain}
                    receiveToken={receiveToken}
                    formattedBalance={finalFormattedBalance}
                    insufficientBalance={finalInsufficientBalance}
                    balanceLoading={finalBalanceLoading}
                    routeLoading={routeLoading}
                    routePending={routePending}
                    isPending={isPending}
                    onSendChainChange={handleSendChainChange}
                    onSendTokenChange={handleSendTokenChange}
                    onAmountChange={wrappedHandleAmountChange}
                />

                {/* Arrow Separator */}
                <div className="flex justify-center items-center lg:self-center">
                    <div className="p-2 lg:p-3 bg-slate-700/50 rounded-full">
                        <ArrowDown className="w-5 h-5 lg:hidden text-slate-400" />
                        <ArrowRight className="w-5 h-5 hidden lg:block text-slate-400" />
                    </div>
                </div>

                <ToSection
                    availableReceiveChains={availableReceiveChains}
                    availableReceiveTokens={availableReceiveTokens}
                    receiveChain={receiveChain}
                    receiveToken={receiveToken}
                    sendChain={sendChain}
                    isPending={isPending}
                    onReceiveChainChange={handleReceiveChainChange}
                    onReceiveTokenChange={handleReceiveTokenChange}
                />
            </div>

            {/* Route Info + Mode + Submit */}
            <div className="space-y-3 lg:space-y-4">
                {/* Transfer Mode Toggle */}
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

                {/* Route Display */}
                <RouteDisplay
                    routeLoading={routeLoading}
                    routePending={routePending}
                    routeError={routeError}
                    routeInfo={routeInfo}
                    routeIsStale={routeIsStale}
                    quoteAgeSeconds={quoteAgeSeconds}
                    selectedReceiveToken={selectedReceiveToken}
                    amount={amount}
                />

                {/* Submit Button */}
                <TransferButton
                    canSubmit={canSubmit}
                    isPending={isPending}
                    isRefreshing={isRefreshing}
                    isWalletReady={isWalletReady}
                    pathfinderSuccess={pathfinderResponse?.success ?? false}
                    routeLoading={routeLoading}
                    routePending={routePending}
                    routeIsStale={routeIsStale}
                    mode={mode}
                    onSubmit={handleSubmit}
                />
            </div>
        </div>
    );
}
